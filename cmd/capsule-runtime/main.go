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

package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"capsule/internal/cli/runtimecli"
	"capsule/internal/i18n"
	"capsule/internal/runtime/reaper"
	"capsule/internal/sys/log"
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

	return runtimecli.Run(ctx, os.Args)
}
