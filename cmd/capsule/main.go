package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"capsule/internal/build/manager"
	"capsule/internal/i18n"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"
	"capsule/internal/version"

	"github.com/containers/buildah"
	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

func main() {
	if buildah.InitReexec() {
		return
	}
	os.Exit(run())
}

func run() int {
	i18n.Setup()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	runner := NewRunner()
	return exitcode.Report(ctx, buildApp(runner).Run(ctx, os.Args))
}

func buildApp(runner *Runner) *cli.Command {
	cli.HelpFlag = &cli.BoolFlag{Name: "help", Aliases: []string{"h"}, Usage: gotext.Get("Show help")}
	cli.VersionFlag = &cli.BoolFlag{Name: "version", Usage: gotext.Get("Print the version")}

	return &cli.Command{
		Name:            "capsule",
		Version:         version.Version,
		HideHelpCommand: true,
		Usage:           gotext.Get("Create portable Linux containers from OCI images"),
		Description: gotext.Get(`capsule is a tool for creating portable Linux containers as single ELF executables.
It reads a YAML config file specifying the image and commands, then produces a self-contained binary.`),
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
					&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: gotext.Get("Verbose output")},
				},
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					log.Init(cmd.Bool("verbose"))
					path := cmd.Args().First()
					if path == "" {
						path = cmd.String("config")
					}
					return r.Build(ctx, path, cmd.String("output"), cmd.String("compression"))
				}),
			},
			{
				Name:  "list",
				Usage: gotext.Get("List installed capsules"),
				Flags: []cli.Flag{
					&cli.StringSliceFlag{Name: "path", Aliases: []string{"p"}, Usage: gotext.Get("Additional directory to scan (repeatable)")},
				},
				Action: runner.wrap(func(_ context.Context, cmd *cli.Command, r *Runner) error {
					return r.List(cmd.StringSlice("path"))
				}),
			},
			{
				Name:      "update",
				Usage:     gotext.Get("Rebuild installed capsules from their source YAML"),
				ArgsUsage: "[name|path]...",
				Flags: []cli.Flag{
					&cli.BoolFlag{Name: "dry-run", Aliases: []string{"dr"}, Usage: gotext.Get("Show the rebuild plan without actually executing it")},
					&cli.BoolFlag{Name: "keep-going", Aliases: []string{"k"}, Usage: gotext.Get("Continue past failed capsules instead of stopping")},
					&cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: gotext.Get("Verbose output")},
					&cli.StringSliceFlag{Name: "path", Aliases: []string{"p"}, Usage: gotext.Get("Additional directory to scan (repeatable)")},
				},
				Action: runner.wrap(func(ctx context.Context, cmd *cli.Command, r *Runner) error {
					log.Init(cmd.Bool("verbose"))
					return r.UpdateInstalled(ctx, cmd.Args().Slice(), manager.UpdateOpts{
						DryRun:    cmd.Bool("dry-run"),
						KeepGoing: cmd.Bool("keep-going"),
					}, cmd.StringSlice("path"))
				}),
			},
		},
	}
}
