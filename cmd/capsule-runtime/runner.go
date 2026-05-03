package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"runtime/debug"

	"capsule/internal/runtime/bwrap"
	"capsule/internal/runtime/commit"
	"capsule/internal/runtime/export"
	"capsule/internal/runtime/hostexec"
	"capsule/internal/runtime/nvidia"
	"capsule/internal/runtime/overlay"
	"capsule/internal/runtime/session"
	"capsule/internal/runtime/update"
	"capsule/internal/sys/log"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

// Runner owns the loaded capsule state and exposes commands as methods.
type Runner struct {
	state *appState
}

func NewRunner(state *appState) *Runner {
	return &Runner{state: state}
}

// wrap binds the Runner to a domain-shaped action and returns a cli.ActionFunc.
func (r *Runner) wrap(fn func(context.Context, *cli.Command, *Runner) error) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		return fn(ctx, cmd, r)
	}
}

// runOptions are the CLI-supplied per-invocation knobs.
type runOptions struct {
	Binds      []string
	Env        []string
	EnvUnset   []string
	Home       string
	NoOverlay  bool
	NoNvidia   bool
	SquashFuse string
}

func collectOpts(cmd *cli.Command) runOptions {
	return runOptions{
		Binds:      cmd.StringSlice("bind"),
		Env:        cmd.StringSlice("env"),
		EnvUnset:   cmd.StringSlice("unsetenv"),
		Home:       cmd.String("home"),
		NoOverlay:  cmd.Bool("no-overlay"),
		NoNvidia:   cmd.Bool("no-nvidia"),
		SquashFuse: cmd.String("squashfuse"),
	}
}

func (o runOptions) sessionOpts() session.Options {
	return session.Options{
		NoOverlay:  o.NoOverlay,
		NoNvidia:   o.NoNvidia,
		SquashFuse: o.SquashFuse,
	}
}

func (r *Runner) Default(ctx context.Context, args []string, opts runOptions) error {
	return r.runInContainer(ctx, args, opts)
}

func (r *Runner) Shell(ctx context.Context, extraArgs []string, opts runOptions) error {
	return r.runInContainer(ctx, append([]string{"/bin/bash"}, extraArgs...), opts)
}

// MountOnly prints the squashfs mount point and leaves it mounted.
func (r *Runner) MountOnly(ctx context.Context) error {
	s, err := session.Open(r.state.selfPath, r.state.layout.SquashfsOffset, session.Options{})
	if err != nil {
		return err
	}
	rootPath, err := s.MountRoot(ctx)
	if err != nil {
		s.Close()
		return err
	}
	fmt.Println(rootPath)
	return nil
}

// Symlink dispatches a binary aliased via a symlink to the capsule.
func (r *Runner) Symlink(ctx context.Context, args []string) error {
	target := ""
	for _, b := range r.state.cfg.Binaries {
		if filepath.Base(b) == r.state.execName {
			target = b
			break
		}
	}
	if target == "" {
		return errors.New(gotext.Get("capsule symlink %q has no matching exported binary", r.state.execName))
	}
	return r.runInContainer(ctx, append([]string{target}, args...), runOptions{})
}

func (r *Runner) runInContainer(ctx context.Context, cmd []string, opts runOptions) error {
	s, err := session.Open(r.state.selfPath, r.state.layout.SquashfsOffset, opts.sessionOpts())
	if err != nil {
		return err
	}
	defer s.Close()

	rootPath, err := s.MountRoot(ctx)
	if err != nil {
		return err
	}
	ov, err := s.EnableOverlay(ctx, rootPath)
	if err != nil {
		return err
	}

	rootMain := rootPath
	rootWritable := false
	mergedDir := ""
	if ov != nil {
		rootMain = ov.RootPath
		rootWritable = true
	} else {
		mergedDir, err = s.StageMergedUserFiles(rootPath)
		if err != nil {
			return fmt.Errorf("merge user files: %w", err)
		}
	}

	env := bwrap.EnvFromOS()
	if opts.Home != "" {
		env.CapsuleHome = opts.Home
	}

	spec := &bwrap.Spec{
		RootPath:      rootMain,
		RootWritable:  rootWritable,
		MergedUserDir: mergedDir,
		Cfg:           r.state.cfg,
		Cmd:           cmd,
		Env:           env,
		Binds:         opts.Binds,
		EnvSet:        opts.Env,
		EnvUnset:      opts.EnvUnset,
	}

	if r.state.cfg.HostExec {
		srv, lerr := hostexec.Listen()
		if lerr != nil {
			return fmt.Errorf("hostexec listen: %w", lerr)
		}
		defer func() {
			if err := srv.Close(); err != nil {
				log.Debug("hostexec close failed", "error", err)
			}
		}()
		go srv.Serve(ctx)
		spec.HostExecSocket = srv.SocketPath()
		spec.HostExecBinPath = r.state.selfPath
	}

	// Setup peak is over; bwrap blocks for the app's lifetime — return pages now.
	debug.FreeOSMemory()

	code, err := spec.Run(ctx, s.Bundle())
	if err != nil {
		return err
	}
	if code != 0 {
		return cli.Exit("", code)
	}
	return nil
}

