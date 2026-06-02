package nvidia

import (
	"os"
	"path/filepath"
)

type LibLayout struct {
	Lib64 string
	Lib32 string
}

// DetectLayout returns the container's lib layout: Debian multiarch, Fedora lib64, or flat.
func DetectLayout(containerRoot string) LibLayout {
	if isDirAtRoot(containerRoot, "usr/lib/x86_64-linux-gnu") {
		return LibLayout{Lib64: "usr/lib/x86_64-linux-gnu", Lib32: "usr/lib/i386-linux-gnu"}
	}
	if isLib64DistinctFromLib(containerRoot) {
		return LibLayout{Lib64: "usr/lib64", Lib32: "usr/lib"}
	}
	return LibLayout{Lib64: "usr/lib", Lib32: "usr/lib32"}
}

func isDirAtRoot(root, rel string) bool {
	st, err := os.Stat(filepath.Join(root, rel))
	return err == nil && st.IsDir()
}

// isLib64DistinctFromLib rejects Arch's /usr/lib64 → /usr/lib symlink.
func isLib64DistinctFromLib(root string) bool {
	st, err := os.Lstat(filepath.Join(root, "usr/lib64"))
	if err != nil {
		return false
	}
	if !st.IsDir() {
		return false
	}
	return st.Mode()&os.ModeSymlink == 0
}
