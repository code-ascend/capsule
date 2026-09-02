package reaper

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"capsule/internal/sys/log"

	"golang.org/x/sys/unix"
)

const (
	defaultPollInterval = 200 * time.Millisecond
	killPasses          = 5
)

// EnableSubReaper sets PR_SET_CHILD_SUBREAPER; call before exec'ing descendants.
func EnableSubReaper() error {
	return unix.Prctl(unix.PR_SET_CHILD_SUBREAPER, 1, 0, 0, 0)
}

// Reaper drains in-capsule descendants on shutdown.
type Reaper struct {
	grace        time.Duration
	pollInterval time.Duration
	selfPid      int
	selfNS       string
	zombies      map[int]bool
}

// New snapshots our pid and mount ns for descendant filtering.
func New(grace time.Duration) *Reaper {
	pid := os.Getpid()
	ns, _ := readMountNS(pid)
	return &Reaper{
		grace:        grace,
		pollInterval: defaultPollInterval,
		selfPid:      pid,
		selfNS:       ns,
		zombies:      map[int]bool{},
	}
}

// Wait blocks until every in-capsule descendant exits or ctx is cancelled (SIGTERM then SIGKILL).
func (r *Reaper) Wait(ctx context.Context) {
	if r.drain(ctx.Done()) {
		return
	}

	pids := r.inCapsule()
	if len(pids) == 0 {
		return
	}
	log.Info("capsule: shutdown requested, sending SIGTERM", "count", len(pids))
	signalAll(pids, syscall.SIGTERM)

	timeout, cancel := context.WithTimeout(context.Background(), r.grace)
	defer cancel()
	if r.drain(timeout.Done()) {
		log.Debug("capsule: descendants exited gracefully")
		return
	}

	r.killAll()
}

// killAll SIGKILLs in-capsule descendants in several passes to catch late forks.
func (r *Reaper) killAll() {
	for i := range killPasses {
		pids := r.inCapsule()
		if len(pids) == 0 {
			return
		}
		if i == 0 {
			log.Warn("capsule: descendants ignored SIGTERM, sending SIGKILL", "count", len(pids))
		}
		signalAll(pids, syscall.SIGKILL)
		time.Sleep(r.pollInterval)
		r.reapOrphans()
	}
}

// drain polls until in-capsule descendants are gone, or stop fires.
func (r *Reaper) drain(stop <-chan struct{}) bool {
	tick := time.NewTicker(r.pollInterval)
	defer tick.Stop()
	for {
		r.reapOrphans()
		if len(r.inCapsule()) == 0 {
			return true
		}
		select {
		case <-tick.C:
		case <-stop:
			return false
		}
	}
}

// reapOrphans reaps zombies seen on two polls; os/exec children never linger that long.
func (r *Reaper) reapOrphans() {
	seen := map[int]bool{}
	for _, pid := range zombieChildren(r.selfPid) {
		if !r.zombies[pid] {
			seen[pid] = true
			continue
		}
		var ws syscall.WaitStatus
		if _, err := syscall.Wait4(pid, &ws, syscall.WNOHANG, nil); err != nil {
			log.Debug("reaper: wait4 failed", "pid", pid, "error", err)
			seen[pid] = true
		}
	}
	r.zombies = seen
}

// inCapsule lists live descendants outside our mount ns; unreadable ns counts as inside.
func (r *Reaper) inCapsule() []int {
	all := descendants(r.selfPid)
	if len(all) == 0 || r.selfNS == "" {
		return all
	}
	out := all[:0]
	for _, pid := range all {
		ns, err := readMountNS(pid)
		switch {
		case err == nil && ns == r.selfNS:
			continue
		case errors.Is(err, fs.ErrNotExist):
			continue
		case err != nil:
			if isZombie(pid) {
				continue
			}
			log.Debug("reaper: mount ns unreadable, assuming in-capsule", "pid", pid, "error", err)
		}
		out = append(out, pid)
	}
	return out
}

func signalAll(pids []int, sig syscall.Signal) {
	for _, pid := range pids {
		_ = syscall.Kill(pid, sig)
	}
}

// zombieChildren lists direct children of root in zombie state.
func zombieChildren(root int) []int {
	var out []int
	for _, pid := range children(root) {
		if isZombie(pid) {
			out = append(out, pid)
		}
	}
	return out
}

// children lists direct children of pid from /proc/pid/task/*/children.
func children(pid int) []int {
	taskDir := filepath.Join("/proc", strconv.Itoa(pid), "task")
	tids, err := os.ReadDir(taskDir)
	if err != nil {
		return nil
	}
	var out []int
	for _, t := range tids {
		data, err := os.ReadFile(filepath.Join(taskDir, t.Name(), "children"))
		if err != nil {
			continue
		}
		for f := range strings.FieldsSeq(string(data)) {
			if p, err := strconv.Atoi(f); err == nil {
				out = append(out, p)
			}
		}
	}
	return out
}

// descendants BFS's the process tree under root.
func descendants(root int) []int {
	var out []int
	queue := children(root)
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		out = append(out, p)
		queue = append(queue, children(p)...)
	}
	return out
}

// isZombie reports whether /proc/pid/status shows state Z; false when unreadable.
func isZombie(pid int) bool {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return false
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "State:"); ok {
			return strings.HasPrefix(strings.TrimSpace(v), "Z")
		}
	}
	return false
}

func readMountNS(pid int) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/%d/ns/mnt", pid))
}
