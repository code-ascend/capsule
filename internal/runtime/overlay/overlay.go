package overlay

import (
	"crypto/md5"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"capsule/internal/runtime/mount"
)

type Locator struct {
	CapsulePath string
	Base        string
}

func New(capsulePath string) *Locator {
	base := os.Getenv("CAPSULE_OVERLAY_DIR")
	if base == "" {
		home, _ := os.UserHomeDir()
		base = baseForHome(home, capsulePath)
	}
	return &Locator{CapsulePath: capsulePath, Base: base}
}

// NewForUser returns the locator for another user's runtime.
func NewForUser(capsulePath, userHome string) *Locator {
	return &Locator{CapsulePath: capsulePath, Base: baseForHome(userHome, capsulePath)}
}

// baseForHome builds the overlay base under home, resolving symlinks.
func baseForHome(home, capsulePath string) string {
	if resolved, err := filepath.EvalSymlinks(home); err == nil {
		home = resolved
	}
	return filepath.Join(home, ".local", "share", "capsule", "overlay_"+HashPath(capsulePath))
}

func (l *Locator) Upper() string  { return filepath.Join(l.Base, "upper") }
func (l *Locator) Work() string   { return filepath.Join(l.Base, "work") }
func (l *Locator) Merged() string { return filepath.Join(l.Base, "merged") }
func (l *Locator) EtcDir() string { return filepath.Join(l.Upper(), "etc") }

func (l *Locator) VersionMarker(name string) string {
	return filepath.Join(l.Base, name)
}

func (l *Locator) EnsureDirs() error {
	for _, d := range []string{l.Upper(), l.Work(), l.Merged()} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return err
		}
	}
	return nil
}

// Clean unmounts the merged overlay (if mounted) and removes the whole base.
func (l *Locator) Clean() error {
	st, err := os.Stat(l.Base)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if !st.IsDir() {
		return fmt.Errorf("not a directory: %s", l.Base)
	}
	_ = mount.Unmount(l.Merged())
	return os.RemoveAll(l.Base)
}

// HashPath returns the 8-char MD5 prefix used to derive overlay dir names.
// MD5 here is a non-cryptographic path hash, not a security primitive.
func HashPath(path string) string {
	sum := md5.Sum([]byte(path + "\n"))
	return hex.EncodeToString(sum[:])[:8]
}
