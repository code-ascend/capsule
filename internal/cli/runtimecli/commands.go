// capsule
// Copyright (C) 2026 Дмитрий Удалов dmitry@udalov.online
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package runtimecli

import (
	"context"

	"capsule/internal/cli/clihelp"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"
	"capsule/internal/version"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

// Run dispatches a runtime invocation and returns the process exit code.
func Run(ctx context.Context, args []string) int {
	if code, handled := earlyDispatch(ctx); handled {
		return code
	}
	runner, err := newRunner()
	if err != nil {
		return exitcode.Report(ctx, err)
	}
	if runner.isSymlinkInvocation() {
		return exitcode.Report(ctx, runner.Symlink(ctx, args[1:]))
	}
	return exitcode.Report(ctx, newApp(runner).Run(ctx, args))
}

func newApp(runner *Runner) *cli.Command {
	clihelp.Setup()
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
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runner.Shell(ctx, cmd.Args().Slice(), collectOpts(cmd))
				},
			},
			{
				Name:  "mount-only",
				Usage: gotext.Get("Mount the squashfs and print the mount point"),
				Action: func(ctx context.Context, _ *cli.Command) error {
					return runner.MountOnly(ctx)
				},
			},
			{
				Name:      "export",
				Usage:     gotext.Get("Export apps/binaries to the host (all|apps|binaries)"),
				ArgsUsage: "[filter]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runner.Export(ctx, cmd.Args().First())
				},
			},
			{
				Name:      "unexport",
				Usage:     gotext.Get("Remove exported apps/binaries (all|apps|binaries)"),
				ArgsUsage: "[filter]",
				Action: func(_ context.Context, cmd *cli.Command) error {
					return runner.Unexport(cmd.Args().First())
				},
			},
			{
				Name:  "commit",
				Usage: gotext.Get("Commit overlay changes into the squashfs image"),
				Action: func(ctx context.Context, _ *cli.Command) error {
					return runner.Commit(ctx)
				},
			},
			{
				Name:  "update",
				Usage: gotext.Get("Run the configured update script and commit the result"),
				Action: func(ctx context.Context, _ *cli.Command) error {
					return runner.Update(ctx)
				},
			},
			{
				Name:  "clean",
				Usage: gotext.Get("Remove overlay data (reset capsule to a clean state)"),
				Action: func(_ context.Context, _ *cli.Command) error {
					return runner.Clean()
				},
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
			&cli.StringFlag{
				Name:    "sandbox",
				Sources: cli.EnvVars("CAPSULE_SANDBOX"),
				Usage:   gotext.Get("Isolation level: `shared|isolated|strict` (overrides config)"),
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runner.Default(ctx, cmd.Args().Slice(), collectOpts(cmd))
		},
	}
}

func ptr[T any](v T) *T { return &v }
