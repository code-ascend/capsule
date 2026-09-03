package lowprio

import (
	"os/exec"
	"strings"
	"testing"
)

func niceOf(t *testing.T, start func(*exec.Cmd) error) string {
	t.Helper()
	cmd := exec.Command("nice")
	out := &strings.Builder{}
	cmd.Stdout = out
	if err := start(cmd); err != nil {
		t.Skip(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	return strings.TrimSpace(out.String())
}

func TestStartLowersIOPriority(t *testing.T) {
	if _, err := exec.LookPath("ionice"); err != nil {
		t.Skip("ionice not installed")
	}
	cmd := exec.Command("sh", "-c", "ionice -p $$")
	out := &strings.Builder{}
	cmd.Stdout = out
	if err := Start(cmd); err != nil {
		t.Skip(err)
	}
	if err := cmd.Wait(); err != nil {
		t.Fatal(err)
	}
	if got := strings.TrimSpace(out.String()); got != "best-effort: prio 7" {
		t.Fatalf("child ioprio = %q, want best-effort prio 7", got)
	}
}

func TestStartLowersChildOnly(t *testing.T) {
	if got := niceOf(t, Start); got != "19" {
		t.Fatalf("child nice = %s, want 19", got)
	}
	// A plain start afterwards must not inherit the lowered priority from a recycled thread.
	if got := niceOf(t, (*exec.Cmd).Start); got == "19" {
		t.Fatal("priority leaked into the process")
	}
}
