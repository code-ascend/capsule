package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"capsule/internal/sys/srcref"
)

// File copies local content into the image before install steps.
type File struct {
	Src string `yaml:"src"`
	Dst string `yaml:"dst"`
}

func (c *Config) validateFiles() error {
	for i, file := range c.Files {
		if strings.TrimSpace(file.Src) == "" {
			return fmt.Errorf("files[%d].src is required", i)
		}
		if srcref.IsRemote(file.Src) {
			return fmt.Errorf("files[%d].src must be a local path, URLs are not supported", i)
		}
		if !filepath.IsAbs(file.Dst) {
			return fmt.Errorf("files[%d].dst must be an absolute path inside the image", i)
		}
		for part := range strings.SplitSeq(file.Dst, "/") {
			if part == ".." {
				return fmt.Errorf("files[%d].dst must not contain '..'", i)
			}
		}
	}
	return nil
}

// resolveFiles makes sources absolute relative to the recipe directory and checks that each pattern matches something.
func (c *Config) resolveFiles(recipe string) ([]File, error) {
	files := make([]File, 0, len(c.Files))
	for i, file := range c.Files {
		src := expandTilde(file.Src)
		if !filepath.IsAbs(src) {
			if recipe == "" || srcref.IsRemote(recipe) {
				return nil, fmt.Errorf("files[%d].src: relative path requires a local recipe", i)
			}
			src = filepath.Join(filepath.Dir(recipe), src)
		}
		abs, err := filepath.Abs(src)
		if err != nil {
			return nil, fmt.Errorf("files[%d].src: %w", i, err)
		}
		matches, err := filepath.Glob(abs)
		if err != nil {
			return nil, fmt.Errorf("files[%d].src %q: %w", i, abs, err)
		}
		if len(matches) == 0 {
			return nil, fmt.Errorf("files[%d].src %q: %w", i, abs, os.ErrNotExist)
		}
		files = append(files, File{Src: abs, Dst: file.Dst})
	}
	return files, nil
}
