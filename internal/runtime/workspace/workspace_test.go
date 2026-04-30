package workspace

import (
	"slices"
	"testing"
)

func TestCleanupRunsLIFO(t *testing.T) {
	w := &Workspace{}
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
	w := &Workspace{}
	n := 0
	w.AddCleanup(func() error { n++; return nil })
	w.Cleanup()
	w.Cleanup()
	if n != 1 {
		t.Errorf("expected 1 invocation, got %d", n)
	}
}

func TestCleanupContinuesOnError(t *testing.T) {
	w := &Workspace{}
	called := 0
	w.AddCleanup(func() error { called++; return nil })
	w.AddCleanup(func() error { called++; return errBoom })
	w.AddCleanup(func() error { called++; return nil })
	w.Cleanup()
	if called != 3 {
		t.Errorf("expected all 3 cleanups to run despite error, got %d", called)
	}
}

var errBoom = stringError("boom")

type stringError string

func (s stringError) Error() string { return string(s) }
