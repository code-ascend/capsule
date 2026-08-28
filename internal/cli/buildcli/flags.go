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
	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

// verboseFlag is the persistent debug-logging switch shared by every command.
func verboseFlag() cli.Flag {
	return &cli.BoolFlag{Name: "verbose", Aliases: []string{"v"}, Usage: gotext.Get("Verbose output")}
}

// pathFlag adds extra scan directories to capsule discovery commands.
func pathFlag() cli.Flag {
	return &cli.StringSliceFlag{Name: "path", Aliases: []string{"p"}, Usage: gotext.Get("Additional directory to scan (repeatable)")}
}

func jsonFlags() []cli.Flag {
	return []cli.Flag{
		&cli.BoolFlag{Name: "json", Usage: gotext.Get("Machine-readable output")},
	}
}
