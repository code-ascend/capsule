package fsutil

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
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
	return err == nil && st.Mode()&0o111 != 0
}

// CopyFile copies src to dst, creating dst's parent dirs if needed.
func CopyFile(src, dst string) (err error) {
	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close %s: %w", dst, cerr)
		}
	}()
	if _, err = io.Copy(out, in); err != nil {
		return fmt.Errorf("copy %s: %w", dst, err)
	}
	return nil
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
