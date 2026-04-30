package export

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
)

// iconSizes — large first so the host prefers high-DPI.
var iconSizes = []string{"256x256", "128x128", "64x64", "48x48", "scalable"}

func findAndCopyIcon(root, iconName, xdgDataHome string) (string, error) {
	for _, size := range iconSizes {
		ext := "png"
		if size == "scalable" {
			ext = "svg"
		}
		src := filepath.Join(root, "usr/share/icons/hicolor", size, "apps", iconName+"."+ext)
		if _, err := os.Stat(src); err == nil {
			dst := filepath.Join(xdgDataHome, "icons/hicolor", size, "apps", iconName+"."+ext)
			if err := copyFile(src, dst); err != nil {
				return "", err
			}
			return dst, nil
		}
	}
	for _, ext := range []string{"png", "svg", "xpm"} {
		src := filepath.Join(root, "usr/share/pixmaps", iconName+"."+ext)
		if _, err := os.Stat(src); err == nil {
			dst := filepath.Join(xdgDataHome, "icons/hicolor/48x48/apps", iconName+"."+ext)
			if err := copyFile(src, dst); err != nil {
				return "", err
			}
			return dst, nil
		}
	}
	return "", nil
}

func copyFile(src, dst string) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer out.Close()
	if _, err := io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s: %w", dst, err)
	}
	return nil
}

func removeIconFromHicolor(iconName, xdgDataHome string) []string {
	var removed []string
	hicolor := filepath.Join(xdgDataHome, "icons/hicolor")
	_ = filepath.WalkDir(hicolor, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fs.SkipAll
			}
			return nil
		}
		if d.IsDir() {
			return nil
		}
		base := filepath.Base(path)
		if base == iconName+".png" || base == iconName+".svg" || base == iconName+".xpm" {
			if err := os.Remove(path); err == nil {
				removed = append(removed, path)
			}
		}
		return nil
	})
	return removed
}
