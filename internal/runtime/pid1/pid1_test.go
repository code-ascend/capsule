package pid1

import (
	"errors"
	"flag"
	"io"
	"net"
	"os"
	"os/exec"
	"os/signal"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestHelperInit is the re-exec target: it runs Run as PID 1 of the namespace the test created.
func TestHelperInit(t *testing.T) {
	if os.Getenv("CAPSULE_PID1_HELPER") == "" {
		t.Skip("helper only")
	}
	os.Exit(Run(flag.Args()))
}

// sandboxEnd returns the in-sandbox side of l and drops the host's copy, like Run's caller does after Start.
func sandboxEnd(t *testing.T, l *Link) *net.UnixConn {
	t.Helper()
	conn, err := unixConn(l.Child())
	if err != nil {
		t.Fatal(err)
	}
	l.CloseChild()
	t.Cleanup(func() { _ = conn.Close() })
	return conn
}

func newLink(t *testing.T) *Link {
	t.Helper()
	l, err := NewLink()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(l.Close)
	return l
}

func TestLinkReportsExitCode(t *testing.T) {
	l := newLink(t)
	end := sandboxEnd(t, l)
	go func() {
		_, _ = end.Write([]byte{kindExit, 42})
		_ = end.Close()
	}()
	if code, ok := l.Wait(nil); !ok || code != 42 {
		t.Fatalf("Wait = %d, %v; want 42, true", code, ok)
	}
}

func TestLinkMainExitedThenCode(t *testing.T) {
	l := newLink(t)
	end := sandboxEnd(t, l)
	go func() {
		_, _ = end.Write([]byte{kindMainExited, 0})
		_, _ = end.Write([]byte{kindExit, 5})
		_ = end.Close()
	}()
	var mains int
	code, ok := l.Wait(func() { mains++ })
	if !ok || code != 5 {
		t.Fatalf("Wait = %d, %v; want 5, true", code, ok)
	}
	if mains != 1 {
		t.Fatalf("onMainExited called %d times, want 1", mains)
	}
}

func TestLinkEOFWithoutCode(t *testing.T) {
	l := newLink(t)
	end := sandboxEnd(t, l)
	_ = end.Close()
	if code, ok := l.Wait(nil); ok {
		t.Fatalf("Wait = %d, true; want ok=false after silent EOF", code)
	}
}

func TestLinkStopHalfCloses(t *testing.T) {
	l := newLink(t)
	end := sandboxEnd(t, l)
	l.Stop()
	l.Stop() // idempotent
	if buf, err := io.ReadAll(end); err != nil || len(buf) != 0 {
		t.Fatalf("sandbox side read %v, %v; want clean EOF", buf, err)
	}
	// The exit code still travels the other way after the half-close.
	_, _ = end.Write([]byte{kindExit, 3})
	_ = end.Close()
	if code, ok := l.Wait(nil); !ok || code != 3 {
		t.Fatalf("Wait = %d, %v; want 3, true", code, ok)
	}
}

func TestHostLinkAbsent(t *testing.T) {
	// fd 3 is not a socket in a plain test process (or not open at all).
	if hostLink() != nil {
		t.Fatal("hostLink must be nil without an inherited socket")
	}
}

func TestExitCode(t *testing.T) {
	cases := map[string]struct {
		ws   unix.WaitStatus
		want int
	}{
		"exit 0":   {unix.WaitStatus(0), 0},
		"exit 3":   {unix.WaitStatus(3 << 8), 3},
		"sigterm":  {unix.WaitStatus(unix.SIGTERM), 143},
		"sigkill":  {unix.WaitStatus(unix.SIGKILL), 137},
		"exit 255": {unix.WaitStatus(255 << 8), 255},
	}
	for name, c := range cases {
		if got := exitCode(c.ws); got != c.want {
			t.Errorf("%s: exitCode = %d, want %d", name, got, c.want)
		}
	}
}

func TestReapRecordsMainStatus(t *testing.T) {
	sigs := make(chan os.Signal, 16)
	signal.Notify(sigs, unix.SIGCHLD)
	defer signal.Stop(sigs)

	proc, err := os.StartProcess("/bin/sh", []string{"sh", "-c", "exit 3"}, &os.ProcAttr{})
	if err != nil {
		t.Skip(err)
	}
	s := &sandbox{mainPid: proc.Pid}
	deadline := time.After(5 * time.Second)
	for !s.reap() {
		select {
		case <-sigs:
		case <-deadline:
			t.Fatal("child never reaped")
		}
	}
	if s.code != 3 {
		t.Fatalf("code = %d, want 3", s.code)
	}
}

func TestRunRefusesOutsidePid1(t *testing.T) {
	if code := Run([]string{"true"}); code == 0 {
		t.Fatal("Run outside PID 1 must fail")
	}
}

// startInit re-execs the test binary as PID 1 of a fresh user+pid namespace running Run(args).
func startInit(t *testing.T, l *Link, args ...string) *exec.Cmd {
	t.Helper()
	if err := exec.Command("unshare", "-Urpf", "--kill-child", "true").Run(); err != nil {
		t.Skipf("unshare -Urpf unavailable: %v", err)
	}
	argv := append([]string{"-Urpf", "--kill-child", os.Args[0], "-test.run=^TestHelperInit$", "--"}, args...)
	cmd := exec.Command("unshare", argv...)
	cmd.Env = append(os.Environ(), "CAPSULE_PID1_HELPER=1")
	cmd.Stderr = os.Stderr
	cmd.ExtraFiles = []*os.File{l.Child()}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	l.CloseChild()
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd
}

// The launcher exits at once but an orphan keeps the namespace alive: init must wait for it and still report the launcher's code.
func TestInitOutlivesLauncher(t *testing.T) {
	l := newLink(t)
	started := time.Now()
	cmd := startInit(t, l, "sh", "-c", "sleep 0.5 & exit 5")
	code, ok := l.Wait(nil)
	held := time.Since(started)
	if err := cmd.Wait(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 5 {
			t.Fatalf("unshare: %v", err)
		}
	}
	if !ok || code != 5 {
		t.Fatalf("reported %d, %v; want 5, true", code, ok)
	}
	if held < 500*time.Millisecond {
		t.Fatalf("init returned after %v, before the orphan finished", held)
	}
}

func TestInitStopsOnHostRequest(t *testing.T) {
	cases := map[string]struct {
		argv []string
		want int
	}{
		"plain":           {[]string{"sleep", "30"}, 128 + int(unix.SIGTERM)},
		"ignores SIGTERM": {[]string{"sh", "-c", `trap "" TERM; sleep 30`}, 128 + int(unix.SIGHUP)},
		// SIGCONT releases both pending signals; the kernel delivers the lower-numbered SIGHUP first.
		"stopped by SIGSTOP": {[]string{"sh", "-c", `kill -STOP $$; sleep 30`}, 128 + int(unix.SIGHUP)},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			l := newLink(t)
			cmd := startInit(t, l, c.argv...)
			time.Sleep(300 * time.Millisecond) // let init exec the command
			l.Stop()
			done := make(chan struct{})
			var code int
			var ok bool
			go func() { code, ok = l.Wait(nil); close(done) }()
			select {
			case <-done:
			case <-time.After(shutdownGrace):
				t.Fatal("sandbox did not stop before the SIGKILL grace")
			}
			_ = cmd.Wait()
			if !ok || code != c.want {
				t.Fatalf("reported %d, %v; want %d, true", code, ok, c.want)
			}
		})
	}
}
