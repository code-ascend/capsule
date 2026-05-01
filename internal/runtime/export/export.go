package export

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"capsule/internal/format/binconfig"

	"github.com/leonelquinteros/gotext"
)

type Filter string

const (
	FilterAll      Filter = "all"
	FilterApps     Filter = "apps"
	FilterBinaries Filter = "binaries"
)

func ParseFilter(s string) (Filter, error) {
	switch s {
	case "", "all":
		return FilterAll, nil
	case "apps":
		return FilterApps, nil
	case "binaries":
		return FilterBinaries, nil
	default:
		return "", errors.New(gotext.Get("unknown filter %q (allowed: all, apps, binaries)", s))
	}
}

type Paths struct {
	XDGDataHome string
	XDGBinHome  string
}

func DefaultPaths() (*Paths, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}
	dataHome := os.Getenv("XDG_DATA_HOME")
	if dataHome == "" {
		dataHome = filepath.Join(home, ".local/share")
	}
	return &Paths{
		XDGDataHome: dataHome,
		XDGBinHome:  filepath.Join(home, ".local/bin"),
	}, nil
}

func Apps(root, capsulePath string, cfg *binconfig.Config, p *Paths) error {
	if len(cfg.Apps) == 0 {
		return nil
	}
	for _, a := range cfg.Apps {
		src := filepath.Join(root, a.Desktop)
		if _, err := os.Stat(src); err != nil {
			fmt.Fprintln(os.Stderr, gotext.Get("Warning:"), a.Desktop, gotext.Get("not found in capsule"))
			continue
		}
		dst := filepath.Join(p.XDGDataHome, "applications", filepath.Base(a.Desktop))
		if err := transformDesktop(src, dst, capsulePath, a.Icon, a.NameSuffix); err != nil {
			return err
		}
		fmt.Println(gotext.Get("Desktop:"), dst)
		if a.Icon != "" {
			if path, err := findAndCopyIcon(root, a.Icon, p.XDGDataHome); err == nil && path != "" {
				fmt.Println(gotext.Get("Icon:   "), path)
			}
		}
	}
	return nil
}

func Binaries(capsulePath string, cfg *binconfig.Config, p *Paths) error {
	if len(cfg.Binaries) == 0 {
		return nil
	}
	if err := os.MkdirAll(p.XDGBinHome, 0755); err != nil {
		return err
	}
	for _, b := range cfg.Binaries {
		dst := filepath.Join(p.XDGBinHome, filepath.Base(b))
		if _, err := os.Stat(dst); err == nil {
			fmt.Println(gotext.Get("Skip:   "), dst, gotext.Get("(exists)"))
			continue
		}
		body := fmt.Sprintf("#!/bin/sh\nexec %q %q \"$@\"\n", capsulePath, b)
		if err := os.WriteFile(dst, []byte(body), 0755); err != nil {
			return fmt.Errorf("%s: %w", gotext.Get("write %s", dst), err)
		}
		fmt.Println(gotext.Get("Binary: "), dst)
	}
	return nil
}

func UnexportApps(cfg *binconfig.Config, p *Paths) error {
	for _, a := range cfg.Apps {
		dst := filepath.Join(p.XDGDataHome, "applications", filepath.Base(a.Desktop))
		if err := os.Remove(dst); err == nil {
			fmt.Println(gotext.Get("Removed:"), dst)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if a.Icon != "" {
			for _, removed := range removeIconFromHicolor(a.Icon, p.XDGDataHome) {
				fmt.Println("Removed:", removed)
			}
		}
	}
	return nil
}

// UnexportBinaries only removes wrappers that actually reference our capsule.
func UnexportBinaries(capsulePath string, cfg *binconfig.Config, p *Paths) error {
	for _, b := range cfg.Binaries {
		dst := filepath.Join(p.XDGBinHome, filepath.Base(b))
		body, err := os.ReadFile(dst)
		if err != nil {
			continue
		}
		if !strings.Contains(string(body), capsulePath) {
			continue
		}
		if err := os.Remove(dst); err == nil {
			fmt.Println(gotext.Get("Removed:"), dst)
		}
	}
	return nil
}

func MaybeUpdateDesktopCaches(p *Paths) {
	if !strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "GNOME") {
		return
	}
	tryRun("gtk-update-icon-cache", "-f", "-t", filepath.Join(p.XDGDataHome, "icons/hicolor"))
	tryRun("update-desktop-database", filepath.Join(p.XDGDataHome, "applications"))
}
