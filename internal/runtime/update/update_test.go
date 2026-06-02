package update

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestBackupRoundTrip(t *testing.T) {
	parent := t.TempDir()
	upper := filepath.Join(parent, "upper")
	mustMkdir(t, upper)
	mustWrite(t, filepath.Join(upper, "a.txt"), "hello")
	mustMkdir(t, filepath.Join(upper, "sub"))
	mustWrite(t, filepath.Join(upper, "sub", "b.txt"), "world")
	if err := os.Symlink("a.txt", filepath.Join(upper, "link")); err != nil {
		t.Fatal(err)
	}

	backup, err := Take(t.Context(), upper)
	if err != nil {
		t.Fatalf("take: %v", err)
	}

	// Imitate a failing update.
	mustWrite(t, filepath.Join(upper, "a.txt"), "trashed")
	mustRemove(t, filepath.Join(upper, "sub", "b.txt"))
	mustRemove(t, filepath.Join(upper, "link"))

	if err := backup.Restore(upper); err != nil {
		t.Fatalf("restore: %v", err)
	}

	if got := mustRead(t, filepath.Join(upper, "a.txt")); got != "hello" {
		t.Errorf("a.txt = %q, want hello", got)
	}
	if got := mustRead(t, filepath.Join(upper, "sub", "b.txt")); got != "world" {
		t.Errorf("sub/b.txt = %q, want world", got)
	}
	target, err := os.Readlink(filepath.Join(upper, "link"))
	if err != nil {
		t.Fatalf("readlink: %v", err)
	}
	if target != "a.txt" {
		t.Errorf("link target = %q, want a.txt", target)
	}
}

func TestBackupPreservesMode(t *testing.T) {
	parent := t.TempDir()
	upper := filepath.Join(parent, "upper")
	mustMkdir(t, upper)
	exec := filepath.Join(upper, "script.sh")
	mustWrite(t, exec, "#!/bin/sh\n")
	if err := os.Chmod(exec, 0750); err != nil {
		t.Fatal(err)
	}

	backup, err := Take(t.Context(), upper)
	if err != nil {
		t.Fatal(err)
	}
	st, err := os.Stat(filepath.Join(backup.Path, "script.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode().Perm() != 0750 {
		t.Errorf("mode = %o, want 0750", st.Mode().Perm())
	}
}

func TestBackupDiscard(t *testing.T) {
	parent := t.TempDir()
	upper := filepath.Join(parent, "upper")
	mustMkdir(t, upper)
	mustWrite(t, filepath.Join(upper, "a.txt"), "x")

	backup, err := Take(t.Context(), upper)
	if err != nil {
		t.Fatal(err)
	}
	backup.Discard()
	if _, err := os.Stat(backup.Path); !os.IsNotExist(err) {
		t.Errorf("Discard left backup at %s", backup.Path)
	}
}

func TestBackupNilSafe(t *testing.T) {
	var b *Backup
	if err := b.Restore("/dev/null"); err != nil {
		t.Errorf("nil Restore: %v", err)
	}
	b.Discard() // must not panic
}

func TestBackupPreservesSpecialModeAndDirTime(t *testing.T) {
	parent := t.TempDir()
	upper := filepath.Join(parent, "upper")
	mustMkdir(t, upper)

	suid := filepath.Join(upper, "suid")
	mustWrite(t, suid, "x")
	if err := os.Chmod(suid, os.ModeSetuid|0777); err != nil {
		t.Fatal(err)
	}

	dir := filepath.Join(upper, "d")
	mustMkdir(t, dir)
	mustWrite(t, filepath.Join(dir, "child"), "y")
	want := time.Date(2021, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := os.Chtimes(dir, want, want); err != nil {
		t.Fatal(err)
	}

	backup, err := Take(t.Context(), upper)
	if err != nil {
		t.Fatal(err)
	}

	st, err := os.Stat(filepath.Join(backup.Path, "suid"))
	if err != nil {
		t.Fatal(err)
	}
	if st.Mode()&os.ModeSetuid == 0 {
		t.Errorf("setuid bit lost, mode = %v", st.Mode())
	}
	if st.Mode().Perm() != 0777 {
		t.Errorf("perm = %o, want 0777 (umask not defeated)", st.Mode().Perm())
	}

	dst, err := os.Stat(filepath.Join(backup.Path, "d"))
	if err != nil {
		t.Fatal(err)
	}
	if !dst.ModTime().Equal(want) {
		t.Errorf("dir mtime = %v, want %v (child write bumped it)", dst.ModTime(), want)
	}
}

func mustMkdir(t *testing.T, p string) {
	t.Helper()
	if err := os.MkdirAll(p, 0755); err != nil {
		t.Fatal(err)
	}
}

func mustWrite(t *testing.T, p, body string) {
	t.Helper()
	if err := os.WriteFile(p, []byte(body), 0644); err != nil {
		t.Fatal(err)
	}
}

func mustRead(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

func mustRemove(t *testing.T, p string) {
	t.Helper()
	if err := os.Remove(p); err != nil {
		t.Fatal(err)
	}
}
