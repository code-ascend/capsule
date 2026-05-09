package bwrap

import (
	"strings"
	"testing"

	"capsule/internal/format/binconfig"
)

func buildJoined(s *Spec) string {
	return strings.Join(s.Build(), " ")
}

func TestBuildReadOnlyRoot(t *testing.T) {
	got := buildJoined(&Spec{
		RootPath: "/mnt/squashfs",
		Cfg:      &binconfig.Config{},
		Cmd:      []string{"/bin/ls"},
	})
	if !strings.Contains(got, "--ro-bind /mnt/squashfs /") {
		t.Fatalf("expected ro-bind root, got: %s", got)
	}
}

func TestBuildWritableRoot(t *testing.T) {
	got := buildJoined(&Spec{
		RootPath:     "/var/overlay/merged",
		RootWritable: true,
		Cfg:          &binconfig.Config{},
		Cmd:          []string{"/bin/bash"},
	})
	if !strings.Contains(got, "--bind /var/overlay/merged /") {
		t.Fatalf("expected rw bind root, got: %s", got)
	}
}

func TestBuildLaunchFallback(t *testing.T) {
	args := (&Spec{
		RootPath: "/mnt",
		Cfg:      &binconfig.Config{Launch: "/usr/bin/foo bar baz"},
	}).Build()
	tail := args[len(args)-3:]
	want := []string{"/usr/bin/foo", "bar", "baz"}
	for i, w := range want {
		if tail[i] != w {
			t.Fatalf("expected launch tail %v, got %v", want, tail)
		}
	}
}

func TestBuildDefaultBash(t *testing.T) {
	args := (&Spec{
		RootPath: "/mnt",
		Cfg:      &binconfig.Config{},
	}).Build()
	if args[len(args)-1] != "/bin/bash" {
		t.Fatalf("expected /bin/bash fallback, got %s", args[len(args)-1])
	}
}

func TestBuildEnvSetSorted(t *testing.T) {
	got := buildJoined(&Spec{
		RootPath: "/mnt",
		Cfg: &binconfig.Config{
			EnvSet: map[string]string{
				"BBB": "2",
				"AAA": "1",
				"CCC": "3",
			},
		},
	})
	a := strings.Index(got, "--setenv AAA")
	b := strings.Index(got, "--setenv BBB")
	c := strings.Index(got, "--setenv CCC")
	if a < 0 || b < 0 || c < 0 || !(a < b && b < c) {
		t.Fatalf("env order not stable: AAA=%d BBB=%d CCC=%d", a, b, c)
	}
}

func TestBuildEnvUnset(t *testing.T) {
	got := buildJoined(&Spec{
		RootPath: "/mnt",
		Cfg:      &binconfig.Config{EnvUnset: []string{"LD_PRELOAD"}},
	})
	if !strings.Contains(got, "--unsetenv LD_PRELOAD") {
		t.Fatalf("missing unsetenv: %s", got)
	}
}

func TestSpecBinds(t *testing.T) {
	got := buildJoined(&Spec{
		RootPath: "/mnt",
		Cfg:      &binconfig.Config{},
		Binds:    []string{"/host/foo:/cont/foo", "/data", "/x:/y"},
	})
	for _, want := range []string{
		"--bind /host/foo /cont/foo",
		"--bind /data /data",
		"--bind /x /y",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestHostExecArgs(t *testing.T) {
	got := buildJoined(&Spec{
		RootPath:        "/mnt",
		Cfg:             &binconfig.Config{},
		HostExecSocket:  "@capsule-host-exec-1234-abcd",
		HostExecBinPath: "/var/home/dm/.local/bin/arch_test",
	})
	wants := []string{
		"--ro-bind /var/home/dm/.local/bin/arch_test /usr/local/bin/capsule-host-exec",
		"--ro-bind /var/home/dm/.local/bin/arch_test /usr/local/bin/xdg-open",
		"--ro-bind /var/home/dm/.local/bin/arch_test /usr/local/bin/gio",
		"--ro-bind /var/home/dm/.local/bin/arch_test /usr/local/bin/flatpak",
		"--setenv CAPSULE_HOST_SOCKET @capsule-host-exec-1234-abcd",
	}
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("missing %q in %q", want, got)
		}
	}
}

func TestHostExecArgsDisabledWithoutFields(t *testing.T) {
	got := buildJoined(&Spec{
		RootPath: "/mnt",
		Cfg:      &binconfig.Config{},
	})
	for _, banned := range []string{
		"capsule-host-exec",
		"/usr/local/bin/xdg-open",
		"/usr/local/bin/gio",
		"/usr/local/bin/flatpak",
		"CAPSULE_HOST_SOCKET",
	} {
		if strings.Contains(got, banned) {
			t.Errorf("unexpected %q in disabled spec: %q", banned, got)
		}
	}
}

func TestParentDirArgs(t *testing.T) {
	cases := map[string][]string{
		"/var/home/dm":  {"--dir", "/var", "--dir", "/var/home"},
		"/home/foo":     {"--dir", "/home"},
		"/srv/users/dm": {"--dir", "/srv", "--dir", "/srv/users"},
		"/foo":          nil,
		"/":             nil,
	}
	for in, want := range cases {
		got := parentDirArgs(in)
		if strings.Join(got, " ") != strings.Join(want, " ") {
			t.Errorf("parentDirArgs(%q) = %v, want %v", in, got, want)
		}
	}
}

func TestTopComponent(t *testing.T) {
	cases := map[string]string{
		"/home/foo":         "/home",
		"/var/home/foo":     "/var",
		"/Users/foo":        "/Users",
		"/home/foo/sub/dir": "/home",
	}
	for in, want := range cases {
		got := topComponent(in)
		if got != want {
			t.Errorf("topComponent(%q) = %q, want %q", in, got, want)
		}
	}
}
