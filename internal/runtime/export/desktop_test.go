package export

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name, body string) string {
	t.Helper()
	p := filepath.Join(dir, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestTransformDesktopRewritesExec(t *testing.T) {
	src := writeFile(t, t.TempDir(), "foo.desktop",
		`[Desktop Entry]
Name=Foo
Exec=foo --bar baz
TryExec=/usr/bin/foo
Icon=foo
`)
	dst := filepath.Join(t.TempDir(), "foo.desktop")
	if err := transformDesktop(src, dst, "/path/to/capsule", "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	body := string(got)

	if !strings.Contains(body, "Exec=/path/to/capsule foo --bar baz") {
		t.Errorf("Exec= not rewritten:\n%s", body)
	}
	if strings.Contains(body, "TryExec=") {
		t.Errorf("TryExec should be dropped:\n%s", body)
	}
	if !strings.Contains(body, "Icon=foo") {
		t.Errorf("Icon should be preserved when no override:\n%s", body)
	}
}

func TestTransformDesktopIconOverride(t *testing.T) {
	src := writeFile(t, t.TempDir(), "x.desktop",
		"[Desktop Entry]\nName=X\nExec=x\nIcon=x\n")
	dst := filepath.Join(t.TempDir(), "x.desktop")
	if err := transformDesktop(src, dst, "/cap", "myicon", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !strings.Contains(string(got), "Icon=myicon") {
		t.Errorf("override missed:\n%s", got)
	}
}

func TestTransformDesktopNameSuffix(t *testing.T) {
	src := writeFile(t, t.TempDir(), "x.desktop",
		"[Desktop Entry]\nName=X\nName[ru]=Х\nExec=x\n")
	dst := filepath.Join(t.TempDir(), "x.desktop")
	if err := transformDesktop(src, dst, "/cap", "", " (capsule)"); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	body := string(got)
	if !strings.Contains(body, "Name=X (capsule)") {
		t.Errorf("Name suffix missed:\n%s", body)
	}
	if !strings.Contains(body, "Name[ru]=Х (capsule)") {
		t.Errorf("Name[ru] suffix missed:\n%s", body)
	}
}

func TestTransformDesktopDropsDBusActivatable(t *testing.T) {
	src := writeFile(t, t.TempDir(), "x.desktop",
		"[Desktop Entry]\nName=X\nExec=x\nDBusActivatable=true\nIcon=x\n")
	dst := filepath.Join(t.TempDir(), "x.desktop")
	if err := transformDesktop(src, dst, "/cap", "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	body := string(got)
	if strings.Contains(body, "DBusActivatable=true") {
		t.Errorf("DBusActivatable=true should be dropped:\n%s", body)
	}
	if !strings.Contains(body, "Icon=x") {
		t.Errorf("Icon should still be present:\n%s", body)
	}
}

func TestTransformDesktopKeepsDBusActivatableFalse(t *testing.T) {
	src := writeFile(t, t.TempDir(), "x.desktop",
		"[Desktop Entry]\nName=X\nExec=x\nDBusActivatable=false\n")
	dst := filepath.Join(t.TempDir(), "x.desktop")
	if err := transformDesktop(src, dst, "/cap", "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	if !strings.Contains(string(got), "DBusActivatable=false") {
		t.Errorf("DBusActivatable=false must be preserved:\n%s", got)
	}
}

func TestParseDesktopIconReadsEntrySection(t *testing.T) {
	src := writeFile(t, t.TempDir(), "code.desktop",
		`[Desktop Entry]
Name=Visual Studio Code
Exec=code %F
Icon=visual-studio-code
Type=Application

[Desktop Action new-empty-window]
Name=New Empty Window
Exec=code --new-window %F
Icon=action-icon
`)
	got := parseDesktopIcon(src)
	if got != "visual-studio-code" {
		t.Errorf("parseDesktopIcon=%q, want %q", got, "visual-studio-code")
	}
}

func TestParseDesktopIconMissingFile(t *testing.T) {
	if got := parseDesktopIcon(filepath.Join(t.TempDir(), "missing.desktop")); got != "" {
		t.Errorf("missing file must return empty, got %q", got)
	}
}

func TestParseDesktopIconNoIconLine(t *testing.T) {
	src := writeFile(t, t.TempDir(), "x.desktop",
		"[Desktop Entry]\nName=X\nExec=x\n")
	if got := parseDesktopIcon(src); got != "" {
		t.Errorf("missing Icon= must return empty, got %q", got)
	}
}

func TestTransformDesktopExecQuoted(t *testing.T) {
	src := writeFile(t, t.TempDir(), "x.desktop",
		`Exec="foo bar" --flag
`)
	dst := filepath.Join(t.TempDir(), "x.desktop")
	if err := transformDesktop(src, dst, "/cap", "", ""); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(dst)
	body := string(got)
	// First word is `"foo`, becomes `foo` after stripping quotes.
	if !strings.Contains(body, "Exec=/cap ") {
		t.Errorf("Exec rewrite failed:\n%s", body)
	}
}
