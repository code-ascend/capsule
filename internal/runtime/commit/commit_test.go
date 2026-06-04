package commit

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirIsEmpty(t *testing.T) {
	t.Run("nonexistent", func(t *testing.T) {
		ok, err := dirIsEmpty("/this/does/not/exist/at/all")
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !ok {
			t.Error("nonexistent must report empty=true")
		}
	})
	t.Run("empty dir", func(t *testing.T) {
		ok, err := dirIsEmpty(t.TempDir())
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if !ok {
			t.Error("empty dir must report empty=true")
		}
	})
	t.Run("non-empty dir", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "x"), nil, 0o644); err != nil {
			t.Fatal(err)
		}
		ok, err := dirIsEmpty(dir)
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if ok {
			t.Error("non-empty dir must report empty=false")
		}
	})
}
