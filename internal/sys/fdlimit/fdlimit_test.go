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

package fdlimit

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestRaise(t *testing.T) {
	if err := Raise(); err != nil {
		t.Fatalf("Raise: %v", err)
	}
	var l unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &l); err != nil {
		t.Fatal(err)
	}
	want := min(l.Max, maxNoFile)
	if l.Cur != want {
		t.Errorf("soft limit %d, want %d", l.Cur, want)
	}
}
