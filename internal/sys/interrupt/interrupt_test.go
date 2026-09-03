package interrupt

import (
	"context"
	"os"
	"syscall"
	"testing"
	"time"
)

func interruptSelf(t *testing.T) {
	t.Helper()
	if err := syscall.Kill(os.Getpid(), syscall.SIGINT); err != nil {
		t.Fatal(err)
	}
}

func cancelled(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	case <-time.After(2 * time.Second):
		return false
	}
}

func TestContextCancelsOnSIGINT(t *testing.T) {
	ctx, cancel := Context(context.Background())
	defer cancel()
	interruptSelf(t)
	if !cancelled(ctx) {
		t.Fatal("SIGINT did not cancel the context")
	}
}

func TestLendKeepsContextAlive(t *testing.T) {
	ctx, cancel := Context(context.Background())
	defer cancel()
	restore := Lend()
	interruptSelf(t) // ignored: the test process must survive and ctx must stay open
	if err := syscall.Kill(os.Getpid(), syscall.SIGQUIT); err != nil {
		t.Fatal(err)
	}
	select {
	case <-ctx.Done():
		t.Fatal("SIGINT cancelled the context while lent")
	case <-time.After(200 * time.Millisecond):
	}
	restore()
	interruptSelf(t)
	if !cancelled(ctx) {
		t.Fatal("SIGINT did not cancel the context after restore")
	}
}