func (r *Runner) Export(ctx context.Context, filter string) error {
	f, err := export.ParseFilter(filter)
	if err != nil {
		return err
	}
	s, err := session.Open(r.state.selfPath, r.state.layout.SquashfsOffset, session.Options{})
	if err != nil {
		return err
	}
	defer s.Close()
	rootPath, err := s.MountRoot(ctx)
	if err != nil {
		return err
	}

	ex, err := export.New(r.state.selfPath, r.state.cfg, rootPath)
	if err != nil {
		return err
	}

	switch f {
	case export.FilterAll:
		if err = ex.Apps(); err != nil {
			return err
		}
		if err = ex.Binaries(); err != nil {
			return err
		}
	case export.FilterApps:
		if err = ex.Apps(); err != nil {
			return err
		}
	case export.FilterBinaries:
		if err = ex.Binaries(); err != nil {
			return err
		}
	}
	ex.MaybeUpdateDesktopCaches(ctx)
	fmt.Println(gotext.Get("Export complete"))
	return nil
}

func (r *Runner) Unexport(filter string) error {
	f, err := export.ParseFilter(filter)
	if err != nil {
		return err
	}
	ex, err := export.New(r.state.selfPath, r.state.cfg, "")
	if err != nil {
		return err
	}
	switch f {
	case export.FilterAll:
		if err = ex.UnexportApps(); err != nil {
			return err
		}
		if err = ex.UnexportBinaries(); err != nil {
			return err
		}
	case export.FilterApps:
		if err = ex.UnexportApps(); err != nil {
			return err
		}
	case export.FilterBinaries:
		if err = ex.UnexportBinaries(); err != nil {
			return err
		}
	}
	fmt.Println(gotext.Get("Unexport complete"))
	return nil
}

func (r *Runner) Commit(ctx context.Context) error {
	s, err := session.Open(r.state.selfPath, r.state.layout.SquashfsOffset, session.Options{})
	if err != nil {
		return err
	}
	defer s.Close()
	if !s.Workspace().LastSession() {
		return errors.New(gotext.Get("commit refused: other capsule sessions are active; close them first"))
	}
	rootPath, err := s.MountRoot(ctx)
	if err != nil {
		return err
	}

	loc := overlay.New(r.state.selfPath)
	if err = loc.EnsureDirs(); err != nil {
		return err
	}

	if err = r.commitOptions(s, loc, rootPath).Run(ctx); err != nil {
		if errors.Is(err, commit.ErrEmpty) {
			fmt.Println(gotext.Get("Nothing to commit"))
			return nil
		}
		return err
	}
	r.resetSudoUserOverlay()
	if info, err := os.Stat(r.state.selfPath); err == nil {
		fmt.Println(gotext.Get("Commit complete (%.2f MB)", float64(info.Size())/(1024*1024)))
	}
	return nil
}

func (r *Runner) Update(ctx context.Context) error {
	if err := update.CheckPreconditions(r.state.cfg.UpdateScript); err != nil {
		return err
	}
	s, err := session.Open(r.state.selfPath, r.state.layout.SquashfsOffset, session.Options{})
	if err != nil {
		return err
	}
	defer s.Close()
	if !s.Workspace().LastSession() {
		return errors.New(gotext.Get("update refused: other capsule sessions are active; close them first"))
	}
	rootPath, err := s.MountRoot(ctx)
	if err != nil {
		return err
	}
	ov, err := s.EnableOverlay(ctx, rootPath)
	if err != nil {
		return err
	}
	if ov == nil {
		return errors.New(gotext.Get("update requires overlay; could not mount unionfs"))
	}

	backup, err := update.Take(ctx, ov.Loc.Upper())
	if err != nil {
		return err
	}

	spec := &bwrap.Spec{
		RootPath:     ov.RootPath,
		RootWritable: true,
		Cfg:          r.state.cfg,
		Cmd:          []string{"/bin/bash", "-c", "set -e; " + r.state.cfg.UpdateScript},
		Env:          bwrap.EnvFromOS(),
	}
	code, runErr := spec.Run(ctx, s.Bundle())
	if runErr != nil || code != 0 {
		log.Error("update script failed; rolling back", "exit", code, "err", runErr)
		if rerr := backup.Restore(ov.Loc.Upper()); rerr != nil {
			return fmt.Errorf("rollback failed: %w (original error: %v)", rerr, runErr)
		}
		if runErr != nil {
			return runErr
		}
		return cli.Exit("", code)
	}
	backup.Discard()

	if err = r.commitOptions(s, ov.Loc, rootPath).Run(ctx); err != nil && !errors.Is(err, commit.ErrEmpty) {
		return err
	}
	r.resetSudoUserOverlay()
	fmt.Println(gotext.Get("Update complete"))
	return nil
}

func (r *Runner) Clean() error {
	loc := overlay.New(r.state.selfPath)
	if err := loc.Clean(); err != nil {
		return err
	}
	fmt.Println(gotext.Get("Overlay removed:"), loc.Base)
	return nil
}

func (r *Runner) commitOptions(s *session.Session, loc *overlay.Locator, rootPath string) *commit.Options {
	return &commit.Options{
		CapsulePath:    r.state.selfPath,
		Layout:         r.state.layout,
		Overlay:        loc,
		Bundle:         s.Bundle(),
		Compression:    r.state.cfg.Compression,
		SquashfsMount:  rootPath,
		PreCommitClean: nvidia.CleanUpper,
	}
}

// resetSudoUserOverlay wipes the invoking user's overlay after a sudo commit.
func (r *Runner) resetSudoUserOverlay() {
	if os.Getuid() != 0 {
		return
	}
	sudoUser := os.Getenv("SUDO_USER")
	if sudoUser == "" {
		return
	}
	u, err := user.Lookup(sudoUser)
	if err != nil {
		return
	}
	loc := overlay.NewForUser(r.state.selfPath, u.HomeDir)
	if err := loc.Clean(); err == nil {
		log.Info("cleaned sudo user overlay", "user", sudoUser, "dir", loc.Base)
	}
}
