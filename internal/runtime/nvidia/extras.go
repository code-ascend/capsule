package nvidia

import (
	"os"
	"path/filepath"
	"strings"

	"capsule/internal/runtime/fsutil"
)

var driDirs = []string{
	"/usr/lib/dri",
	"/usr/lib64/dri",
	"/usr/lib/x86_64-linux-gnu/dri",
	"/usr/lib32/dri",
	"/usr/lib/i386-linux-gnu/dri",
}

var gbmDirs = []string{
	"/usr/lib64/gbm",
	"/usr/lib/gbm",
	"/usr/lib/x86_64-linux-gnu/gbm",
}

func CopyDRIVAAPI(containerRoot string, layout LibLayout) {
	for _, d := range driDirs {
		if !fsutil.IsDir(d) {
			continue
		}
		drv := filepath.Join(d, "nvidia_drv_video.so")
		if fsutil.Exists(drv) {
			dst := filepath.Join(containerRoot, d)
			_ = os.MkdirAll(dst, 0755)
			_ = copyFollowSymlink(drv, filepath.Join(dst, "nvidia_drv_video.so"))
		}
		bundle := filepath.Join(d, "nvidia-vaapi-driver")
		if fsutil.IsDir(bundle) {
			_ = copyTreeFollow(bundle, filepath.Join(containerRoot, d, "nvidia-vaapi-driver"))
		}
	}
	// Cross-distro fallback: ensure the primary lib has at least one driver.
	primary := filepath.Join(containerRoot, layout.Lib64, "dri")
	if !fsutil.Exists(filepath.Join(primary, "nvidia_drv_video.so")) {
		for _, d := range driDirs {
			cand := filepath.Join(containerRoot, d, "nvidia_drv_video.so")
			if fsutil.Exists(cand) {
				_ = os.MkdirAll(primary, 0755)
				_ = copyFollowSymlink(cand, filepath.Join(primary, "nvidia_drv_video.so"))
				break
			}
		}
	}
}

func CopyGBM(containerRoot string, layout LibLayout) {
	for _, d := range gbmDirs {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		dst := filepath.Join(containerRoot, d)
		for _, e := range entries {
			if !strings.Contains(e.Name(), "nvidia") {
				continue
			}
			_ = os.MkdirAll(dst, 0755)
			_ = copyFollowSymlink(filepath.Join(d, e.Name()), filepath.Join(dst, e.Name()))
		}
	}
	primary := filepath.Join(containerRoot, layout.Lib64, "gbm")
	if hasNvidiaFile(primary) {
		return
	}
	for _, d := range gbmDirs {
		src := filepath.Join(containerRoot, d)
		if !hasNvidiaFile(src) {
			continue
		}
		_ = os.MkdirAll(primary, 0755)
		for _, p := range siblings(src) {
			if !strings.Contains(filepath.Base(p), "nvidia") {
				continue
			}
			_ = copyFollowSymlink(p, filepath.Join(primary, filepath.Base(p)))
		}
		return
	}
}

// CopyALTNonStandard handles ALT Linux's symlinked NVIDIA lib dirs.
func CopyALTNonStandard(containerRoot string) {
	for _, p := range []string{
		"/etc/X11/lib_nvidia",
		"/etc/X11/lib64_nvidia",
		"/etc/X11/lib32_nvidia",
		"/usr/lib/nvidia-current",
	} {
		if _, err := os.Lstat(p); err != nil {
			continue
		}
		dst := filepath.Join(containerRoot, filepath.Dir(p))
		_ = os.MkdirAll(dst, 0755)
		_ = copyTreeFollow(p, filepath.Join(dst, filepath.Base(p)))
	}
}

// CopyALTMesaDRI ports ALT's Mesa DRI (lives under X11/modules/dri instead of dri).
func CopyALTMesaDRI(containerRoot string) {
	src := filepath.Join(containerRoot, "usr/lib64/X11/modules/dri")
	if !fsutil.IsDir(src) {
		return
	}
	dst := filepath.Join(containerRoot, "usr/lib64/dri")
	_ = os.MkdirAll(dst, 0755)
	for _, p := range siblings(src) {
		name := filepath.Base(p)
		if !strings.HasSuffix(name, "_dri.so") {
			continue
		}
		if fsutil.Exists(filepath.Join(dst, name)) {
			continue
		}
		_ = copyFollowSymlink(p, filepath.Join(dst, name))
	}
}

func copyTreeFollow(src, dst string) error {
	st, err := os.Lstat(src)
	if err != nil {
		return err
	}
	if st.Mode()&os.ModeSymlink != 0 {
		resolved, err := filepath.EvalSymlinks(src)
		if err != nil {
			return err
		}
		return copyTreeFollow(resolved, dst)
	}
	if !st.IsDir() {
		return copyFollowSymlink(src, dst)
	}
	if err := os.MkdirAll(dst, st.Mode().Perm()); err != nil {
		return err
	}
	entries, err := os.ReadDir(src)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if err := copyTreeFollow(filepath.Join(src, e.Name()), filepath.Join(dst, e.Name())); err != nil {
			return err
		}
	}
	return nil
}
