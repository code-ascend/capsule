package reaper

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"capsule/internal/sys/log"
)

const (
	prSetChildSubReaper = 36 // magic number from Linux kernel
	defaultPollInterval = 200 * time.Millisecond
)

// EnableSubReaper flips PR_SET_CHILD_SUBREAPER. Call early, before exec'ing
// any descendant we want to inherit on orphan.
func EnableSubReaper() error {
	_, _, errno := syscall.Syscall6(syscall.SYS_PRCTL, prSetChildSubReaper, 1, 0, 0, 0, 0)
	if errno != 0 {
		return errno
	}
	return nil
}

// Reaper drains in-capsule descendants on shutdown.
type Reaper struct {
	grace        time.Duration
	pollInterval time.Duration
	selfPid      int
	selfNS       string
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
	}
}

// Wait blocks until every in-capsule descendant exits, or ctx is cancelled.
// On cancel: SIGTERM, wait grace, SIGKILL survivors.
func (r *Reaper) Wait(ctx context.Context) {
	reaped := r.startReapLoop()

	if r.drain(ctx.Done(), reaped) {
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
	if r.drain(timeout.Done(), reaped) {
		log.Debug("capsule: descendants exited gracefully")
		return
	}

	pids = r.inCapsule()
	if len(pids) > 0 {
		log.Warn("capsule: descendants ignored SIGTERM, sending SIGKILL", "count", len(pids))
		signalAll(pids, syscall.SIGKILL)
	}
}

// startReapLoop reaps adopted orphans in the background. The returned
// channel closes when the whole descendant tree (incl. infrastructure)
// is gone.
func (r *Reaper) startReapLoop() <-chan struct{} {
	done := make(chan struct{})
	go func() {
		defer close(done)
		for {
			var ws syscall.WaitStatus
			_, err := syscall.Wait4(-1, &ws, 0, nil)
			if errors.Is(err, syscall.ECHILD) {
				return
			}
			if errors.Is(err, syscall.EINTR) {
				continue
			}
			if err != nil {
				log.Debug("reaper: wait4 failed", "error", err)
				return
			}
		}
	}()
	return done
}

// drain polls until in-capsule descendants are gone; true if drained,
// false when stop fires first.
func (r *Reaper) drain(stop, reaped <-chan struct{}) bool {
	tick := time.NewTicker(r.pollInterval)
	defer tick.Stop()
	for {
		if len(r.inCapsule()) == 0 {
			return true
		}
		select {
		case <-tick.C:
		case <-reaped:
			return true
		case <-stop:
			return false
		}
	}
}

// inCapsule lists descendants in a different mount ns than ours.
func (r *Reaper) inCapsule() []int {
	all := descendants(r.selfPid)
	if len(all) == 0 || r.selfNS == "" {
		return all
	}
	out := all[:0]
	for _, pid := range all {
		ns, err := readMountNS(pid)
		if err != nil || ns == r.selfNS {
			continue
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

// descendants walks /proc and BFS's the parent map from root.
func descendants(root int) []int {
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return nil
	}
	children := make(map[int][]int, len(entries))
	for _, e := range entries {
		pid, err := strconv.Atoi(e.Name())
		if err != nil {
			continue
		}
		if ppid := readPPid(pid); ppid > 0 {
			children[ppid] = append(children[ppid], pid)
		}
	}

	var out []int
	queue := []int{root}
	for len(queue) > 0 {
		p := queue[0]
		queue = queue[1:]
		for _, c := range children[p] {
			out = append(out, c)
			queue = append(queue, c)
		}
	}
	return out
}

func readPPid(pid int) int {
	data, err := os.ReadFile(filepath.Join("/proc", strconv.Itoa(pid), "status"))
	if err != nil {
		return 0
	}
	for line := range strings.SplitSeq(string(data), "\n") {
		if !strings.HasPrefix(line, "PPid:") {
			continue
		}
		fields := strings.Fields(line)
		if len(fields) >= 2 {
			ppid, _ := strconv.Atoi(fields[1])
			return ppid
		}
	}
	return 0
}

func readMountNS(pid int) (string, error) {
	return os.Readlink(fmt.Sprintf("/proc/%d/ns/mnt", pid))
}
