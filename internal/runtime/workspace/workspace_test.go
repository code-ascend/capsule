package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"syscall"
	"testing"
)

func TestCleanupRunsLIFO(t *testing.T) {
	w := newTestWS(t, t.TempDir())
	var order []int
	w.AddCleanup(func() error { order = append(order, 1); return nil })
	w.AddCleanup(func() error { order = append(order, 2); return nil })
	w.AddCleanup(func() error { order = append(order, 3); return nil })
	w.Cleanup()
	if !slices.Equal(order, []int{3, 2, 1}) {
		t.Errorf("expected LIFO [3 2 1], got %v", order)
	}
}

func TestCleanupRunsOnce(t *testing.T) {
	w := newTestWS(t, t.TempDir())
	n := 0
	w.AddCleanup(func() error { n++; return nil })
	w.Cleanup()
	w.Cleanup()
	if n != 1 {
		t.Errorf("expected 1 invocation, got %d", n)
	}
}

func TestCleanupContinuesOnError(t *testing.T) {
	w := newTestWS(t, t.TempDir())
	var seen []string
	w.AddCleanup(func() error { seen = append(seen, "first"); return nil })
	w.AddCleanup(func() error { seen = append(seen, "boom"); return errBoom })
	w.AddCleanup(func() error { seen = append(seen, "third"); return nil })
	w.Cleanup()
	want := []string{"third", "boom", "first"}
	if !slices.Equal(seen, want) {
		t.Errorf("expected %v, got %v", want, seen)
	}
}

// TestPeerCoordination spawns a goroutine peer holding LOCK_SH on the same path.
func TestPeerCoordination(t *testing.T) {
	tmp := t.TempDir()
	w := newTestWS(t, tmp)

	peerReady := make(chan struct{})
	peerRelease := make(chan struct{})
	peerDone := make(chan struct{})
	go func() {
		defer close(peerDone)
		f, err := os.OpenFile(filepath.Join(tmp, "ws.lock"), os.O_RDWR, 0o600)
		if err != nil {
			t.Errorf("peer open: %v", err)
			close(peerReady)
			return
		}
		defer func() { _ = f.Close() }()
		if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
			t.Errorf("peer flock: %v", err)
			close(peerReady)
			return
		}
		close(peerReady)
		<-peerRelease
	}()
	<-peerReady

	if w.LastSession() {
		t.Fatal("peer alive but LastSession=true")
	}

	close(peerRelease)
	<-peerDone

	if !w.LastSession() {
		t.Fatal("peer gone but LastSession=false")
	}
	w.Cleanup()
	if _, err := os.Stat(w.Dir); !os.IsNotExist(err) {
		t.Errorf("dir must be removed by last session, got err=%v", err)
	}
}

// TestSetupLockExcludesPeers verifies setup lock blocks an independent FD's LOCK_EX.
func TestSetupLockExcludesPeers(t *testing.T) {
	w := newTestWS(t, t.TempDir())
	gotInside := make(chan struct{})
	release := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- w.WithSetupLock(func() error {
			close(gotInside)
			<-release
			return nil
		})
	}()
	<-gotInside

	f, err := os.OpenFile(w.setupPath, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = f.Close() }()
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err == nil {
		_ = syscall.Flock(int(f.Fd()), syscall.LOCK_UN)
		t.Fatal("expected setup lock held; got LOCK_EX|NB success")
	}

	close(release)
	if err := <-done; err != nil {
		t.Fatalf("WithSetupLock returned: %v", err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		t.Fatalf("expected setup lock free after closure: %v", err)
	}
}

// newTestWS opens a Workspace at <tmp>/ws with LOCK_SH held; multiple calls = peers.
func newTestWS(t *testing.T, tmp string) *Workspace {
	t.Helper()
	base := filepath.Join(tmp, "ws")
	if err := os.MkdirAll(base, 0o700); err != nil {
		t.Fatal(err)
	}
	f, err := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		_ = f.Close()
		t.Fatal(err)
	}
	return &Workspace{Dir: base, setupPath: base + ".setup.lock", aliveFD: f}
}

var errBoom = stringError("boom")

type stringError string

func (s stringError) Error() string { return string(s) }
