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

package buildcli

import (
	"context"

	"capsule/internal/build/manager"
	"capsule/internal/cli/clihelp"
	"capsule/internal/sys/log"
	"capsule/internal/version"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

// New builds the root command.
func New() *cli.Command {
	clihelp.Setup()
	root := &cli.Command{
		Name:                  "capsule",
		Version:               version.Version,
		HideHelpCommand:       true,
		EnableShellCompletion: true,
		Usage:                 gotext.Get("Create portable Linux containers from OCI images"),
		Description: gotext.Get(`capsule is a tool for creating portable Linux containers as single ELF executables.
It reads a YAML config file specifying the image and commands, then produces a self-contained binary.`),
		Flags: []cli.Flag{verboseFlag()},
		Commands: []*cli.Command{
			{
				Name:        "build",
				Usage:       gotext.Get("Build a portable container from an OCI image"),
				ArgsUsage:   "[config.yaml]",
				Description: gotext.Get("Build a portable container from an OCI image using a YAML config file."),
				Flags: []cli.Flag{
					&cli.StringFlag{Name: "config", Aliases: []string{"c"}, Usage: gotext.Get("Path to YAML config file")},
					&cli.StringFlag{Name: "output", Aliases: []string{"o"}, Usage: gotext.Get("Output file path (overrides config)")},
					&cli.StringFlag{Name: "compression", Usage: gotext.Get("SquashFS compression: zstd, lz4, gzip, xz (overrides config)")},
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					initLog(cmd)
					path := cmd.Args().First()
					if path == "" {
						path = cmd.String("config")
					}
					return build(ctx, path, cmd.String("output"), cmd.String("compression"))
				},
			},
			{
				Name:  "list",
				Usage: gotext.Get("List installed capsules"),
				Flags: append(jsonFlags(), pathFlag()),
				Action: func(_ context.Context, cmd *cli.Command) error {
					initLog(cmd)
					return renderResult(cmd, list(cmd.StringSlice("path")), nil)
				},
			},
			{
				Name:  "clean-storage",
				Usage: gotext.Get("Wipe capsule's private build storage"),
				Action: func(_ context.Context, cmd *cli.Command) error {
					initLog(cmd)
					return cleanStorage()
				},
			},
			{
				Name:      "update",
				Usage:     gotext.Get("Rebuild installed capsules from their source YAML"),
				ArgsUsage: "[name|path]...",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "dry-run", Aliases: []string{"dr"}, Usage: gotext.Get("Show the rebuild plan without actually executing it")},
					&cli.BoolFlag{Name: "keep-going", Aliases: []string{"k"}, Usage: gotext.Get("Continue past failed capsules instead of stopping")},
					pathFlag(),
				},
				Action: func(ctx context.Context, cmd *cli.Command) error {
					initLog(cmd)
					return updateInstalled(ctx, cmd.Args().Slice(), manager.UpdateOpts{
						DryRun:    cmd.Bool("dry-run"),
						KeepGoing: cmd.Bool("keep-going"),
					}, cmd.StringSlice("path"))
				},
			},
		},
	}
	clihelp.SilenceUsageErrors(root)
	return root
}

// initLog enables debug logging when --verbose is set anywhere in the flag chain.
func initLog(cmd *cli.Command) {
	log.Init(cmd.Bool("verbose"))
}
