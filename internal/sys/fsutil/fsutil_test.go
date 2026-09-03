package fsutil

import (
	"os"
	"os/user"
	"testing"
)

func TestExpandHomeIn(t *testing.T) {
	tests := []struct{ in, want string }{
		{"~", "/h"},
		{"~/x/y", "/h/x/y"},
		{"~x", "~x"},
		{"/abs", "/abs"},
		{"rel", "rel"},
	}
	for _, tc := range tests {
		if got := ExpandHomeIn(tc.in, "/h"); got != tc.want {
			t.Errorf("ExpandHomeIn(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestSudoUserHome(t *testing.T) {
	t.Setenv("SUDO_USER", "")
	if got := SudoUserHome(); got != "" {
		t.Errorf("no SUDO_USER: got %q", got)
	}
	t.Setenv("SUDO_USER", "capsule-no-such-user-42")
	if got := SudoUserHome(); got != "" {
		t.Errorf("unknown SUDO_USER: got %q", got)
	}
	me, err := user.Current()
	if err != nil {
		t.Skip("user.Current:", err)
	}
	if _, err := user.Lookup(me.Username); err != nil {
		t.Skipf("current user %q has no passwd entry: %v", me.Username, err)
	}
	t.Setenv("SUDO_USER", me.Username)
	if got := SudoUserHome(); got != me.HomeDir {
		t.Errorf("SUDO_USER=%s: got %q, want %q", me.Username, got, me.HomeDir)
	}
}

func TestInvokingHome(t *testing.T) {
	t.Setenv("SUDO_USER", "capsule-no-such-user-42")
	want, err := os.UserHomeDir()
	if err != nil {
		t.Skip("UserHomeDir:", err)
	}
	got, err := InvokingHome()
	if err != nil || got != want {
		t.Errorf("InvokingHome() = %q, %v; want %q", got, err, want)
	}
}
