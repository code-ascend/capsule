package reaper

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/unix"
)

func TestIsZombieSelf(t *testing.T) {
	if isZombie(os.Getpid()) {
		t.Fatal("isZombie(self) = true")
	}
}

func TestIsZombieMissing(t *testing.T) {
	if isZombie(1<<30 - 1) {
		t.Fatal("isZombie(huge) = true")
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

// A child started but never waited on becomes a zombie, like an adopted orphan that exited.
func zombieChild(t *testing.T) *exec.Cmd {
	t.Helper()
	cmd := exec.Command("true")
	if err := cmd.Start(); err != nil {
		t.Skipf("cannot spawn true: %v", err)
	}
	waitFor(t, time.Second, func() bool {
		return isZombie(cmd.Process.Pid)
	}, "child never became a zombie")
	return cmd
}

func TestReapOrphansNeedsTwoPolls(t *testing.T) {
	cmd := zombieChild(t)
	r := New(time.Second)

	r.reapOrphans()
	if !isZombie(cmd.Process.Pid) {
		t.Fatal("zombie reaped on first poll; must survive one tick")
	}
	r.reapOrphans()
	if _, err := os.Stat(filepath.Join("/proc", strconv.Itoa(cmd.Process.Pid))); err == nil {
		t.Fatal("zombie still present after second poll")
	}
	if err := cmd.Wait(); err == nil {
		t.Fatal("Wait succeeded; reaper should have consumed the status")
	}
}

func TestZombieChildrenListsOnlyZombies(t *testing.T) {
	zombie := zombieChild(t)
	live := spawn(t, "sleep", "5")
	got := zombieChildren(os.Getpid())
	if !slices.Contains(got, zombie.Process.Pid) {
		t.Errorf("zombie %d missing from %v", zombie.Process.Pid, got)
	}
	if slices.Contains(got, live.Process.Pid) {
		t.Errorf("live child %d listed as zombie", live.Process.Pid)
	}
	_, _ = zombie.Process.Wait()
}

// Children waited on by os/exec must keep their exit status while drain polls.
func TestDrainDoesNotStealExecStatus(t *testing.T) {
	r := New(time.Second)
	r.idlePoll = 25 * time.Millisecond
	stop := make(chan struct{})
	holder := spawn(t, "sleep", "5") // keeps drain from returning early
	_ = holder
	// drain never sees an in-capsule descendant here, so run reapOrphans directly at poll rate.
	go func() {
		tick := time.NewTicker(r.idlePoll)
		defer tick.Stop()
		for {
			select {
			case <-stop:
				return
			case <-tick.C:
				r.reapOrphans()
			}
		}
	}()
	defer close(stop)

	for range 40 {
		err := exec.Command("sh", "-c", "exit 3").Run()
		var ee *exec.ExitError
		if !errors.As(err, &ee) || ee.ExitCode() != 3 {
			t.Fatalf("exit status lost: %v", err)
		}
	}
}

func TestChildrenListsDirectChild(t *testing.T) {
	child := spawn(t, "sleep", "5")
	if got := children(os.Getpid()); !slices.Contains(got, child.Process.Pid) {
		t.Fatalf("children missed %d: %v", child.Process.Pid, got)
	}
}

func TestEnableSubReaper(t *testing.T) {
	if err := EnableSubReaper(); err != nil {
		t.Fatal(err)
	}
	var v int32
	err := unix.Prctl(unix.PR_GET_CHILD_SUBREAPER, uintptr(unsafe.Pointer(&v)), 0, 0, 0)
	if err != nil || v != 1 {
		t.Fatalf("PR_GET_CHILD_SUBREAPER = %d, %v; want 1", v, err)
	}
}

// TestHelperNonDumpable runs as a re-exec'd child with unreadable /proc/pid/ns.
func TestHelperNonDumpable(t *testing.T) {
	if os.Getenv("REAPER_HELPER") != "nondumpable" {
		t.Skip("helper only")
	}
	if err := unix.Prctl(unix.PR_SET_DUMPABLE, 0, 0, 0, 0); err != nil {
		os.Exit(3)
	}
	fmt.Println("ready")
	time.Sleep(10 * time.Second)
}

func TestInCapsuleFailClosed(t *testing.T) {
	cmd := exec.Command(os.Args[0], "-test.run=^TestHelperNonDumpable$")
	cmd.Env = append(os.Environ(), "REAPER_HELPER=nondumpable")
	out, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cmd.Process.Kill(); _, _ = cmd.Process.Wait() })
	line, err := bufio.NewReader(out).ReadString('\n')
	if err != nil || line != "ready\n" {
		t.Fatalf("helper did not report ready: %q, %v", line, err)
	}
	pid := cmd.Process.Pid
	if _, err := readMountNS(pid); err == nil {
		t.Skip("ns readable despite non-dumpable child (CAP_SYS_PTRACE)")
	}
	r := New(time.Second)
	if !slices.Contains(r.inCapsule(), pid) {
		t.Fatalf("non-dumpable descendant %d must count as in-capsule", pid)
	}
}

func TestReapOrphansKeepsPidAfterFailedWait(t *testing.T) {
	cmd := zombieChild(t)
	pid := cmd.Process.Pid
	r := New(time.Second)
	r.reapOrphans() // first sighting
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	r.reapOrphans()
	if r.zombies[pid] {
		t.Fatalf("reaped pid %d still tracked", pid)
	}
	z := zombieChild(t)
	r.zombies = map[int]bool{z.Process.Pid: true}
	if _, err := z.Process.Wait(); err != nil {
		t.Fatal(err)
	}
	r.reapOrphans()
	if r.zombies[z.Process.Pid] {
		t.Fatal("vanished pid must not stay tracked")
	}
}
