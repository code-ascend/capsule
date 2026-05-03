package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"capsule/internal/i18n"
	"capsule/internal/runtime/reaper"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"
	"capsule/internal/version"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

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

	if code, handled := earlyDispatch(ctx); handled {
		return code
	}

	runner, err := NewRunner()
	if err != nil {
		return exitcode.Report(ctx, err, gotext.Get("Error"))
	}
	if runner.IsSymlinkInvocation() {
		return exitcode.Report(ctx, runner.Symlink(ctx, os.Args[1:]), gotext.Get("Error"))
	}
	return exitcode.Report(ctx, buildApp(runner).Run(ctx, os.Args), gotext.Get("Error"))
}

func buildApp(runner *Runner) *cli.Command {
	cli.HelpFlag = &cli.BoolFlag{Name: "help", Aliases: []string{"h"}, Usage: gotext.Get("show help"), HideDefault: true, Local: true}
	cli.VersionFlag = &cli.BoolFlag{Name: "version", Usage: gotext.Get("print the version"), HideDefault: true, Local: true}

	return &cli.Command{
		Name:            "capsule",
		Version:         version.Version,
		HideHelpCommand: true,
		Usage:           gotext.Get("Portable Linux container runtime"),
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
