package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"capsule/internal/format/binconfig"
)

const sampleDesktop = `[Desktop Entry]
Name=Foo
Exec=foo --bar
Icon=foo
TryExec=/usr/bin/foo
DBusActivatable=true
Type=Application
`

func TestParseFilter(t *testing.T) {
	cases := map[string]Filter{
		"":         FilterAll,
		"all":      FilterAll,
		"apps":     FilterApps,
		"binaries": FilterBinaries,
	}
	for in, want := range cases {
		got, err := ParseFilter(in)
		if err != nil {
			t.Errorf("ParseFilter(%q) err=%v", in, err)
		}
		if got != want {
			t.Errorf("ParseFilter(%q) = %q, want %q", in, got, want)
		}
	}
	if _, err := ParseFilter("garbage"); err == nil {
		t.Error("ParseFilter(garbage) should error")
	}
}

// newTestExporter wires Exporter against a fresh temp HOME.
func newTestExporter(t *testing.T, cfg *binconfig.Config, root string) *Exporter {
	t.Helper()
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_DATA_HOME", "")
	ex, err := New("/cap", cfg, root)
	if err != nil {
		t.Fatal(err)
	}
	return ex
}

func TestNew(t *testing.T) {
	ex := newTestExporter(t, &binconfig.Config{}, "")
	if ex.paths.XDGDataHome == "" || ex.paths.XDGBinHome == "" {
		t.Errorf("paths empty: %+v", ex.paths)
	}
}

func TestAppsTransformsDesktop(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "usr/share/applications"), 0o755); err != nil {
		t.Fatal(err)
	}
	src := filepath.Join(root, "usr/share/applications/foo.desktop")
	if err := os.WriteFile(src, []byte(sampleDesktop), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &binconfig.Config{Apps: []binconfig.AppExport{
		{Desktop: "/usr/share/applications/foo.desktop"},
	}}
	ex := newTestExporter(t, cfg, root)

	if err := ex.Apps(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(ex.paths.XDGDataHome, "applications/foo.desktop")
	got, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read exported: %v", err)
	}
	body := string(got)
	if !strings.Contains(body, "Exec=/cap foo --bar") {
		t.Errorf("Exec not rewritten:\n%s", body)
	}
	if strings.Contains(body, "TryExec=") || strings.Contains(body, "DBusActivatable=true") {
		t.Errorf("TryExec/DBusActivatable not stripped:\n%s", body)
	}
}

func TestBinariesWritesWrapper(t *testing.T) {
	cfg := &binconfig.Config{Binaries: []string{"/usr/bin/foo"}}
	ex := newTestExporter(t, cfg, "")
	if err := ex.Binaries(); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(ex.paths.XDGBinHome, "foo")
	body, err := os.ReadFile(dst)
	if err != nil {
		t.Fatalf("read wrapper: %v", err)
	}
	got := string(body)
	if !strings.Contains(got, `exec "/cap" "/usr/bin/foo"`) {
		t.Errorf("wrapper body:\n%s", got)
	}
	st, _ := os.Stat(dst)
	if st.Mode()&0o111 == 0 {
		t.Errorf("wrapper not executable: %v", st.Mode())
	}
}

func TestBinariesSkipsExisting(t *testing.T) {
	cfg := &binconfig.Config{Binaries: []string{"/usr/bin/foo"}}
	ex := newTestExporter(t, cfg, "")
	dst := filepath.Join(ex.paths.XDGBinHome, "foo")
	if err := os.MkdirAll(ex.paths.XDGBinHome, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("existing"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ex.Binaries(); err != nil {
		t.Fatal(err)
	}
	body, _ := os.ReadFile(dst)
	if string(body) != "existing" {
		t.Errorf("existing wrapper overwritten: %q", body)
	}
}

func TestUnexportBinariesOnlyOurs(t *testing.T) {
	cfg := &binconfig.Config{Binaries: []string{"/usr/bin/foo"}}
	ex := newTestExporter(t, cfg, "")
	if err := os.MkdirAll(ex.paths.XDGBinHome, 0o755); err != nil {
		t.Fatal(err)
	}
	dst := filepath.Join(ex.paths.XDGBinHome, "foo")
	// alien wrapper that doesn't reference our capsulePath
	if err := os.WriteFile(dst, []byte("#!/bin/sh\nexec /other"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ex.UnexportBinaries(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); err != nil {
		t.Errorf("alien wrapper deleted (must be kept): %v", err)
	}

	// our wrapper — must be removed
	ourBody := `#!/bin/sh
exec "/cap" "/usr/bin/foo" "$@"
`
	if err := os.WriteFile(dst, []byte(ourBody), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := ex.UnexportBinaries(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("our wrapper not removed: err=%v", err)
	}
}

func TestUnexportAppsRemovesDesktop(t *testing.T) {
	cfg := &binconfig.Config{Apps: []binconfig.AppExport{
		{Desktop: "/usr/share/applications/foo.desktop"},
	}}
	ex := newTestExporter(t, cfg, "")
	dst := filepath.Join(ex.paths.XDGDataHome, "applications/foo.desktop")
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(dst, []byte("dummy"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ex.UnexportApps(); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(dst); !os.IsNotExist(err) {
		t.Errorf("desktop file not removed: err=%v", err)
	}
}
