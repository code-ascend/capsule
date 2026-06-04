package nvidia

import (
	"os"
	"path/filepath"
	"testing"
)

func mk(t *testing.T, root string, dirs []string, symlinks map[string]string) {
	t.Helper()
	for _, d := range dirs {
		if err := os.MkdirAll(filepath.Join(root, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for link, target := range symlinks {
		full := filepath.Join(root, link)
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, full); err != nil {
			t.Fatal(err)
		}
	}
}

func TestDetectLayoutDebianMultiarch(t *testing.T) {
	root := t.TempDir()
	mk(t, root, []string{"usr/lib/x86_64-linux-gnu", "usr/lib/i386-linux-gnu"}, nil)
	got := DetectLayout(root)
	if got.Lib64 != "usr/lib/x86_64-linux-gnu" {
		t.Errorf("debian Lib64 = %q", got.Lib64)
	}
	if got.Lib32 != "usr/lib/i386-linux-gnu" {
		t.Errorf("debian Lib32 = %q", got.Lib32)
	}
}

func TestDetectLayoutFedora(t *testing.T) {
	root := t.TempDir()
	mk(t, root, []string{"usr/lib64", "usr/lib"}, nil)
	got := DetectLayout(root)
	if got.Lib64 != "usr/lib64" || got.Lib32 != "usr/lib" {
		t.Errorf("fedora got %+v", got)
	}
}

func TestDetectLayoutArchSymlinkLib64(t *testing.T) {
	root := t.TempDir()
	mk(t, root, []string{"usr/lib"}, map[string]string{"usr/lib64": "lib"})
	got := DetectLayout(root)
	// /usr/lib64 is a symlink → must NOT pick it; falls through to flat.
	if got.Lib64 != "usr/lib" || got.Lib32 != "usr/lib32" {
		t.Errorf("arch symlink got %+v, want flat", got)
	}
}

func TestDetectLayoutFlat(t *testing.T) {
	root := t.TempDir()
	mk(t, root, []string{"usr/lib"}, nil)
	got := DetectLayout(root)
	if got.Lib64 != "usr/lib" || got.Lib32 != "usr/lib32" {
		t.Errorf("flat got %+v", got)
	}
}
