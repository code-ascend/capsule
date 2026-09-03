package supervisor

import (
	"bufio"
	"errors"
	"flag"
	"os"
	"os/exec"
	"os/signal"
	"slices"
	"testing"
	"time"

	"golang.org/x/sys/unix"
)

// TestHelperSupervisor is the re-exec target: it runs Run on the command the test passes after "--".
func TestHelperSupervisor(t *testing.T) {
	if os.Getenv("CAPSULE_SUPERVISOR_HELPER") == "" {
		t.Skip("helper only")
	}
	os.Exit(Run(flag.Args()))
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

func TestDescendantsFollowsTheTree(t *testing.T) {
	// sh -> two sleeps; a sibling sleep started by the test must not appear below sh.
	tree := exec.Command("sh", "-c", "sleep 30 & sleep 30 & echo; wait")
	out, err := tree.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := tree.Start(); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = tree.Process.Kill(); _, _ = tree.Process.Wait() })
	sibling := exec.Command("sleep", "30")
	if err := sibling.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sibling.Process.Kill(); _, _ = sibling.Process.Wait() })
	if _, err := bufio.NewReader(out).ReadString('\n'); err != nil { // echo runs after both sleeps forked
		t.Fatal(err)
	}

	got := descendants(tree.Process.Pid)
	if len(got) != 2 {
		t.Fatalf("descendants(sh) = %v, want the two sleeps", got)
	}
	for _, pid := range got {
		if pid == sibling.Process.Pid || pid == os.Getpid() {
			t.Fatalf("descendants(sh) = %v includes a process outside the tree", got)
		}
	}
	all := descendants(os.Getpid())
	for _, want := range []int{tree.Process.Pid, sibling.Process.Pid, got[0], got[1]} {
		if !slices.Contains(all, want) {
			t.Fatalf("descendants(self) = %v lacks %d", all, want)
		}
	}
	if slices.Contains(all, 1) {
		t.Fatalf("descendants(self) = %v must not reach outside our subtree", all)
	}
}

func TestParentOf(t *testing.T) {
	ppid, ok := parentOf(os.Getpid())
	if !ok || ppid != os.Getppid() {
		t.Fatalf("parentOf(self) = %d, %v; want %d", ppid, ok, os.Getppid())
	}
	if _, ok := parentOf(0); ok {
		t.Fatal("parentOf(0) must fail: no such /proc entry")
	}
}

// startSupervisor re-execs the test binary as the supervisor of args run under the host bwrap, as in production:
// sharing the test's PID namespace (shared mode) or with its own (isolated mode). Skips when bwrap is unavailable.
func startSupervisor(t *testing.T, pidns bool, args ...string) *exec.Cmd {
	t.Helper()
	bwrap, err := exec.LookPath("bwrap")
	if err != nil {
		t.Skipf("host bwrap unavailable: %v", err)
	}
	bw := []string{bwrap, "--ro-bind", "/", "/", "--proc", "/proc", "--dev", "/dev"}
	if pidns {
		bw = append(bw, "--unshare-pid")
	}
	if err := exec.Command(bw[0], append(bw[1:], "true")...).Run(); err != nil {
		t.Skipf("host bwrap unusable: %v", err)
	}
	args = append(bw, args...)
	cmd := exec.Command(os.Args[0], append([]string{"-test.run=^TestHelperSupervisor$", "--"}, args...)...)
	cmd.Env = append(os.Environ(), "CAPSULE_SUPERVISOR_HELPER=1")
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	return cmd
}

// waitCode waits for the supervisor within the SIGKILL grace and returns its exit code.
func waitCode(t *testing.T, cmd *exec.Cmd) int {
	t.Helper()
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		var exitErr *exec.ExitError
		if err == nil {
			return 0
		}
		if !errors.As(err, &exitErr) {
			t.Fatalf("supervisor: %v", err)
		}
		return exitErr.ExitCode()
	case <-time.After(shutdownGrace + time.Second):
		t.Fatal("sandbox did not stop before the SIGKILL grace")
	}
	return -1
}

// modes runs fn once with the sandbox in the test's PID namespace and once in its own.
func modes(t *testing.T, fn func(t *testing.T, pidns bool)) {
	t.Run("shared", func(t *testing.T) { fn(t, false) })
	t.Run("pidns", func(t *testing.T) { fn(t, true) })
}

// The launcher exits at once but an orphan keeps the sandbox alive: the supervisor must wait for it and still exit with the launcher's code.
func TestOutlivesLauncher(t *testing.T) {
	modes(t, func(t *testing.T, pidns bool) {
		started := time.Now()
		cmd := startSupervisor(t, pidns, "sh", "-c", "sleep 0.5 & exit 5")
		code := waitCode(t, cmd)
		held := time.Since(started)
		if code != 5 {
			t.Fatalf("exit code %d, want 5", code)
		}
		if held < 500*time.Millisecond {
			t.Fatalf("supervisor returned after %v, before the orphan finished", held)
		}
	})
}

func TestStopsOnSIGTERM(t *testing.T) {
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
			modes(t, func(t *testing.T, pidns bool) {
				cmd := startSupervisor(t, pidns, c.argv...)
				time.Sleep(300 * time.Millisecond) // let the sandbox exec the command
				if err := cmd.Process.Signal(unix.SIGTERM); err != nil {
					t.Fatal(err)
				}
				if code := waitCode(t, cmd); code != c.want {
					t.Fatalf("exit code %d, want %d", code, c.want)
				}
			})
		})
	}
}

// The supervisor shares the PID namespace with the test: shutdown must reach only its own subtree.
func TestStopSparesOutsiders(t *testing.T) {
	outsider := exec.Command("sleep", "30")
	if err := outsider.Start(); err != nil {
		t.Skip(err)
	}
	t.Cleanup(func() { _ = outsider.Process.Kill(); _, _ = outsider.Process.Wait() })

	// The launcher exits; its orphaned sleep is reparented to the supervisor and must still be found and killed.
	cmd := startSupervisor(t, false, "sh", "-c", "sleep 30 & exit 0")
	time.Sleep(300 * time.Millisecond)
	if err := cmd.Process.Signal(unix.SIGTERM); err != nil {
		t.Fatal(err)
	}
	if code := waitCode(t, cmd); code != 0 {
		t.Fatalf("exit code %d, want the launcher's 0", code)
	}
	if err := unix.Kill(outsider.Process.Pid, 0); err != nil {
		t.Fatalf("outsider %d is gone: %v", outsider.Process.Pid, err)
	}
}
