package hostexec

import (
	"os"
	"strings"
	"testing"
)

func TestBuildHelloEnvWhitelist(t *testing.T) {
	for _, k := range forwardedEnvKeys {
		_ = os.Unsetenv(k)
	}
	t.Setenv("DISPLAY", ":1")
	t.Setenv("LC_TIME", "en_US.UTF-8")
	t.Setenv("PATH", "/should/not/leak")

	req := buildHello([]string{"firefox"}, false, 0)

	if req.Env["DISPLAY"] != ":1" {
		t.Errorf("DISPLAY missing: %q", req.Env["DISPLAY"])
	}
	if req.Env["LC_TIME"] != "en_US.UTF-8" {
		t.Errorf("LC_TIME missing: %q", req.Env["LC_TIME"])
	}
	if _, leaked := req.Env["PATH"]; leaked {
		t.Errorf("PATH must not be forwarded; got %q", req.Env["PATH"])
	}
}

func TestBuildHelloNonTTY(t *testing.T) {
	req := buildHello([]string{"echo", "hi"}, false, 0)
	if req.Tty {
		t.Error("Tty must be false when useTTY=false")
	}
	if req.Cols != 0 || req.Rows != 0 {
		t.Errorf("dims must be zero in non-TTY: %dx%d", req.Cols, req.Rows)
	}
	if !equal(req.Argv, []string{"echo", "hi"}) {
		t.Errorf("argv = %v", req.Argv)
	}
}

func TestPtyBlocked(t *testing.T) {
	cases := map[string]bool{
		"xdg-open":          true,
		"/usr/bin/xdg-open": true,
		"gio":               true,
		"flatpak":           true,
		"firefox":           false,
		"vim":               false,
		"/bin/echo":         false,
	}
	for cmd, want := range cases {
		if got := ptyBlocked(cmd); got != want {
			t.Errorf("ptyBlocked(%q) = %v, want %v", cmd, got, want)
		}
	}
}

func TestForwardedEnvKeysContainsCriticalGUIVars(t *testing.T) {
	must := []string{"DISPLAY", "WAYLAND_DISPLAY", "XDG_RUNTIME_DIR", "DBUS_SESSION_BUS_ADDRESS"}
	have := strings.Join(forwardedEnvKeys, ",")
	for _, k := range must {
		if !strings.Contains(have, k) {
			t.Errorf("forwardedEnvKeys missing critical var %q", k)
		}
	}
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
