package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"slices"
	"syscall"

	"capsule/internal/format/binconfig"
	"capsule/internal/format/selfread"
	"capsule/internal/i18n"
	"capsule/internal/runtime/hostexec"
	"capsule/internal/runtime/reaper"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"
	"capsule/internal/version"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

type appState struct {
	selfPath string
	layout   *selfread.Layout
	cfg      *binconfig.Config
	execName string
	selfName string
}

func init() {
	cli.HelpFlag = &cli.BoolFlag{Name: "help", Usage: "show help", HideDefault: true, Local: true}
	cli.VersionFlag = &cli.BoolFlag{Name: "version", Usage: "print the version", HideDefault: true, Local: true}
}

func main() {
	os.Exit(run())
}

func run() int {
	i18n.Setup()
	if v := os.Getenv("CAPSULE_DEBUG"); v != "" && v != "0" {
		log.Init(true)
	}
	if err := reaper.EnableSubReaper(); err != nil {
		log.Debug("reaper init failed (kernel < 3.4?)", "error", err)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	name := filepath.Base(os.Args[0])
	if name == binconfig.HostExecCommand {
		return hostexec.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	}
	if slices.Contains(binconfig.HostExecForwardedAliases, name) {
		return hostexec.Run(ctx, append([]string{name}, os.Args[1:]...), os.Stdin, os.Stdout, os.Stderr)
	}

	if os.Getenv(binconfig.InsideEnv) != "" {
		fmt.Fprintln(os.Stderr, gotext.Get("capsule: already inside a capsule (host PATH leak); run the in-capsule binary directly instead of the capsule wrapper"))
		return exitcode.Error
	}

	state, err := loadAppState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "capsule-runtime: %v\n", err)
		return exitcode.Error
	}

	runner := NewRunner(state)
	if state.execName != state.selfName {
		return reportErr(ctx, runner.Symlink(ctx, os.Args[1:]))
	}
	return reportErr(ctx, buildApp(runner).Run(ctx, os.Args))
}

// reportErr unwraps cli.ExitCoder, distinguishes interrupt, and prints
// non-exit errors on stderr.
func reportErr(ctx context.Context, err error) int {
	if err == nil {
		return exitcode.OK
	}
	var exitErr cli.ExitCoder
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	if ctx.Err() != nil {
		return exitcode.Interrupted
	}
	fmt.Fprintf(os.Stderr, "capsule-runtime: %v\n", err)
	return exitcode.Error
}

func loadAppState() (*appState, error) {
	selfPath, err := selfread.SelfPath()
	if err != nil {
		return nil, fmt.Errorf("locate self: %w", err)
	}
	layout, err := selfread.ReadLayout(selfPath)
	if err != nil {
		return nil, fmt.Errorf("parse footer: %w", err)
	}
	rawCfg, err := selfread.ReadBinConfig(selfPath, layout)
	if err != nil {
		return nil, fmt.Errorf("read binconfig: %w", err)
	}
	cfg := &binconfig.Config{}
	if len(rawCfg) > 0 {
		cfg, err = binconfig.Unmarshal(rawCfg)
		if err != nil {
			return nil, err
		}
	}

	return &appState{
		selfPath: selfPath,
		layout:   layout,
		cfg:      cfg,
		execName: filepath.Base(os.Args[0]),
		selfName: filepath.Base(selfPath),
	}, nil
}

func buildApp(runner *Runner) *cli.Command {
	return &cli.Command{
		Name:    "capsule",
		Version: version.Version,
		Usage:   gotext.Get("Portable Linux container runtime"),
		Before: func(ctx context.Context, cmd *cli.Command) (context.Context, error) {
			if cmd.Bool("verbose") {
				log.Init(true)
			}
			return ctx, nil
		},
		Commands: []*cli.Command{
			{
				Name:            "shell",
				Usage:           gotext.Get("Start an interactive shell inside the capsule"),
				Aliases:         []string{"s"},
				SkipFlagParsing: true,
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					return r.Shell(ctx, cmd.Args().Slice(), collectOpts(cmd))
				}),
			},
			{
				Name:  "mount-only",
				Usage: gotext.Get("Mount the squashfs and print the mount point"),
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					return r.MountOnly(ctx)
				}),
			},
			{
				Name:      "export",
				Usage:     gotext.Get("Export apps/binaries to the host (all|apps|binaries)"),
				ArgsUsage: "[filter]",
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					return r.Export(ctx, cmd.Args().First())
				}),
			},
			{
				Name:      "unexport",
				Usage:     gotext.Get("Remove exported apps/binaries (all|apps|binaries)"),
				ArgsUsage: "[filter]",
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					return r.Unexport(cmd.Args().First())
				}),
			},
			{
				Name:  "commit",
				Usage: gotext.Get("Commit overlay changes into the squashfs image"),
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					return r.Commit(ctx)
				}),
			},
			{
				Name:  "update",
				Usage: gotext.Get("Run the configured update script and commit the result"),
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					return r.Update(ctx)
				}),
			},
			{
				Name:  "clean",
				Usage: gotext.Get("Remove overlay data (reset capsule to a clean state)"),
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					return r.Clean()
				}),
			},
		},

		StopOnNthArg: ptr(1),
		Flags: []cli.Flag{
			&cli.StringSliceFlag{
				Name:    "bind",
				Aliases: []string{"b"},
				Sources: cli.EnvVars("CAPSULE_BIND"),
				Usage:   gotext.Get("Mount host path into the capsule (`SRC[:DST]`, repeatable)"),
			},
			&cli.StringSliceFlag{
				Name:    "env",
				Aliases: []string{"e"},
				Sources: cli.EnvVars("CAPSULE_ENV"),
				Usage:   gotext.Get("Set env var inside the capsule (`KEY=VAL`, repeatable, overrides config)"),
			},
			&cli.StringSliceFlag{
				Name:    "unsetenv",
				Aliases: []string{"u"},
				Sources: cli.EnvVars("CAPSULE_UNSETENV"),
				Usage:   gotext.Get("Drop env var inside the capsule (`KEY`, repeatable)"),
			},
			&cli.StringFlag{
				Name:    "home",
				Aliases: []string{"h"},
				Sources: cli.EnvVars("CAPSULE_HOME"),
				Usage:   gotext.Get("Override capsule home directory (`PATH`)"),
			},
			&cli.BoolFlag{
				Name:    "verbose",
				Aliases: []string{"v"},
				Sources: cli.EnvVars("CAPSULE_DEBUG"),
				Usage:   gotext.Get("Enable debug logging"),
			},
			&cli.BoolFlag{
				Name:    "no-overlay",
				Sources: cli.EnvVars("CAPSULE_NO_OVERLAY"),
				Usage:   gotext.Get("Disable unionfs overlay (read-only rootfs)"),
			},
			&cli.BoolFlag{
				Name:    "no-nvidia",
				Sources: cli.EnvVars("CAPSULE_NO_NVIDIA"),
				Usage:   gotext.Get("Skip NVIDIA driver passthrough"),
			},
			&cli.StringFlag{
				Name:    "squashfuse",
				Sources: cli.EnvVars("CAPSULE_SQUASHFUSE"),
				Usage:   gotext.Get("Squashfs FUSE backend: `auto|3|ll` (3 is lighter; ll is faster)"),
			},
		},
		Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
			return r.Default(ctx, cmd.Args().Slice(), collectOpts(cmd))
		}),
	}
}

func ptr[T any](v T) *T { return &v }
