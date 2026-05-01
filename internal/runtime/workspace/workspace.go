package workspace

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"sync"

	"capsule/internal/sys/log"
)

type Workspace struct {
	Dir string

	cleanupOnce sync.Once
	cleanupFns  []func() error
}

func New() (*Workspace, error) {
	base := os.Getenv("XDG_RUNTIME_DIR")
	if base == "" {
		base = "/tmp"
	}
	dir := filepath.Join(base, "capsule_"+strconv.Itoa(os.Getpid()))
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", dir, err)
	}
	return &Workspace{Dir: dir}, nil
}

func (w *Workspace) MntPath() string   { return filepath.Join(w.Dir, "mnt") }
func (w *Workspace) UtilsPath() string { return filepath.Join(w.Dir, "utils") }
func (w *Workspace) EtcPath() string   { return filepath.Join(w.Dir, "etc") }

func (w *Workspace) AddCleanup(fn func() error) {
	w.cleanupFns = append(w.cleanupFns, fn)
}

func (w *Workspace) Cleanup() {
	w.cleanupOnce.Do(func() {
		for i := len(w.cleanupFns) - 1; i >= 0; i-- {
			if err := w.cleanupFns[i](); err != nil {
				log.Debug("cleanup callback failed", "i", i, "error", err)
			}
		}
		if w.Dir != "" {
			if err := os.RemoveAll(w.Dir); err != nil {
				log.Debug("remove workspace failed", "dir", w.Dir, "error", err)
			}
		}
	})
}
