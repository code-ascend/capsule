package supervisor

import (
	"errors"
	"os"
	"os/signal"
	"time"

	"capsule/internal/format/binconfig"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"

	"golang.org/x/sys/unix"
)

// shutdownGrace is how long processes get after SIGTERM before SIGKILL.
const shutdownGrace = 5 * time.Second

// Run executes argv (the bwrap command line) under a subreaper and returns the exit code the capsule should report.
func Run(argv []string) int {
	if len(argv) == 0 {
		log.Error(binconfig.SupervisorCommand + ": no command given")
		return exitcode.Error
	}
	if err := unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0); err != nil {
		log.Error(binconfig.SupervisorCommand+": cannot become child subreaper", "error", err)
		return exitcode.Error
	}

	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, unix.SIGCHLD, unix.SIGTERM, unix.SIGINT, unix.SIGQUIT, unix.SIGHUP)

	proc, err := os.StartProcess(argv[0], argv, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Env:   os.Environ(),
	})
	if err != nil {
		log.Error(binconfig.SupervisorCommand+": cannot start", "cmd", argv[0], "error", err)
		return exitcode.Error
	}
	s := &sandbox{mainPid: proc.Pid}
	return s.serve(sigs)
}

// sandbox tracks bwrap and the shutdown state of the process tree below us.
type sandbox struct {
	mainPid    int
	code       int
	mainExited bool
	stopping   bool
}

// serve reaps until the sandbox is empty, escalating SIGTERM to SIGKILL after a stop request.
func (s *sandbox) serve(sigs <-chan os.Signal) int {
	var killAt <-chan time.Time
	for {
		select {
		case sig := <-sigs:
			switch sig {
			case unix.SIGCHLD:
				if s.reap() {
					return s.code
				}
			case unix.SIGTERM:
				killAt = s.terminate(killAt)
			case unix.SIGINT, unix.SIGQUIT, unix.SIGHUP:
				if s.mainExited {
					killAt = s.terminate(killAt)
				}
			}
		case <-killAt:
			killAt = nil
			log.Warn("capsule: processes ignored SIGTERM, sending SIGKILL")
			s.signalSandbox(unix.SIGKILL)
		}
	}
}

// terminate signals every sandbox process once and arms the SIGKILL timer.
// SIGHUP reaches shells that ignore SIGTERM; SIGCONT lets stopped jobs act on either.
func (s *sandbox) terminate(killAt <-chan time.Time) <-chan time.Time {
	if s.stopping {
		return killAt
	}
	s.stopping = true
	log.Info("capsule: shutdown requested, sending SIGTERM")
	s.signalSandbox(unix.SIGTERM, unix.SIGHUP, unix.SIGCONT)
	return time.After(shutdownGrace)
}

// signalSandbox sends sigs to every process below us except the bwrap launcher, which must exit on its own to keep the exit code.
func (s *sandbox) signalSandbox(sigs ...unix.Signal) {
	for _, pid := range descendants(os.Getpid()) {
		if pid == s.mainPid {
			continue
		}
		for _, sig := range sigs {
			_ = unix.Kill(pid, sig)
		}
	}
}

// reap collects exited children; true once none are left.
func (s *sandbox) reap() bool {
	for {
		var ws unix.WaitStatus
		pid, err := unix.Wait4(-1, &ws, unix.WNOHANG, nil)
		switch {
		case errors.Is(err, unix.EINTR):
			continue
		case errors.Is(err, unix.ECHILD):
			return true
		case err != nil:
			log.Debug(binconfig.SupervisorCommand+": wait4 failed", "error", err)
			return false
		case pid == 0:
			return false
		case pid == s.mainPid:
			s.code = exitCode(ws)
			s.mainExited = true
		}
	}
}

// exitCode maps a wait status to a shell-style exit code.
func exitCode(ws unix.WaitStatus) int {
	if ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ws.ExitStatus()
}
