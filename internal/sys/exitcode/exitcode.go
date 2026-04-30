// Package exitcode holds POSIX-style process exit codes shared between cmd
// binaries. Signal-driven exits follow the 128 + signal_number convention.
package exitcode

import "syscall"

const (
	OK          = 0
	Error       = 1
	Interrupted = 128 + int(syscall.SIGINT) // 130
)
