package export

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
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

func defaultPaths() (*Paths, error) {
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

// Exporter wraps capsule metadata and target paths used by all export ops.
type Exporter struct {
	capsulePath string
	cfg         *binconfig.Config
	root        string
	paths       *Paths
}

// New constructs an Exporter using XDG defaults from the host environment.
// `root` is the capsule's mounted rootfs (used by Apps to locate desktop/icon
// files); pass "" when only the unexport methods are needed.
func New(capsulePath string, cfg *binconfig.Config, root string) (*Exporter, error) {
	p, err := defaultPaths()
	if err != nil {
		return nil, err
	}
	return &Exporter{capsulePath: capsulePath, cfg: cfg, root: root, paths: p}, nil
}

// Apps exports .desktop files and their icons.
func (e *Exporter) Apps() error {
	if len(e.cfg.Apps) == 0 {
		return nil
	}
	for _, a := range e.cfg.Apps {
		src := filepath.Join(e.root, a.Desktop)
		if _, err := os.Stat(src); err != nil {
			fmt.Fprintln(os.Stderr, gotext.Get("Warning:"), a.Desktop, gotext.Get("not found in capsule"))
			continue
		}
		icon := a.Icon
		if icon == "" {
			icon = parseDesktopIcon(src)
		}
		name := iconBaseName(icon)
		override := a.Icon
		if override == "" {
			override = name
		}
		dst := filepath.Join(e.paths.XDGDataHome, "applications", filepath.Base(a.Desktop))
		if err := transformDesktop(src, dst, e.capsulePath, override, a.NameSuffix); err != nil {
			return err
		}
		fmt.Println(gotext.Get("Desktop:"), dst)
		if name != "" {
			if paths, err := findAndCopyIcons(e.root, name, e.paths.XDGDataHome); err == nil {
				for _, p := range paths {
					fmt.Println(gotext.Get("Icon:   "), p)
				}
			}
		}
	}
	return nil
}

// Binaries writes shell wrappers under XDGBinHome that re-exec the capsule.
func (e *Exporter) Binaries() error {
	if len(e.cfg.Binaries) == 0 {
		return nil
	}
	if err := os.MkdirAll(e.paths.XDGBinHome, 0755); err != nil {
		return err
	}
	for _, b := range e.cfg.Binaries {
		dst := filepath.Join(e.paths.XDGBinHome, filepath.Base(b))
		if _, err := os.Stat(dst); err == nil {
			fmt.Println(gotext.Get("Skip:   "), dst, gotext.Get("(exists)"))
			continue
		}
		body := fmt.Sprintf("#!/bin/sh\nexec %q %q \"$@\"\n", e.capsulePath, b)
		if err := os.WriteFile(dst, []byte(body), 0755); err != nil {
			return fmt.Errorf("%s: %w", gotext.Get("write %s", dst), err)
		}
		fmt.Println(gotext.Get("Binary: "), dst)
	}
	return nil
}

// UnexportApps removes our exported .desktop files and the icons we copied.
func (e *Exporter) UnexportApps() error {
	for _, a := range e.cfg.Apps {
		dst := filepath.Join(e.paths.XDGDataHome, "applications", filepath.Base(a.Desktop))
		icon := a.Icon
		if icon == "" {
			icon = parseDesktopIcon(dst)
		}
		icon = iconBaseName(icon)
		if err := os.Remove(dst); err == nil {
			fmt.Println(gotext.Get("Removed:"), dst)
		} else if !errors.Is(err, fs.ErrNotExist) {
			return err
		}
		if icon != "" {
			for _, removed := range removeIconFromHiColor(icon, e.paths.XDGDataHome) {
				fmt.Println(gotext.Get("Removed:"), removed)
			}
		}
	}
	return nil
}

// UnexportBinaries removes only wrappers that reference our capsule path.
func (e *Exporter) UnexportBinaries() error {
	for _, b := range e.cfg.Binaries {
		dst := filepath.Join(e.paths.XDGBinHome, filepath.Base(b))
		body, err := os.ReadFile(dst)
		if err != nil {
			continue
		}
		if !strings.Contains(string(body), e.capsulePath) {
			continue
		}
		if err = os.Remove(dst); err == nil {
			fmt.Println(gotext.Get("Removed:"), dst)
		}
	}
	return nil
}

// MaybeUpdateDesktopCaches refreshes GTK icon and desktop caches on GNOME.
func (e *Exporter) MaybeUpdateDesktopCaches(ctx context.Context) {
	if !strings.Contains(strings.ToUpper(os.Getenv("XDG_CURRENT_DESKTOP")), "GNOME") {
		return
	}
	_ = exec.CommandContext(ctx, "gtk-update-icon-cache", "-f", "-t", filepath.Join(e.paths.XDGDataHome, "icons/hicolor")).Run()
	_ = exec.CommandContext(ctx, "update-desktop-database", filepath.Join(e.paths.XDGDataHome, "applications")).Run()
}
