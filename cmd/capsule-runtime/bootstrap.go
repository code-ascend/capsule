package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"slices"

	"capsule/internal/format/binconfig"
	"capsule/internal/format/selfread"
	"capsule/internal/runtime/hostexec"
	"capsule/internal/sys/exitcode"

	"github.com/leonelquinteros/gotext"
)

type appState struct {
	selfPath string
	layout   *selfread.Layout
	cfg      *binconfig.Config
	execName string
	selfName string
}

// earlyDispatch handles binary-name redirects and the inside-capsule guard.
// Returns (code, true) when the call was handled and main should exit.
func earlyDispatch(ctx context.Context) (int, bool) {
	name := filepath.Base(os.Args[0])
	if name == binconfig.HostExecCommand {
		return hostexec.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr), true
	}
	if slices.Contains(binconfig.HostExecForwardedAliases, name) {
		return hostexec.Run(ctx, append([]string{name}, os.Args[1:]...), os.Stdin, os.Stdout, os.Stderr), true
	}
	if os.Getenv(binconfig.InsideEnv) != "" {
		fmt.Fprintln(os.Stderr, gotext.Get("capsule: already inside a capsule (host PATH leak); run the in-capsule binary directly instead of the capsule wrapper"))
		return exitcode.Error, true
	}
	return 0, false
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
