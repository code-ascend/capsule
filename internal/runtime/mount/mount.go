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
	"capsule/internal/sys/mountinfo"

	"golang.org/x/sys/unix"
)

// SquashFuse selects the squashfuse backend.
type SquashFuse string

const (
	SquashFuseAuto SquashFuse = ""
	SquashFuse3    SquashFuse = "3"
	SquashFuseLL   SquashFuse = "ll"
)

const (
	binSquashfuse   = "squashfuse"
	binSquashfuse3  = "squashfuse3"
	binSquashfuseLL = "squashfuse_ll"
)

// ParseSquashFuse validates a backend name; "" and "auto" mean SquashFuseAuto.
func ParseSquashFuse(v string) (SquashFuse, error) {
	switch v {
	case "", "auto":
		return SquashFuseAuto, nil
	case "3":
		return SquashFuse3, nil
	case "ll":
		return SquashFuseLL, nil
	}
	return "", fmt.Errorf("invalid squashfuse backend %q (valid: auto, 3, ll)", v)
}

// Mounter owns shared mount dependencies and per-invocation tuning options.
type Mounter struct {
	Bundle     *bundle.Extractor
	SquashFuse SquashFuse
}

// New creates a Mounter bound to b.
func New(b *bundle.Extractor) *Mounter {
	return &Mounter{Bundle: b}
}

// Squashfs FUSE-mounts the squashfs payload of capsulePath at mountPoint.
func (m *Mounter) Squashfs(ctx context.Context, capsulePath string, offset int64, mountPoint string) error {
	if IsMounted(mountPoint) {
		return nil
	}
	if err := os.MkdirAll(mountPoint, 0o755); err != nil {
		return fmt.Errorf("mkdir mountpoint: %w", err)
	}
	bin := pickSquashFuse(m.Bundle, m.SquashFuse)
	opts := "offset=" + strconv.FormatInt(offset, 10)
	if os.Getuid() == 0 {
		opts += ",allow_other"
	}
	cmd := m.Bundle.Command(ctx, bin, "-o", opts, capsulePath, mountPoint)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("squashfuse mount: %w", err)
	}
	log.Debug("squashfs mounted", "binary", bin, "offset", offset, "mount", mountPoint, "opts", opts)
	return nil
}

// Overlay FUSE-mounts unionfs over lower with upper as RW layer.
func (m *Mounter) Overlay(ctx context.Context, upper, lower, merged string, relaxedPermissions bool) error {
	if IsMounted(merged) {
		log.Debug("overlay already mounted, reusing", "merged", merged)
		return nil
	}
	for _, d := range []string{upper, merged} {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return fmt.Errorf("mkdir %s: %w", d, err)
		}
	}
	bin := "unionfs"
	if m.Bundle.HasBin("unionfs3") {
		bin = "unionfs3"
	}
	if !m.Bundle.HasBin(bin) {
		return errors.New("unionfs binary not found in utils")
	}
	opts := "cow,noatime"
	if relaxedPermissions {
		opts += ",relaxed_permissions"
	}
	if os.Getuid() == 0 {
		opts += ",allow_other"
	}
	spec := upper + "=RW:" + lower + "=RO"
	cmd := m.Bundle.Command(ctx, bin, "-o", opts, spec, merged)
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("unionfs mount: %w", err)
	}
	log.Debug("overlay mounted", "binary", bin, "upper", upper, "lower", lower, "merged", merged, "opts", opts)
	return nil
}

// pickSquashFuse selects the squashfuse binary honoring pref, with fallback.
func pickSquashFuse(b *bundle.Extractor, pref SquashFuse) string {
	switch pref {
	case SquashFuseLL:
		if b.HasBin(binSquashfuseLL) {
			return binSquashfuseLL
		}
	case SquashFuse3:
		if b.HasBin(binSquashfuse3) {
			return binSquashfuse3
		}
	case SquashFuseAuto:
	}
	if b.HasBin(binSquashfuse3) {
		return binSquashfuse3
	}
	if b.HasBin(binSquashfuseLL) {
		return binSquashfuseLL
	}
	return binSquashfuse
}

// Unmount drops point via fusermount, falling back to a lazy detach syscall.
func Unmount(point string) error {
	for IsMounted(point) {
		if fusermount(point) {
			continue
		}
		if err := unix.Unmount(point, unix.MNT_DETACH); err != nil && IsMounted(point) {
			log.Debug("lazy unmount failed", "point", point, "err", err)
			return nil
		}
	}
	return nil
}

// fusermount tries the fuse unmount helpers available on the host.
func fusermount(point string) bool {
	for _, tool := range []string{"fusermount", "fusermount3"} {
		out, err := exec.Command(tool, "-uz", point).CombinedOutput()
		if err == nil {
			return true
		}
		log.Debug("fusermount failed", "tool", tool, "point", point, "err", err, "stderr", string(out))
	}
	return false
}

// IsMounted reports whether `point` is currently a mountpoint.
func IsMounted(point string) bool {
	return mountinfo.IsMounted(point)
}
