package lowprio

import (
	"fmt"
	"os/exec"
	"runtime"

	"golang.org/x/sys/unix"
)

const (
	niceLowest = 19

	ioprioWhoProcess = 1
	ioprioClassBE    = 2
	ioprioClassShift = 13
	ioprioLowest     = ioprioClassBE<<ioprioClassShift | 7
)

// Run is cmd.Run with the child's priorities lowered as in Start.
func Run(cmd *exec.Cmd) error {
	if err := Start(cmd); err != nil {
		return err
	}
	return cmd.Wait()
}

// Start is cmd.Start with the child niced to 19 and its I/O at best-effort level 7.
func Start(cmd *exec.Cmd) error {
	errc := make(chan error, 1)
	go func() {
		runtime.LockOSThread()
		errc <- start(cmd)
	}()
	return <-errc
}

func start(cmd *exec.Cmd) error {
	if err := unix.Setpriority(unix.PRIO_PROCESS, 0, niceLowest); err != nil {
		return fmt.Errorf("setpriority: %w", err)
	}
	if _, _, errno := unix.Syscall(unix.SYS_IOPRIO_SET, ioprioWhoProcess, 0, ioprioLowest); errno != 0 {
		return fmt.Errorf("ioprio_set: %w", errno)
	}
	return cmd.Start()
}
