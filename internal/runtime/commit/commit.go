package commit

import (
	"capsule/internal/sys/fsutil"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"

	"capsule/internal/format/selfread"
	"capsule/internal/runtime/bundle"
	"capsule/internal/runtime/mount"
	"capsule/internal/runtime/overlay"
	"capsule/internal/sys/log"
)

type Options struct {
	CapsulePath   string
	Layout        *selfread.Layout
	Overlay       *overlay.Locator
	Bundle        *bundle.Extractor
	Compression   string
	SquashfsMount string

	// PreCommitClean strips host-specific files (e.g. NVIDIA libs) from upper/
	// before they get baked into a portable squashfs.
	PreCommitClean func(upper string) error
}

var ErrEmpty = errors.New("nothing to commit")

func (opts *Options) Run(ctx context.Context) error {
	upper := opts.Overlay.Upper()
	if empty, err := dirIsEmpty(upper); err != nil {
		return err
	} else if empty {
		return ErrEmpty
	}

	origUID, origGID, hadOwner := fsutil.Owner(opts.CapsulePath)

	if opts.PreCommitClean != nil {
		if err := opts.PreCommitClean(upper); err != nil {
			return fmt.Errorf("pre-commit clean: %w", err)
		}
	}

	merged := opts.Overlay.Merged()
	ownsMount := !mount.IsMounted(merged)
	if ownsMount {
		relaxed := os.Getuid() != 0
		if err := mount.Overlay(ctx, opts.Bundle, upper, opts.SquashfsMount, merged, relaxed); err != nil {
			return fmt.Errorf("mount overlay for commit: %w", err)
		}
	}
	defer func() {
		if ownsMount {
			_ = mount.Unmount(merged)
		}
	}()

	scriptDir := filepath.Dir(opts.CapsulePath)
	newSquashfs := filepath.Join(scriptDir, ".capsule_new.squashfs")
	defer os.Remove(newSquashfs)

	if err := buildSquashfs(ctx, opts.Bundle, merged, newSquashfs, opts.Compression); err != nil {
		return err
	}

	// Unmount squashfuse so the binary file can be replaced.
	if ownsMount {
		_ = mount.Unmount(merged)
	}
	_ = mount.Unmount(opts.SquashfsMount)

	newBinary := filepath.Join(scriptDir, ".capsule_new")
	defer os.Remove(newBinary)
	if err := assembleNewBinary(opts.CapsulePath, opts.Layout, newSquashfs, newBinary); err != nil {
		return err
	}
	if err := os.Chmod(newBinary, 0755); err != nil {
		return err
	}
	if err := os.Rename(newBinary, opts.CapsulePath); err != nil {
		return fmt.Errorf("atomic replace: %w", err)
	}
	if hadOwner {
		if err := os.Chown(opts.CapsulePath, origUID, origGID); err != nil {
			log.Debug("preserve owner failed", "error", err)
		}
	}

	if err := os.RemoveAll(upper); err != nil {
		log.Debug("rm upper failed", "error", err)
	}
	if err := os.MkdirAll(upper, 0755); err != nil {
		log.Debug("mkdir upper failed", "error", err)
	}
	return nil
}

func dirIsEmpty(path string) (bool, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return true, nil
		}
		return false, err
	}
	defer f.Close()
	names, err := f.Readdirnames(1)
	if err == io.EOF {
		return true, nil
	}
	return len(names) == 0, err
}

func buildSquashfs(ctx context.Context, b *bundle.Extractor, src, dst, compression string) error {
	args := []string{src, dst, "-comp", compression, "-noappend", "-no-xattrs"}
	switch compression {
	case "zstd":
		args = append(args, "-b", "1M", "-Xcompression-level", "19")
	case "xz":
		args = append(args, "-b", "1M", "-Xbcj", "x86")
	case "lz4":
		args = append(args, "-b", "256K", "-Xhc")
	case "gzip", "":
		args = append(args, "-b", "1M")
	}
	args = append(args, "-quiet")

	cmd := b.Command(ctx, "mksquashfs", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("mksquashfs: %w", err)
	}
	return nil
}

// assembleNewBinary copies the runtime+binconfig prefix and appends a fresh
// squashfs + footer.
func assembleNewBinary(origPath string, layout *selfread.Layout, newSquashfsPath, dst string) error {
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return err
	}
	defer out.Close()

	in, err := os.Open(origPath)
	if err != nil {
		return err
	}
	defer in.Close()

	if _, err := io.CopyN(out, in, layout.SquashfsOffset); err != nil {
		return fmt.Errorf("copy preamble: %w", err)
	}

	sqfs, err := os.Open(newSquashfsPath)
	if err != nil {
		return err
	}
	defer sqfs.Close()
	sqfsInfo, err := sqfs.Stat()
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, sqfs); err != nil {
		return fmt.Errorf("copy new squashfs: %w", err)
	}
	return selfread.EncodeFooter(out, layout.BinConfigSize, sqfsInfo.Size())
}
