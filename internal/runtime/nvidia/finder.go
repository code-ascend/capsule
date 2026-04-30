package nvidia

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// SearchRoots covers non-FHS locations where NVIDIA configs end up (Nix, Guix).
var SearchRoots = []string{"/usr", "/etc", "/run", "/nix"}

func findFile(name, standardPath string) string {
	if standardPath != "" {
		if _, err := os.Stat(standardPath); err == nil {
			return standardPath
		}
	}
	if name == "" {
		return ""
	}
	return searchByName(name)
}

func searchByName(name string) string {
	for _, root := range SearchRoots {
		var found string
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if d.Name() == name {
				found = path
				return fs.SkipAll
			}
			return nil
		})
		if found != "" {
			return found
		}
	}
	return ""
}

func findByPathPattern(roots []string, substr string) []string {
	var out []string
	for _, root := range roots {
		_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) || errors.Is(err, fs.ErrPermission) {
					return fs.SkipDir
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			if strings.Contains(path, substr) {
				out = append(out, path)
			}
			return nil
		})
	}
	return out
}
