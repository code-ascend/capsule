package workspace

import (
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
)

func TestCleanupRunsLIFO(t *testing.T) {
	w := newTestWorkspace(t)
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
	w := newTestWorkspace(t)
	n := 0
	w.AddCleanup(func() error { n++; return nil })
	w.Cleanup()
	w.Cleanup()
	if n != 1 {
		t.Errorf("expected 1 invocation, got %d", n)
	}
}

func TestCleanupContinuesOnError(t *testing.T) {
	w := newTestWorkspace(t)
	called := 0
	w.AddCleanup(func() error { called++; return nil })
	w.AddCleanup(func() error { called++; return errBoom })
	w.AddCleanup(func() error { called++; return nil })
	w.Cleanup()
	if called != 3 {
		t.Errorf("expected all 3 cleanups to run despite error, got %d", called)
	}
}

func TestCleanupRunsWhenLastSession(t *testing.T) {
	w := newTestWorkspace(t)
	called := 0
	w.AddCleanup(func() error { called++; return nil })
	w.Cleanup()
	if called != 1 {
		t.Errorf("expected callback once for last session, got %d", called)
	}
	if _, err := os.Stat(w.Dir); !os.IsNotExist(err) {
		t.Errorf("expected workspace dir gone, got err=%v", err)
	}
}

func TestCleanupSkipsCallbacksWhenOtherSessionAlive(t *testing.T) {
	w := newTestWorkspace(t)
	// Plant a sentinel for PID 1 (init — guaranteed alive on Linux).
	other := filepath.Join(w.Dir, "sessions", "1")
	if err := os.WriteFile(other, nil, 0600); err != nil {
		t.Fatal(err)
	}
	called := 0
	w.AddCleanup(func() error { called++; return nil })
	w.Cleanup()
	if called != 0 {
		t.Errorf("expected callback skipped while peer is alive, got called=%d", called)
	}
	if _, err := os.Stat(w.Dir); err != nil {
		t.Errorf("expected workspace dir kept while peer is alive, got err=%v", err)
	}
}

func TestNewSweepsStaleSentinels(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tmp)

	capsulePath := filepath.Join(t.TempDir(), "fake-capsule")
	if err := os.WriteFile(capsulePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}

	// Create a workspace, then plant a stale sentinel (huge pid that doesn't exist),
	// and verify a fresh New() prunes it.
	w1, err := New(capsulePath)
	if err != nil {
		t.Fatal(err)
	}
	stalePID := "999999"
	if _, err := os.Stat("/proc/" + stalePID); err == nil {
		t.Skip("PID 999999 actually exists; pick another")
	}
	stale := filepath.Join(w1.Dir, "sessions", stalePID)
	if err := os.WriteFile(stale, nil, 0600); err != nil {
		t.Fatal(err)
	}
	w2, err := New(capsulePath)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(stale); !os.IsNotExist(err) {
		t.Errorf("expected stale sentinel pruned by New(), got err=%v", err)
	}
	w1.Cleanup()
	w2.Cleanup()
}

func TestSameCapsuleShareDir(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("XDG_RUNTIME_DIR", tmp)

	capsulePath := filepath.Join(t.TempDir(), "shared-capsule")
	if err := os.WriteFile(capsulePath, []byte("x"), 0644); err != nil {
		t.Fatal(err)
	}
	a, err := New(capsulePath)
	if err != nil {
		t.Fatal(err)
	}
	b, err := New(capsulePath)
	if err != nil {
		t.Fatal(err)
	}
	if a.Dir != b.Dir {
		t.Errorf("expected same Dir for same capsule, got %q vs %q", a.Dir, b.Dir)
	}
	a.Cleanup()
	b.Cleanup()
}

// newTestWorkspace returns a Workspace rooted in a fresh temp dir with a
// session sentinel for the current pid. Suitable for unit tests that don't
// involve real capsule binaries.
func newTestWorkspace(t *testing.T) *Workspace {
	t.Helper()
	tmp := t.TempDir()
	sessionsDir := filepath.Join(tmp, "sessions")
	if err := os.MkdirAll(sessionsDir, 0700); err != nil {
		t.Fatal(err)
	}
	sessionFile := filepath.Join(sessionsDir, strconv.Itoa(os.Getpid()))
	if err := os.WriteFile(sessionFile, nil, 0600); err != nil {
		t.Fatal(err)
	}
	return &Workspace{Dir: tmp, sessionFile: sessionFile}
}

var errBoom = stringError("boom")

type stringError string

func (s stringError) Error() string { return string(s) }
