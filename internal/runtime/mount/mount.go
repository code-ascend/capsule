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

// Mounter owns shared mount dependencies and per-invocation tuning options.
type Mounter struct {
	Bundle     *bundle.Extractor
	SquashFuse string
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
	if err := os.MkdirAll(mountPoint, 0755); err != nil {
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
		if err := os.MkdirAll(d, 0755); err != nil {
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
func pickSquashFuse(b *bundle.Extractor, pref string) string {
	switch pref {
	case "ll":
		if b.HasBin("squashfuse_ll") {
			return "squashfuse_ll"
		}
	case "3":
		if b.HasBin("squashfuse3") {
			return "squashfuse3"
		}
	}
	if b.HasBin("squashfuse3") {
		return "squashfuse3"
	}
	if b.HasBin("squashfuse_ll") {
		return "squashfuse_ll"
	}
	return "squashfuse"
}

// Unmount drops point via fusermount
func Unmount(point string) error {
	for IsMounted(point) {
		out, err := exec.Command("fusermount", "-uz", point).CombinedOutput()
		if err != nil && IsMounted(point) {
			log.Debug("fusermount -uz failed", "point", point, "err", err, "stderr", string(out))
			return nil
		}
	}
	return nil
}

// IsMounted reports whether `point` is currently a mountpoint.
func IsMounted(point string) bool {
	if point == "" {
		return false
	}
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return false
	}
	scan := mountInfoScan(data)
	for scan.next() {
		if scan.point() == point {
			return true
		}
	}
	return false
}
