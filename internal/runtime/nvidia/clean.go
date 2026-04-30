package nvidia

import (
	"capsule/internal/sys/fsutil"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

var libGlobs = []string{
	"libnvidia*",
	"libcuda*",
	"libnvcuvid*",
	"libnvoptix*",
	"libvdpau_nvidia*",
	"libGLX_nvidia*",
	"libEGL_nvidia*",
	"libGLES*_nvidia*",
	"libnvrtc*",
	"libnv*.so*",
	"libwayland-server.so*",
}

var libDirs = []string{
	"usr/lib",
	"usr/lib32",
	"usr/lib64",
	"usr/lib/x86_64-linux-gnu",
	"usr/lib/i386-linux-gnu",
}

var nvidiaBinaries = []string{
	"nvidia-smi", "nvidia-debugdump", "nvidia-persistenced",
	"nvidia-cuda-mps-control", "nvidia-cuda-mps-server",
}

// CleanUpper strips all host-injected NVIDIA files before --commit so the
// resulting squashfs is portable.
func CleanUpper(upper string) error {
	if _, err := os.Stat(upper); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}

	for _, dir := range libDirs {
		full := filepath.Join(upper, dir)
		if fsutil.IsDir(full) {
			removeMatching(full, libGlobs)
		}
	}

	for _, b := range nvidiaBinaries {
		_ = os.Remove(filepath.Join(upper, "usr/bin", b))
	}

	for _, p := range []string{
		"usr/share/vulkan/icd.d",
		"usr/share/vulkan/implicit_layer.d",
		"usr/share/glvnd/egl_vendor.d",
		"usr/share/egl/egl_external_platform.d",
	} {
		removeFilesContaining(filepath.Join(upper, p), "nvidia")
	}
	_ = os.RemoveAll(filepath.Join(upper, "usr/lib/nvidia/wine"))
	removeFilesPrefix(filepath.Join(upper, "usr/share/nvidia"), "nvidia-application-profiles-")

	for _, dri := range []string{
		"usr/lib/dri", "usr/lib32/dri", "usr/lib64/dri",
		"usr/lib/x86_64-linux-gnu/dri", "usr/lib/i386-linux-gnu/dri",
	} {
		_ = os.Remove(filepath.Join(upper, dri, "nvidia_drv_video.so"))
		_ = os.RemoveAll(filepath.Join(upper, dri, "nvidia-vaapi-driver"))
	}
	for _, gbm := range []string{
		"usr/lib64/gbm", "usr/lib/gbm", "usr/lib/x86_64-linux-gnu/gbm",
	} {
		removeMatching(filepath.Join(upper, gbm), []string{"nvidia*"})
	}
	for _, p := range []string{
		"etc/X11/lib_nvidia", "etc/X11/lib64_nvidia", "etc/X11/lib32_nvidia",
		"usr/lib/nvidia-current",
	} {
		_ = os.RemoveAll(filepath.Join(upper, p))
	}
	return nil
}

func removeMatching(dir string, patterns []string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		for _, pat := range patterns {
			if matched, _ := filepath.Match(pat, e.Name()); matched {
				_ = os.Remove(filepath.Join(dir, e.Name()))
				break
			}
		}
	}
}

func removeFilesContaining(dir, sub string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(e.Name(), sub) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}

func removeFilesPrefix(dir, prefix string) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	for _, e := range entries {
		if !e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
			_ = os.Remove(filepath.Join(dir, e.Name()))
		}
	}
}
