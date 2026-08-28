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
	"slices"
	"testing"

	"capsule/internal/format/binconfig"

	"github.com/urfave/cli/v3"
)

func flagNames(flags []cli.Flag) []string {
	var names []string
	for _, f := range flags {
		names = append(names, f.Names()[0])
	}
	return names
}

func TestRootFlagsDefault(t *testing.T) {
	names := flagNames(rootFlags(&binconfig.Config{}))
	for _, want := range []string{"no-overlay", "no-nvidia"} {
		if !slices.Contains(names, want) {
			t.Errorf("flag %q must be present by default, got %v", want, names)
		}
	}
}

func TestRootFlagsBakedHidden(t *testing.T) {
	names := flagNames(rootFlags(&binconfig.Config{NoOverlay: true, NoNvidia: true}))
	for _, banned := range []string{"no-overlay", "no-nvidia"} {
		if slices.Contains(names, banned) {
			t.Errorf("baked flag %q must be omitted, got %v", banned, names)
		}
	}
	for _, want := range []string{"bind", "verbose", "sandbox"} {
		if !slices.Contains(names, want) {
			t.Errorf("flag %q must stay, got %v", want, names)
		}
	}
}
