package fsutil

import (
	"os"
	"syscall"
)

func Exists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func IsDir(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.IsDir()
}

func IsExecutable(path string) bool {
	st, err := os.Stat(path)
	return err == nil && st.Mode()&0111 != 0
}

// Owner returns the UID/GID of path
func Owner(path string) (uid, gid int, ok bool) {
	st, err := os.Stat(path)
	if err != nil {
		return 0, 0, false
	}
	sys, isStat := st.Sys().(*syscall.Stat_t)
	if !isStat {
		return 0, 0, false
	}
	return int(sys.Uid), int(sys.Gid), true
}
