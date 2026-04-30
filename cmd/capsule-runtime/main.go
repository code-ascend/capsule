package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"capsule/internal/format/binconfig"
	"capsule/internal/format/selfread"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"

	"github.com/urfave/cli/v3"
)

type appState struct {
	selfPath string
	layout   *selfread.Layout
	cfg      *binconfig.Config
	execName string
	selfName string
	debug    bool
}

func main() {
	os.Exit(run())
}

func run() int {
	if v := os.Getenv("CAPSULE_DEBUG"); v != "" && v != "0" {
		log.Init(true)
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	state, err := loadAppState()
	if err != nil {
		fmt.Fprintf(os.Stderr, "capsule-runtime: %v\n", err)
		return exitcode.Error
	}

	if state.execName != state.selfName {
		if err = runSymlink(ctx, state, os.Args[1:]); err != nil {
			fmt.Fprintf(os.Stderr, "capsule-runtime: %v\n", err)
			return exitcode.Error
		}
		return exitcode.OK
	}

	if err = buildApp(state).Run(ctx, normalizeArgs(os.Args)); err != nil {
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
	return exitcode.OK
}

// normalizeArgs maps legacy `--shell` / `--export` / ... onto cli/v3 subcommands.
func normalizeArgs(args []string) []string {
	if len(args) < 2 {
		return args
	}
	aliases := map[string]string{
		"--help":       "help",
		"-h":           "help",
		"--shell":      "shell",
		"-s":           "shell",
		"--mount-only": "mount-only",
		"--export":     "export",
		"--unexport":   "unexport",
		"--commit":     "commit",
		"--update":     "update",
		"--clean":      "clean",
	}
	out := make([]string, len(args))
	copy(out, args)
	if cmd, ok := aliases[out[1]]; ok {
		out[1] = cmd
	}
	return out
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
	rawCfg, err := selfread.ReadBinconfig(selfPath, layout)
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
		debug:    log.IsDebug(),
	}, nil
}

func buildApp(state *appState) *cli.Command {
	return &cli.Command{
		Name:  "capsule",
		Usage: "Portable Linux container runtime",
		Commands: []*cli.Command{
			{
				Name:            "shell",
				Usage:           "Start an interactive shell inside the capsule",
				Aliases:         []string{"s"},
				SkipFlagParsing: true,
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runShell(ctx, state, cmd.Args().Slice())
				},
			},
			{
				Name:  "mount-only",
				Usage: "Mount the squashfs and print the mount point",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runMountOnly(ctx, state)
				},
			},
			{
				Name:      "export",
				Usage:     "Export apps/binaries to the host (all|apps|binaries)",
				ArgsUsage: "[filter]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runExport(ctx, state, cmd.Args().First())
				},
			},
			{
				Name:      "unexport",
				Usage:     "Remove exported apps/binaries (all|apps|binaries)",
				ArgsUsage: "[filter]",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runUnexport(state, cmd.Args().First())
				},
			},
			{
				Name:  "commit",
				Usage: "Commit overlay changes into the squashfs image",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runCommit(ctx, state)
				},
			},
			{
				Name:  "update",
				Usage: "Run the configured update script and commit the result",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runUpdate(ctx, state)
				},
			},
			{
				Name:  "clean",
				Usage: "Remove overlay data (reset capsule to a clean state)",
				Action: func(ctx context.Context, cmd *cli.Command) error {
					return runClean(state)
				},
			},
		},
		// SkipFlagParsing on root preserves `./capsule /bin/ls -la /` invocation.
		SkipFlagParsing: true,
		Action: func(ctx context.Context, cmd *cli.Command) error {
			return runDefault(ctx, state, cmd.Args().Slice())
		},
	}
}
