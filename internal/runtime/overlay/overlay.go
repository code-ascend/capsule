package overlay

import (
	"crypto/md5" //nolint:gosec // path hashing, not security
	"encoding/hex"
	"os"
	"path/filepath"
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
		if err := os.MkdirAll(d, 0755); err != nil {
			return err
		}
	}
	return nil
}

// HashPath returns the 8-char MD5 prefix used to derive overlay dir names.
func HashPath(path string) string {
	sum := md5.Sum([]byte(path + "\n")) //nolint:gosec
	return hex.EncodeToString(sum[:])[:8]
}
