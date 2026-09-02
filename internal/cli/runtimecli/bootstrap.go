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
	"errors"
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

// earlyDispatch handles binary-name redirects and the inside-capsule guard, returning (code, true) when handled.
func earlyDispatch(ctx context.Context) (int, bool) {
	name := filepath.Base(os.Args[0])
	if name == binconfig.HostExecCommand {
		return hostexec.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr), true
	}
	if slices.Contains(binconfig.HostExecForwardedAliases, name) {
		return hostexec.Run(ctx, append([]string{name}, os.Args[1:]...), os.Stdin, os.Stdout, os.Stderr), true
	}
	if os.Getenv(binconfig.InsideEnv) != "" {
		err := errors.New(gotext.Get("already inside a capsule (host PATH leak); run the in-capsule binary directly instead of the capsule wrapper"))
		return exitcode.Report(ctx, err), true
	}
	return 0, false
}

func loadAppState() (*appState, error) {
	selfPath, err := selfread.SelfPath()
	if err != nil {
		return nil, fmt.Errorf("locate self: %w", err)
	}
	layout, cfg, err := selfread.LoadConfig(selfPath)
	if err != nil {
		return nil, err
	}
	return &appState{
		selfPath: selfPath,
		layout:   layout,
		cfg:      cfg,
		execName: filepath.Base(os.Args[0]),
		selfName: filepath.Base(selfPath),
	}, nil
}
