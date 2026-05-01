package reaper

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

func TestReadPPidMatchesProc(t *testing.T) {
	got := readPPid(os.Getpid())
	if got != os.Getppid() {
		t.Fatalf("readPPid(self) = %d, want %d", got, os.Getppid())
	}
}

func TestReadPPidMissing(t *testing.T) {
	if got := readPPid(1<<30 - 1); got != 0 {
		t.Fatalf("readPPid(huge) = %d, want 0", got)
	}
}

func TestDescendantsFindsLiveChild(t *testing.T) {
	cmd := spawn(t, "sleep", "5")
	waitFor(t, time.Second, func() bool {
		return slices.Contains(descendants(os.Getpid()), cmd.Process.Pid)
	}, "descendants did not report child pid")
}

func TestDescendantsFindsGrandchild(t *testing.T) {
	pidFile := filepath.Join(t.TempDir(), "pid")
	cmd := spawn(t, "sh", "-c", "sleep 5 & echo $! > "+pidFile+"; wait")
	defer func() { _ = syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL) }()

	var grandPID int
	waitFor(t, 2*time.Second, func() bool {
		data, _ := os.ReadFile(pidFile)
		pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
		if err != nil || pid <= 0 {
			return false
		}
		grandPID = pid
		return true
	}, "grandchild pid file never written")

	if !slices.Contains(descendants(os.Getpid()), grandPID) {
		t.Fatalf("descendants missed grandchild pid %d", grandPID)
	}
}

func TestNewSnapshotsIdentity(t *testing.T) {
	r := New(time.Second)
	if r.selfPid != os.Getpid() {
		t.Fatalf("selfPid = %d, want %d", r.selfPid, os.Getpid())
	}
	if r.selfNS == "" {
		t.Fatalf("selfNS empty; expected /proc/self/ns/mnt readable")
	}
	if r.grace != time.Second {
		t.Fatalf("grace = %v, want 1s", r.grace)
	}
}

// In-capsule filtering: processes in our mount ns must be excluded.
// Without CAP_SYS_ADMIN we can't spawn a real different-ns process here,
// so we verify the negative case — same-ns child is filtered out.
func TestInCapsuleFiltersSameNS(t *testing.T) {
	cmd := spawn(t, "sleep", "5")
	r := New(time.Second)
	waitFor(t, time.Second, func() bool {
		return slices.Contains(descendants(os.Getpid()), cmd.Process.Pid)
	}, "child not visible via /proc")

	if slices.Contains(r.inCapsule(), cmd.Process.Pid) {
		t.Fatalf("inCapsule() included same-ns pid %d", cmd.Process.Pid)
	}
}

func TestWaitReturnsWhenNoDescendants(t *testing.T) {
	done := make(chan struct{})
	go func() {
		New(time.Second).Wait(context.Background())
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Wait blocked despite empty descendant set")
	}
}

// spawn starts cmd, registers cleanup, and returns it. Skips on exec failure.
func spawn(t *testing.T, name string, args ...string) *exec.Cmd {
	t.Helper()
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn %s: %v", name, err)
	}
	t.Cleanup(func() {
		_ = cmd.Process.Kill()
		_, _ = cmd.Process.Wait()
	})
	return cmd
}

func waitFor(t *testing.T, max time.Duration, cond func() bool, msg string) {
	t.Helper()
	deadline := time.Now().Add(max)
	for time.Now().Before(deadline) {
		if cond() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal(msg)
}
