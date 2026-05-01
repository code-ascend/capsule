package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"

	"capsule/internal/runtime/bwrap"
	"capsule/internal/runtime/commit"
	"capsule/internal/runtime/export"
	"capsule/internal/runtime/hostexec"
	"capsule/internal/runtime/nvidia"
	"capsule/internal/runtime/overlay"
	"capsule/internal/runtime/update"
	"capsule/internal/sys/log"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

// resetSudoUserOverlay wipes the invoking user's overlay after a sudo commit;
// otherwise their stale upper would whiteout files in the new squashfs.
func resetSudoUserOverlay(capsulePath string) {
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
	loc := overlay.NewForUser(capsulePath, u.HomeDir)
	if err := loc.Clean(); err == nil {
		log.Info("cleaned sudo user overlay", "user", sudoUser, "dir", loc.Base)
	}
}

func runDefault(ctx context.Context, state *appState, args []string) error {
	return runInContainer(ctx, state, args)
}

func runShell(ctx context.Context, state *appState, extraArgs []string) error {
	return runInContainer(ctx, state, append([]string{"/bin/bash"}, extraArgs...))
}

// runMountOnly skips workspace cleanup so the mount survives the process exit.
func runMountOnly(ctx context.Context, state *appState) error {
	s, err := openSession(state)
	if err != nil {
		return err
	}
	m, err := s.mountRoot(ctx)
	if err != nil {
		s.close()
		return err
	}
	fmt.Println(m.RootPath)
	return nil
}

func runInContainer(ctx context.Context, state *appState, cmd []string) error {
	s, err := openSession(state)
	if err != nil {
		return err
	}
	defer s.close()

	m, err := s.mountRoot(ctx)
	if err != nil {
		return err
	}
	ov, err := s.enableOverlay(ctx, m)
	if err != nil {
		return err
	}

	rootPath := m.RootPath
	rootWritable := false
	mergedDir := ""
	if ov != nil {
		rootPath = ov.RootPath
		rootWritable = true
	} else {
		mergedDir, err = s.stageMergedUserFiles(m)
		if err != nil {
			return fmt.Errorf("merge user files: %w", err)
		}
	}

	spec := &bwrap.Spec{
		RootPath:      rootPath,
		RootWritable:  rootWritable,
		MergedUserDir: mergedDir,
		Cfg:           s.state.cfg,
		Cmd:           cmd,
		Env:           bwrap.EnvFromOS(),
	}

	if state.cfg.HostExec {
		srv, lerr := hostexec.Listen()
		if lerr != nil {
			return fmt.Errorf("hostexec listen: %w", lerr)
		}
		defer srv.Close()
		go srv.Serve(ctx)
		spec.HostExecSocket = srv.SocketPath()
		spec.HostExecBinPath = state.selfPath
	}

	code, err := spec.Run(ctx, s.bundle)
	if err != nil {
		return err
	}
	if code != 0 {
		return cli.Exit("", code)
	}
	return nil
}

func runSymlink(ctx context.Context, state *appState, args []string) error {
	target := ""
	for _, b := range state.cfg.Binaries {
		if filepath.Base(b) == state.execName {
			target = b
			break
		}
	}
	if target == "" {
		return errors.New(gotext.Get("capsule symlink %q has no matching exported binary", state.execName))
	}
	return runInContainer(ctx, state, append([]string{target}, args...))
}

func runExport(ctx context.Context, state *appState, filter string) error {
	f, err := export.ParseFilter(filter)
	if err != nil {
		return err
	}
	s, err := openSession(state)
	if err != nil {
		return err
	}
	defer s.close()
	m, err := s.mountRoot(ctx)
	if err != nil {
		return err
	}

	ex, err := export.New(state.selfPath, state.cfg, m.RootPath)
	if err != nil {
		return err
	}

	switch f {
	case export.FilterAll, export.FilterApps:
		if err = ex.Apps(); err != nil {
			return err
		}
		if f == export.FilterApps {
			break
		}
		fallthrough
	case export.FilterBinaries:
		if err = ex.Binaries(); err != nil {
			return err
		}
	}
	ex.MaybeUpdateDesktopCaches(ctx)
	fmt.Println(gotext.Get("Export complete"))
	return nil
}

func runUnexport(state *appState, filter string) error {
	f, err := export.ParseFilter(filter)
	if err != nil {
		return err
	}
	ex, err := export.New(state.selfPath, state.cfg, "")
	if err != nil {
		return err
	}
	switch f {
	case export.FilterAll, export.FilterApps:
		if err = ex.UnexportApps(); err != nil {
			return err
		}
		if f == export.FilterApps {
			break
		}
		fallthrough
	case export.FilterBinaries:
		if err = ex.UnexportBinaries(); err != nil {
			return err
		}
	}
	fmt.Println(gotext.Get("Unexport complete"))
	return nil
}

func runCommit(ctx context.Context, state *appState) error {
	s, err := openSession(state)
	if err != nil {
		return err
	}
	defer s.close()
	if !s.workspace.LastSession() {
		return errors.New(gotext.Get("commit refused: other capsule sessions are active; close them first"))
	}
	m, err := s.mountRoot(ctx)
	if err != nil {
		return err
	}

	loc := overlay.New(state.selfPath)
	if err = loc.EnsureDirs(); err != nil {
		return err
	}

	opts := &commit.Options{
		CapsulePath:    state.selfPath,
		Layout:         state.layout,
		Overlay:        loc,
		Bundle:         s.bundle,
		Compression:    state.cfg.Compression,
		SquashfsMount:  m.RootPath,
		PreCommitClean: nvidia.CleanUpper,
	}
	if err = opts.Run(ctx); err != nil {
		if errors.Is(err, commit.ErrEmpty) {
			fmt.Println(gotext.Get("Nothing to commit"))
			return nil
		}
		return err
	}
	resetSudoUserOverlay(state.selfPath)
	if info, err := os.Stat(state.selfPath); err == nil {
		fmt.Println(gotext.Get("Commit complete (%.2f MB)", float64(info.Size())/(1024*1024)))
	}
	return nil
}

func runUpdate(ctx context.Context, state *appState) error {
	if err := update.CheckPreconditions(state.cfg.UpdateScript); err != nil {
		return err
	}
	s, err := openSession(state)
	if err != nil {
		return err
	}
	defer s.close()
	if !s.workspace.LastSession() {
		return errors.New(gotext.Get("update refused: other capsule sessions are active; close them first"))
	}
	m, err := s.mountRoot(ctx)
	if err != nil {
		return err
	}
	ov, err := s.enableOverlay(ctx, m)
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
		Cfg:          s.state.cfg,
		Cmd:          []string{"/bin/bash", "-c", "set -e; " + state.cfg.UpdateScript},
		Env:          bwrap.EnvFromOS(),
	}
	code, runErr := spec.Run(ctx, s.bundle)
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

	opts := &commit.Options{
		CapsulePath:    state.selfPath,
		Layout:         state.layout,
		Overlay:        ov.Loc,
		Bundle:         s.bundle,
		Compression:    state.cfg.Compression,
		SquashfsMount:  m.RootPath,
		PreCommitClean: nvidia.CleanUpper,
	}
	if err = opts.Run(ctx); err != nil && !errors.Is(err, commit.ErrEmpty) {
		return err
	}
	resetSudoUserOverlay(state.selfPath)
	fmt.Println(gotext.Get("Update complete"))
	return nil
}

func runClean(state *appState) error {
	loc := overlay.New(state.selfPath)
	if err := loc.Clean(); err != nil {
		return err
	}
	fmt.Println(gotext.Get("Overlay removed:"), loc.Base)
	return nil
}
