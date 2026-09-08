package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFilesValidation(t *testing.T) {
	for name, file := range map[string]File{
		"empty source":    {Dst: "/tmp/"},
		"url source":      {Src: "https://example.com/foo.rpm", Dst: "/tmp/"},
		"empty target":    {Src: "foo.rpm"},
		"relative target": {Src: "foo.rpm", Dst: "tmp/"},
		"target parent":   {Src: "foo.rpm", Dst: "/tmp/../etc/foo"},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := Config{Image: "archlinux", Compression: "zstd", Files: []File{file}}
			if err := cfg.Validate(); err == nil {
				t.Fatal("invalid file mapping accepted")
			}
		})
	}
}

func TestFilesResolveAgainstRecipe(t *testing.T) {
	recipeDir := t.TempDir()
	src := filepath.Join(recipeDir, "foo.rpm")
	if err := os.WriteFile(src, []byte("package"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(t.TempDir())
	recipe := filepath.Join(recipeDir, "recipe.yaml")
	yaml := "image: archlinux\nfiles:\n  - src: foo.rpm\n    dst: /tmp/packages/\n"
	if err := os.WriteFile(recipe, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, raw, err := Load(recipe)
	if err != nil {
		t.Fatal(err)
	}
	files := cfg.Files
	if len(files) != 1 || files[0].Src != src || files[0].Dst != "/tmp/packages/" {
		t.Fatalf("unexpected resolved files: %+v", files)
	}
	if string(raw) != yaml {
		t.Fatal("resolution changed the raw recipe")
	}
}

func TestFilesResolveRemoteRecipe(t *testing.T) {
	cfg := Config{Files: []File{{Src: "foo.rpm", Dst: "/tmp/"}}}
	if _, err := cfg.resolveFiles("https://example.com/recipe.yaml"); err == nil {
		t.Fatal("relative local path accepted for a remote recipe")
	}
	cfg.Files[0].Src = t.TempDir()
	if _, err := cfg.resolveFiles("https://example.com/recipe.yaml"); err != nil {
		t.Fatalf("absolute local directory must work: %v", err)
	}
}

func TestFilesLoadExpandsHome(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("SUDO_USER", "")
	src := filepath.Join(home, "Downloads", "DaVinci_Resolve_Linux.zip")
	if err := os.MkdirAll(filepath.Dir(src), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(src, []byte("installer"), 0o644); err != nil {
		t.Fatal(err)
	}
	recipe := filepath.Join(t.TempDir(), "davinci.yaml")
	yaml := "image: archlinux\nfiles:\n  - src: ~/Downloads/DaVinci_Resolve_Linux.zip\n    dst: /tmp/\n"
	if err := os.WriteFile(recipe, []byte(yaml), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(recipe)
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.Files[0].Src; got != src {
		t.Fatalf("source = %q, want %q", got, src)
	}
}

func TestFilesResolveKeepsGlob(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "DaVinci_Resolve_18.6_Linux.zip"), []byte("installer"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := Config{Files: []File{{Src: "DaVinci_Resolve_*_Linux.zip", Dst: "/tmp/"}}}
	files, err := cfg.resolveFiles(filepath.Join(dir, "recipe.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, "DaVinci_Resolve_*_Linux.zip"); files[0].Src != want {
		t.Fatalf("glob must stay a pattern for buildah: got %q, want %q", files[0].Src, want)
	}
	cfg.Files[0].Src = "Other_*.zip"
	if _, err := cfg.resolveFiles(filepath.Join(dir, "recipe.yaml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("unmatched glob must fail with ErrNotExist, got %v", err)
	}
}

func TestFilesMissingSource(t *testing.T) {
	cfg := Config{Files: []File{{Src: "missing.rpm", Dst: "/tmp/"}}}
	_, err := cfg.resolveFiles(filepath.Join(t.TempDir(), "recipe.yaml"))
	if err == nil || !strings.Contains(err.Error(), "files[0].src") || !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected an actionable source error, got %v", err)
	}
}
