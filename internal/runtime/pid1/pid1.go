// Package pid1 is the sandbox init: runs the capsule command, reaps orphans, shuts the namespace down on request.
package pid1

import (
	"errors"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"time"

	"capsule/internal/format/binconfig"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"

	"golang.org/x/sys/unix"
)

const (
	// shutdownGrace is how long processes get after SIGTERM before SIGKILL.
	shutdownGrace = 5 * time.Second

	exitNotFound   = 127
	exitCannotExec = 126
)

// Run executes argv as PID 1 and returns the exit code the capsule should report.
func Run(argv []string) int {
	if os.Getpid() != 1 {
		log.Error(binconfig.InitCommand + ": must be PID 1 of a sandbox")
		return exitcode.Error
	}
	if len(argv) == 0 {
		log.Error(binconfig.InitCommand + ": no command given")
		return exitcode.Error
	}
	link := hostLink()

	// Terminal signals are the main command's while it lives; afterwards they mean shutdown.
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, unix.SIGCHLD, unix.SIGTERM, unix.SIGINT, unix.SIGQUIT, unix.SIGHUP)

	code := start(argv, link, sigs, stopRequests(link))
	report(link, code)
	return code
}

// start launches the main command and serves the namespace until it is empty.
func start(argv []string, link *net.UnixConn, sigs <-chan os.Signal, stop <-chan struct{}) int {
	path, err := exec.LookPath(argv[0])
	if err != nil {
		log.Error(binconfig.InitCommand+": command not found", "cmd", argv[0], "error", err)
		return exitNotFound
	}
	proc, err := os.StartProcess(path, argv, &os.ProcAttr{
		Files: []*os.File{os.Stdin, os.Stdout, os.Stderr},
		Env:   os.Environ(),
	})
	if err != nil {
		log.Error(binconfig.InitCommand+": cannot start", "cmd", path, "error", err)
		return exitCannotExec
	}
	s := &sandbox{mainPid: proc.Pid, link: link}
	return s.serve(sigs, stop)
}

// sandbox tracks the main command and the shutdown state of the namespace.
type sandbox struct {
	mainPid    int
	link       *net.UnixConn // may be nil when PID 1 runs without a host
	code       int
	mainExited bool
	stopping   bool
}

// serve reaps until the namespace is empty, escalating SIGTERM to SIGKILL after a stop request.
func (s *sandbox) serve(sigs <-chan os.Signal, stop <-chan struct{}) int {
	var killAt <-chan time.Time
	for {
		select {
		case sig := <-sigs:
			switch sig {
			case unix.SIGCHLD:
				mainWasAlive := !s.mainExited
				if s.reap() {
					return s.code
				}
				// Main command gone but orphans remain: tell the host so it can reclaim the terminal.
				if mainWasAlive && s.mainExited {
					s.notify(kindMainExited)
				}
			case unix.SIGTERM:
				killAt = s.terminate(killAt)
			case unix.SIGINT, unix.SIGQUIT, unix.SIGHUP:
				if s.mainExited {
					killAt = s.terminate(killAt)
				}
			}
		case <-stop:
			stop = nil
			killAt = s.terminate(killAt)
		case <-killAt:
			killAt = nil
			log.Warn("capsule: processes ignored SIGTERM, sending SIGKILL")
			_ = unix.Kill(-1, unix.SIGKILL)
		}
	}
}

// terminate signals every process in the namespace once and arms the SIGKILL timer.
// SIGHUP reaches shells that ignore SIGTERM; SIGCONT lets stopped jobs act on either.
func (s *sandbox) terminate(killAt <-chan time.Time) <-chan time.Time {
	if s.stopping {
		return killAt
	}
	s.stopping = true
	log.Info("capsule: shutdown requested, sending SIGTERM")
	for _, sig := range []unix.Signal{unix.SIGTERM, unix.SIGHUP, unix.SIGCONT} {
		_ = unix.Kill(-1, sig)
	}
	return time.After(shutdownGrace)
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
			log.Debug(binconfig.InitCommand+": wait4 failed", "error", err)
			return false
		case pid == 0:
			return false
		case pid == s.mainPid:
			s.code = exitCode(ws)
			s.mainExited = true
		}
	}
}

// notify sends a link frame to the host, best-effort.
func (s *sandbox) notify(kind byte) {
	if s.link == nil {
		return
	}
	if err := writeFrame(s.link, kind, 0); err != nil {
		log.Debug(binconfig.InitCommand+": notify host", "kind", string(kind), "error", err)
	}
}

// exitCode maps a wait status to a shell-style exit code.
func exitCode(ws unix.WaitStatus) int {
	if ws.Signaled() {
		return 128 + int(ws.Signal())
	}
	return ws.ExitStatus()
}

// stopRequests closes the returned channel when the host half-closes the link or goes away.
func stopRequests(link *net.UnixConn) <-chan struct{} {
	if link == nil {
		return nil
	}
	stop := make(chan struct{})
	go func() {
		_, _ = io.Copy(io.Discard, link)
		close(stop)
	}()
	return stop
}

// report hands the final exit code to the host as the last link frame.
func report(link *net.UnixConn, code int) {
	if link == nil {
		return
	}
	if err := writeFrame(link, kindExit, byte(code)); err != nil {
		log.Debug(binconfig.InitCommand+": report exit code", "error", err)
	}
	_ = link.Close()
}
