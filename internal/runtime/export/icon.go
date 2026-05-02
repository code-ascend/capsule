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

var iconSizes = []string{"256x256", "128x128", "64x64", "48x48", "scalable"}
var iconExts = []string{"png", "svg", "xpm"}
var iconUnsizedSrcDirs = []string{"usr/share/pixmaps", "usr/share/icons"}
var iconHostRoots = []string{"icons/hicolor", "pixmaps"}

func findAndCopyIcon(root, iconName, xdgDataHome string) (string, error) {
	for _, size := range iconSizes {
		ext := "png"
		if size == "scalable" {
			ext = "svg"
		}
		src := filepath.Join(root, "usr/share/icons/hicolor", size, "apps", iconName+"."+ext)
		if _, err := os.Stat(src); err == nil {
			dst := filepath.Join(xdgDataHome, "icons/hicolor", size, "apps", iconName+"."+ext)
			if err = fsutil.CopyFile(src, dst); err != nil {
				return "", err
			}
			return dst, nil
		}
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
				return "", err
			}
			return dst, nil
		}
	}
	return "", nil
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
