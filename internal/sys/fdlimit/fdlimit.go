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

import "golang.org/x/sys/unix"

// maxNoFile caps the raise: huge limits slow down close-loop programs like rpm.
const maxNoFile = 524288

// Raise lifts the soft RLIMIT_NOFILE towards the hard limit; helps unionfs-fuse
// and fd-hungry apps. The explicit Setrlimit makes exec'd children inherit it.
func Raise() error {
	var l unix.Rlimit
	if err := unix.Getrlimit(unix.RLIMIT_NOFILE, &l); err != nil {
		return err
	}
	target := min(l.Max, maxNoFile)
	if l.Cur >= target {
		return nil
	}
	l.Cur = target
	return unix.Setrlimit(unix.RLIMIT_NOFILE, &l)
}
