package exitcode

import "syscall"

const (
	OK          = 0
	Error       = 1
	Interrupted = 128 + int(syscall.SIGINT) // 130
)

// FromStatus maps a wait status to a shell-style exit code: the exit status, or 128+signal.
func FromStatus(ws syscall.WaitStatus) int {
	if ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ws.ExitStatus()
}
