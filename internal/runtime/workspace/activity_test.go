package workspace

import (
	"os"
	"slices"
	"testing"
)

func TestActiveAndHolders(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", t.TempDir())
	capsule := "/fake/capsule/binary"

	active, err := Active(capsule)
	if err != nil {
		t.Fatalf("Active before lock: %v", err)
	}
	if active {
		t.Fatal("Active=true before any session")
	}

	ws, err := New(capsule)
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	active, err = Active(capsule)
	if err != nil {
		t.Fatalf("Active with lock: %v", err)
	}
	if !active {
		t.Fatal("Active=false while session holds the lock")
	}

	pids, err := Holders(capsule)
	if err != nil {
		t.Fatalf("Holders: %v", err)
	}
	if !slices.Contains(pids, os.Getpid()) {
		t.Errorf("Holders=%v, want to contain %d", pids, os.Getpid())
	}

	ws.Cleanup()
	active, err = Active(capsule)
	if err != nil {
		t.Fatalf("Active after cleanup: %v", err)
	}
	if active {
		t.Fatal("Active=true after last session cleanup")
	}
}
