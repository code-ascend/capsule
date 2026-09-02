package fsutil

import (
	"fmt"
	"io"
	"os"
	"os/user"
	"path/filepath"
	"strings"
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

// ExpandHome resolves a leading ~ against the current user's home directory.
func ExpandHome(p string) string {
	home, err := os.UserHomeDir()
	if err != nil {
		return p
	}
	return ExpandHomeIn(p, home)
}

// ExpandHomeIn resolves a leading ~ against home.
func ExpandHomeIn(p, home string) string {
	if p == "~" || strings.HasPrefix(p, "~/") {
		return filepath.Join(home, p[1:])
	}
	return p
}

// SudoUserHome returns the home of the user behind sudo, or "" when not running under sudo.
func SudoUserHome() string {
	name := os.Getenv("SUDO_USER")
	if name == "" {
		return ""
	}
	u, err := user.Lookup(name)
	if err != nil {
		return ""
	}
	return u.HomeDir
}

// InvokingHome returns the home of the invoking user, preferring the sudo caller.
func InvokingHome() (string, error) {
	if home := SudoUserHome(); home != "" {
		return home, nil
	}
	return os.UserHomeDir()
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

// SyncDir flushes directory metadata so a rename inside dir survives a crash.
func SyncDir(dir string) error {
	d, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer func() { _ = d.Close() }()
	return d.Sync()
}
