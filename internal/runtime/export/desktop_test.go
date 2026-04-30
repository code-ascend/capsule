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
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
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
