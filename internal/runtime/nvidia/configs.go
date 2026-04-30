package nvidia

import (
	"capsule/internal/sys/fsutil"
	"os"
	"path/filepath"
	"strings"

	"capsule/internal/sys/log"
)

var configFiles = []string{
	"/usr/share/vulkan/icd.d/nvidia_icd.json",
	"/usr/share/vulkan/implicit_layer.d/nvidia_layers.json",
	"/usr/lib/nvidia/wine/nvngx.dll",
	"/usr/lib/nvidia/wine/_nvngx.dll",
	"/usr/share/egl/egl_external_platform.d/20_nvidia_xcb.json",
	"/usr/share/nvidia/nvidia-application-profiles-{version}-rc",
}

func CopyConfigs(containerRoot, version string) {
	for _, f := range configFiles {
		f = strings.ReplaceAll(f, "{version}", version)
		dir := filepath.Dir(f)
		name := filepath.Base(f)
		found := findFile(name, f)
		if found == "" {
			continue
		}
		dst := filepath.Join(containerRoot, dir)
		if err := os.MkdirAll(dst, 0755); err == nil {
			_ = copyFollowSymlink(found, filepath.Join(dst, filepath.Base(found)))
		}
		copyNvidiaSiblings(filepath.Dir(found), dst)
		log.Debug("nvidia config copied", "name", name, "from", found, "to", dst)
	}
}

// CopyEGLVendor forces 10_nvidia.json to win over Mesa's 50_mesa.json.
func CopyEGLVendor(containerRoot string) {
	dst := filepath.Join(containerRoot, "usr/share/glvnd/egl_vendor.d")
	src := findFile("10_nvidia.json", "/usr/share/glvnd/egl_vendor.d/10_nvidia.json")
	if src == "" {
		for _, p := range findByPathPattern(SearchRoots, "/egl_vendor.d/") {
			if strings.Contains(filepath.Base(p), "nvidia") {
				src = p
				break
			}
		}
	}
	if src == "" {
		return
	}
	if err := os.MkdirAll(dst, 0755); err == nil {
		_ = copyFollowSymlink(src, filepath.Join(dst, "10_nvidia.json"))
	}
}

func CopyEGLPlatform(containerRoot string) {
	dst := filepath.Join(containerRoot, "usr/share/egl/egl_external_platform.d")
	if hasNvidiaFile(dst) {
		return
	}
	for _, p := range findByPathPattern(SearchRoots, "/egl_external_platform.d/") {
		if !strings.Contains(filepath.Base(p), "nvidia") {
			continue
		}
		if err := os.MkdirAll(dst, 0755); err == nil {
			_ = copyFollowSymlink(p, filepath.Join(dst, filepath.Base(p)))
		}
	}
}

func CopyVulkanFallbacks(containerRoot string) {
	icdDst := filepath.Join(containerRoot, "usr/share/vulkan/icd.d")
	if !fsutil.Exists(filepath.Join(icdDst, "nvidia_icd.json")) {
		if src := findFile("nvidia_icd.json", ""); src != "" {
			_ = os.MkdirAll(icdDst, 0755)
			_ = copyFollowSymlink(src, filepath.Join(icdDst, filepath.Base(src)))
			copyNvidiaSiblings(filepath.Dir(src), icdDst)
		}
	}
	layerDst := filepath.Join(containerRoot, "usr/share/vulkan/implicit_layer.d")
	if !fsutil.Exists(filepath.Join(layerDst, "nvidia_layers.json")) {
		if src := findFile("nvidia_layers.json", ""); src != "" {
			_ = os.MkdirAll(layerDst, 0755)
			_ = copyFollowSymlink(src, filepath.Join(layerDst, filepath.Base(src)))
		}
	}
}

// CopyWineDLSS handles nixpkgs's non-standard nvngx.dll location.
func CopyWineDLSS(containerRoot string) {
	dst := filepath.Join(containerRoot, "usr/lib/nvidia/wine")
	if fsutil.Exists(filepath.Join(dst, "nvngx.dll")) {
		return
	}
	src := ""
	for _, p := range findByPathPattern([]string{"/usr", "/run", "/nix"}, "/nvidia/wine/") {
		if filepath.Base(p) == "nvngx.dll" {
			src = p
			break
		}
	}
	if src == "" {
		return
	}
	if err := os.MkdirAll(dst, 0755); err != nil {
		return
	}
	for _, sibling := range siblings(filepath.Dir(src)) {
		if !strings.Contains(filepath.Base(sibling), "nvngx") {
			continue
		}
		_ = copyFollowSymlink(sibling, filepath.Join(dst, filepath.Base(sibling)))
	}
}

// CopyWaylandServerLib — libnvidia-egl-wayland needs it on the host side.
func CopyWaylandServerLib(containerRoot string, layout LibLayout, entries []LdEntry) {
	for _, e := range entries {
		if !strings.Contains(e.Soname, "libwayland-server.so") {
			continue
		}
		dst := filepath.Join(containerRoot, layout.Lib64, filepath.Base(e.Path))
		_ = os.MkdirAll(filepath.Dir(dst), 0755)
		_ = copyFollowSymlink(e.Path, dst)
		return
	}
}

func copyNvidiaSiblings(srcDir, dstDir string) {
	for _, p := range siblings(srcDir) {
		if !strings.Contains(filepath.Base(p), "nvidia") {
			continue
		}
		_ = copyFollowSymlink(p, filepath.Join(dstDir, filepath.Base(p)))
	}
}

func siblings(dir string) []string {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		out = append(out, filepath.Join(dir, e.Name()))
	}
	return out
}

func hasNvidiaFile(dir string) bool {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, e := range entries {
		if !e.IsDir() && strings.Contains(e.Name(), "nvidia") {
			return true
		}
	}
	return false
}
