package workspace

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"sync"
	"syscall"

	"capsule/internal/runtime/overlay"
	"capsule/internal/sys/log"
)

// Workspace is a per-capsule scratch dir shared across concurrent launchers.
type Workspace struct {
	Dir       string
	setupPath string
	aliveFD   *os.File

	cleanupOnce sync.Once
	cleanupFns  []func() error
}

// New attaches to the shared workspace for capsulePath under LOCK_SH.
func New(capsulePath string) (*Workspace, error) {
	base, err := chooseBaseDir(capsulePath)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(filepath.Dir(base), 0700); err != nil {
		return nil, fmt.Errorf("mkdir parent: %w", err)
	}

	f, err := os.OpenFile(base+".lock", os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, fmt.Errorf("open activity lock: %w", err)
	}
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_SH); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("flock activity lock: %w", err)
	}

	if err = os.MkdirAll(base, 0700); err != nil {
		_ = f.Close()
		return nil, fmt.Errorf("mkdir %s: %w", base, err)
	}

	return &Workspace{Dir: base, setupPath: base + ".setup.lock", aliveFD: f}, nil
}

func (w *Workspace) MntPath() string   { return filepath.Join(w.Dir, "mnt") }
func (w *Workspace) UtilsPath() string { return filepath.Join(w.Dir, "utils") }
func (w *Workspace) EtcPath() string   { return filepath.Join(w.Dir, "etc") }

// AddCleanup registers a LIFO callback run only if we are the last session.
func (w *Workspace) AddCleanup(fn func() error) {
	w.cleanupFns = append(w.cleanupFns, fn)
}

// WithSetupLock runs fn under LOCK_EX on the setup-lock file.
func (w *Workspace) WithSetupLock(fn func() error) error {
	f, err := os.OpenFile(w.setupPath, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return fmt.Errorf("open setup lock: %w", err)
	}
	defer f.Close()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		return fmt.Errorf("flock setup lock: %w", err)
	}
	return fn()
}

// LastSession reports whether no other process holds the activity lock.
func (w *Workspace) LastSession() bool {
	if err := syscall.Flock(int(w.aliveFD.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		return false
	}
	if err := syscall.Flock(int(w.aliveFD.Fd()), syscall.LOCK_SH); err != nil {
		log.Warn("failed to restore SH after LastSession", "error", err)
	}
	return true
}

// Cleanup runs callbacks and removes Dir if we were last; else just drops SH.
func (w *Workspace) Cleanup() {
	w.cleanupOnce.Do(func() {
		if err := syscall.Flock(int(w.aliveFD.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
			_ = w.aliveFD.Close()
			return
		}
		for i := len(w.cleanupFns) - 1; i >= 0; i-- {
			if err := w.cleanupFns[i](); err != nil {
				log.Debug("cleanup callback failed", "i", i, "error", err)
			}
		}
		if err := os.RemoveAll(w.Dir); err != nil {
			log.Debug("remove workspace failed", "dir", w.Dir, "error", err)
		}
		_ = os.Remove(w.setupPath)
		_ = w.aliveFD.Close()
	})
}

func chooseBaseDir(capsulePath string) (string, error) {
	u, err := user.Current()
	if err != nil {
		return "", err
	}
	name := "capsule_" + u.Username + "_" + overlay.HashPath(capsulePath)
	if rt := os.Getenv("XDG_RUNTIME_DIR"); rt != "" {
		return filepath.Join(rt, name), nil
	}
	return filepath.Join("/tmp", name), nil
}
