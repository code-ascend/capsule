package mount

import (
	"os"
	"path/filepath"
	"testing"

	"capsule/internal/runtime/bundle"
)

func TestParseSquashFuse(t *testing.T) {
	for in, want := range map[string]SquashFuse{"": SquashFuseAuto, "auto": SquashFuseAuto, "3": SquashFuse3, "ll": SquashFuseLL} {
		got, err := ParseSquashFuse(in)
		if err != nil || got != want {
			t.Errorf("ParseSquashFuse(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	for _, in := range []string{"l1", "squashfuse3", "LL"} {
		if _, err := ParseSquashFuse(in); err == nil {
			t.Errorf("ParseSquashFuse(%q): want error", in)
		}
	}
}

func fakeBundle(t *testing.T, bins ...string) *bundle.Extractor {
	t.Helper()
	b := bundle.New(t.TempDir())
	for _, name := range bins {
		if err := os.WriteFile(filepath.Join(b.Dir, name), []byte("#!/bin/sh\n"), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	return b
}

func TestPickSquashFuse(t *testing.T) {
	tests := []struct {
		name string
		bins []string
		pref SquashFuse
		want string
	}{
		{"auto prefers 3", []string{binSquashfuse3, binSquashfuseLL}, SquashFuseAuto, binSquashfuse3},
		{"auto falls back to ll", []string{binSquashfuseLL}, SquashFuseAuto, binSquashfuseLL},
		{"auto last resort", nil, SquashFuseAuto, binSquashfuse},
		{"ll honoured", []string{binSquashfuse3, binSquashfuseLL}, SquashFuseLL, binSquashfuseLL},
		{"ll missing falls back", []string{binSquashfuse3}, SquashFuseLL, binSquashfuse3},
		{"3 honoured", []string{binSquashfuse3, binSquashfuseLL}, SquashFuse3, binSquashfuse3},
		{"3 missing falls back", []string{binSquashfuseLL}, SquashFuse3, binSquashfuseLL},
	}
	for _, tc := range tests {
		if got := pickSquashFuse(fakeBundle(t, tc.bins...), tc.pref); got != tc.want {
			t.Errorf("%s: got %q, want %q", tc.name, got, tc.want)
		}
	}
}
