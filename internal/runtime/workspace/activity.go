package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// lockFile returns the activity-lock path for a workspace base dir.
func lockFile(base string) string { return base + ".lock" }

// LockPath returns the activity-lock path for capsulePath.
func LockPath(capsulePath string) (string, error) {
	base, err := chooseBaseDir(capsulePath)
	if err != nil {
		return "", err
	}
	return lockFile(base), nil
}

// Active reports whether any identifiable session holds the activity lock for capsulePath.
func Active(capsulePath string) (bool, error) {
	pids, err := Holders(capsulePath)
	if err != nil {
		return false, err
	}
	return len(pids) > 0, nil
}

// Held probes the activity lock by briefly acquiring it; call only when no session is expected.
func Held(capsulePath string) (bool, error) {
	lock, err := LockPath(capsulePath)
	if err != nil {
		return false, err
	}
	f, err := os.OpenFile(lock, os.O_RDWR, 0o600)
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	defer func() { _ = f.Close() }()
	if err = syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		if errors.Is(err, syscall.EWOULDBLOCK) {
			return true, nil
		}
		return false, err
	}
	return false, nil
}

// Holders returns PIDs of processes verified to hold the activity lock for capsulePath open.
func Holders(capsulePath string) ([]int, error) {
	lock, err := LockPath(capsulePath)
	if err != nil {
		return nil, err
	}
	return holdersOf(lock)
}

// holdersOf returns one PID per verified flock entry on the lock file.
func holdersOf(lock string) ([]int, error) {
	var st syscall.Stat_t
	if err := syscall.Stat(lock, &st); err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	fileRef := fmt.Sprintf("%02x:%02x:%d", unix.Major(st.Dev), unix.Minor(st.Dev), st.Ino)

	data, err := os.ReadFile("/proc/locks")
	if err != nil {
		return nil, err
	}
	var pids []int
	for line := range strings.SplitSeq(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 7 || fields[1] != "FLOCK" || fields[5] != fileRef {
			continue
		}
		pid, err := strconv.Atoi(fields[4])
		if err != nil || pid <= 0 || !holdsOpen(pid, lock) {
			continue
		}
		pids = append(pids, pid)
	}
	return pids, nil
}

// holdsOpen reports whether pid has lock among its open file descriptors.
func holdsOpen(pid int, lock string) bool {
	fdDir := filepath.Join("/proc", strconv.Itoa(pid), "fd")
	fds, err := os.ReadDir(fdDir)
	if err != nil {
		return false
	}
	for _, fd := range fds {
		if target, err := os.Readlink(filepath.Join(fdDir, fd.Name())); err == nil && target == lock {
			return true
		}
	}
	return false
}
