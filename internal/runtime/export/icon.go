package export

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"capsule/internal/sys/fsutil"
)

var (
	iconSizes          = []string{"256x256", "128x128", "64x64", "48x48", "32x32", "24x24", "16x16", "scalable"}
	iconExts           = []string{"png", "svg", "xpm"}
	iconUnsizedSrcDirs = []string{"usr/share/pixmaps", "usr/share/icons"}
	iconHostRoots      = []string{"icons/hicolor", "pixmaps"}
)

func findAndCopyIcons(root, iconName, xdgDataHome string) ([]string, error) {
	var copied []string
	for _, size := range iconSizes {
		exts := []string{"png", "xpm"}
		if size == "scalable" {
			exts = []string{"svg"}
		}
		for _, ext := range exts {
			rel := filepath.Join("icons/hicolor", size, "apps", iconName+"."+ext)
			src := filepath.Join(root, "usr/share", rel)
			if _, err := os.Stat(src); err != nil {
				continue
			}
			dst := filepath.Join(xdgDataHome, rel)
			if err := fsutil.CopyFile(src, dst); err != nil {
				return copied, err
			}
			copied = append(copied, dst)
			break
		}
	}
	if len(copied) > 0 {
		return copied, nil
	}
	for _, dir := range iconUnsizedSrcDirs {
		for _, ext := range iconExts {
			src := filepath.Join(root, dir, iconName+"."+ext)
			if _, err := os.Stat(src); err != nil {
				continue
			}
			var dst string
			if ext == "svg" {
				dst = filepath.Join(xdgDataHome, "icons/hicolor/scalable/apps", iconName+"."+ext)
			} else {
				dst = filepath.Join(xdgDataHome, "pixmaps", iconName+"."+ext)
			}
			if err := fsutil.CopyFile(src, dst); err != nil {
				return copied, err
			}
			return append(copied, dst), nil
		}
	}
	return copied, nil
}

func iconBaseName(value string) string {
	if !strings.HasPrefix(value, "/") {
		return value
	}
	base := filepath.Base(value)
	return strings.TrimSuffix(base, filepath.Ext(base))
}

func removeIconFromHiColor(iconName, xdgDataHome string) []string {
	var removed []string
	for _, sub := range iconHostRoots {
		_ = filepath.WalkDir(filepath.Join(xdgDataHome, sub), func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return fs.SkipAll
				}
				return nil
			}
			if d.IsDir() {
				return nil
			}
			ext := strings.TrimPrefix(filepath.Ext(path), ".")
			if filepath.Base(path) == iconName+"."+ext && slices.Contains(iconExts, ext) {
				if err = os.Remove(path); err == nil {
					removed = append(removed, path)
				}
			}
			return nil
		})
	}
	return removed
}
