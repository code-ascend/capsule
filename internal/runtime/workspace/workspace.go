package workspace

import (
	"errors"
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"capsule/internal/runtime/overlay"
	"capsule/internal/sys/log"
)

// cleanupGrace bridges back-to-back launches before tearing down shared mounts.
const cleanupGrace = time.Second

// sessionsSubDir holds per-pid sentinel files used for refcounting.
const sessionsSubDir = "sessions"

type Workspace struct {
	Dir         string
	sessionFile string

	cleanupOnce sync.Once
	cleanupFns  []func() error
}

// New attaches to the shared workspace for capsulePath.
func New(capsulePath string) (*Workspace, error) {
	base, err := chooseBaseDir(capsulePath)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(base, 0700); err != nil {
		return nil, fmt.Errorf("mkdir %s: %w", base, err)
	}

	sessionsDir := filepath.Join(base, sessionsSubDir)
	if err = os.MkdirAll(sessionsDir, 0700); err != nil {
		return nil, err
	}
	cleanupStaleSessions(sessionsDir)

	pid := strconv.Itoa(os.Getpid())
	sessionFile := filepath.Join(sessionsDir, pid)
	if err = os.WriteFile(sessionFile, nil, 0600); err != nil {
		return nil, fmt.Errorf("write session sentinel: %w", err)
	}

	return &Workspace{Dir: base, sessionFile: sessionFile}, nil
}

func (w *Workspace) MntPath() string   { return filepath.Join(w.Dir, "mnt") }
func (w *Workspace) UtilsPath() string { return filepath.Join(w.Dir, "utils") }
func (w *Workspace) EtcPath() string   { return filepath.Join(w.Dir, "etc") }

// LastSession reports whether we are the only live session.
func (w *Workspace) LastSession() bool {
	entries, err := os.ReadDir(filepath.Join(w.Dir, sessionsSubDir))
	if err != nil {
		return true
	}
	live := 0
	self := filepath.Base(w.sessionFile)
	for _, e := range entries {
		if e.Name() == self {
			continue
		}
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if pidAlive(pid) {
			live++
		}
	}
	return live == 0
}

func (w *Workspace) AddCleanup(fn func() error) {
	w.cleanupFns = append(w.cleanupFns, fn)
}

// Cleanup drops our sentinel and tears down the workspace if we were last.
func (w *Workspace) Cleanup() {
	w.cleanupOnce.Do(func() {
		if w.sessionFile != "" {
			_ = os.Remove(w.sessionFile)
		}
		if !w.noOtherSessions() {
			return
		}
		time.Sleep(cleanupGrace)
		if !w.noOtherSessions() {
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
	})
}

// noOtherSessions reports whether no live session sentinels remain.
func (w *Workspace) noOtherSessions() bool {
	entries, err := os.ReadDir(filepath.Join(w.Dir, sessionsSubDir))
	if err != nil {
		return true
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if pidAlive(pid) {
			return false
		}
	}
	return true
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

func cleanupStaleSessions(sessionsDir string) {
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return
	}
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if !pidAlive(pid) {
			_ = os.Remove(filepath.Join(sessionsDir, e.Name()))
		}
	}
}

func pidAlive(pid int) bool {
	_, err := os.Stat("/proc/" + strconv.Itoa(pid))
	return !errors.Is(err, os.ErrNotExist)
}
