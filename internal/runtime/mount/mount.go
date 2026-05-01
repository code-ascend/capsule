package mount

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"

	"capsule/internal/runtime/bundle"
	"capsule/internal/sys/log"
)

func Squashfs(ctx context.Context, b *bundle.Extractor, capsulePath string, offset int64, mountPoint string) error {
	if isMounted(mountPoint) {
		return nil
	}
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
		return fmt.Errorf("mkdir mountpoint: %w", err)
	}
	bin := "squashfuse"
	if b.HasBin("squashfuse_ll") {
		bin = "squashfuse_ll"
	}
	opts := "offset=" + strconv.FormatInt(offset, 10)
	if os.Getuid() == 0 {
		opts += ",allow_other"
	}
	cmd := b.Command(ctx, bin, "-o", opts, capsulePath, mountPoint)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("squashfuse mount: %w", err)
	}
	log.Debug("squashfs mounted", "binary", bin, "offset", offset, "mount", mountPoint, "opts", opts)
	return nil
}

func Overlay(ctx context.Context, b *bundle.Extractor, upper, lower, merged string, relaxedPermissions bool) error {
	if isMounted(merged) {
		return fmt.Errorf("overlay already mounted at %s ", merged)
	}
	for _, d := range []string{upper, merged} {
		if err := os.MkdirAll(d, 0755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	bin := "unionfs"
	if b.HasBin("unionfs3") {
		bin = "unionfs3"
	}
	if !b.HasBin(bin) {
		return errors.New("unionfs binary not found in utils")
	}
	opts := "cow,noatime"
	if relaxedPermissions {
		opts += ",relaxed_permissions"
	}
	// allow_other lets privilege-dropping processes (e.g. pacman → alpm)
	if os.Getuid() == 0 {
		opts += ",allow_other"
	}
	spec := upper + "=RW:" + lower + "=RO"
	cmd := b.Command(ctx, bin, "-o", opts, spec, merged)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unionfs mount: %w", err)
	}
	log.Debug("overlay mounted", "binary", bin, "upper", upper, "lower", lower, "merged", merged, "opts", opts)
	return nil
}

// Unmount falls back to lazy `-uz` so a busy FUSE mount still unwinds.
func Unmount(point string) error {
	if !isMounted(point) {
		return nil
	}
	if err := exec.Command("fusermount", "-u", point).Run(); err == nil {
		return nil
	}
	if err := exec.Command("fusermount", "-uz", point).Run(); err == nil {
		return nil
	}
	log.Debug("fusermount failed, leaving mount as-is", "point", point)
	return nil
}

// IsMounted reports whether `point` is currently a mountpoint.
func IsMounted(point string) bool {
	return isMounted(point)
}

func isMounted(point string) bool {
	if point == "" {
		return false
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	scan := mountinfoScan(data)
	for scan.next() {
		if scan.point() == point {
			return true
		}
	}
	return false
}
