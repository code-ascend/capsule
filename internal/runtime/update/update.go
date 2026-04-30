package update

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"syscall"
)

type Backup struct {
	Path string
}

// Take snapshots upperDir into a sibling _backup directory, preserving mode,
// symlinks, ownership, and mtime. Special files (block/char/fifo/sock) are
// skipped — overlay-upper shouldn't contain them.
func Take(ctx context.Context, upperDir string) (*Backup, error) {
	backup := filepath.Join(filepath.Dir(upperDir), filepath.Base(upperDir)+"_backup")
	if err := os.RemoveAll(backup); err != nil {
		return nil, fmt.Errorf("clean prior backup: %w", err)
	}
	if err := copyTree(ctx, upperDir, backup); err != nil {
		_ = os.RemoveAll(backup)
		return nil, fmt.Errorf("snapshot upper: %w", err)
	}
	return &Backup{Path: backup}, nil
}

func (b *Backup) Restore(upperDir string) error {
	if b == nil {
		return nil
	}
	if err := os.RemoveAll(upperDir); err != nil {
		return err
	}
	return os.Rename(b.Path, upperDir)
}

func (b *Backup) Discard() {
	if b == nil {
		return
	}
	_ = os.RemoveAll(b.Path)
}

var (
	ErrEmptyScript = errors.New("no update script defined in capsule config")
	ErrNotRoot     = errors.New("--update requires root privileges")
)

func CheckPreconditions(script string) error {
	if script == "" {
		return ErrEmptyScript
	}
	if os.Getuid() != 0 {
		return ErrNotRoot
	}
	return nil
}

func copyTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		info, err := d.Info()
		if err != nil {
			return err
		}
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			link, err := os.Readlink(path)
			if err != nil {
				return err
			}
			if err = os.Symlink(link, target); err != nil {
				return err
			}
			return lchownFromInfo(target, info)
		case info.IsDir():
			if err = os.MkdirAll(target, info.Mode().Perm()); err != nil {
				return err
			}
			return chownAndTime(target, info)
		case info.Mode().IsRegular():
			if err = copyRegular(path, target, info); err != nil {
				return err
			}
			return chownAndTime(target, info)
		default:
			return nil // skip block/char/fifo/sock
		}
	})
}

func copyRegular(src, dst string, info fs.FileInfo) error {
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}

func chownAndTime(path string, info fs.FileInfo) error {
	if st, ok := info.Sys().(*syscall.Stat_t); ok {
		if err := os.Chown(path, int(st.Uid), int(st.Gid)); err != nil {
			return err
		}
	}
	return os.Chtimes(path, info.ModTime(), info.ModTime())
}

func lchownFromInfo(path string, info fs.FileInfo) error {
	st, ok := info.Sys().(*syscall.Stat_t)
	if !ok {
		return nil
	}
	return os.Lchown(path, int(st.Uid), int(st.Gid))
}
