package exitcode

import "syscall"

const (
	OK          = 0
	Error       = 1
	Interrupted = 128 + int(syscall.SIGINT) // 130
)
