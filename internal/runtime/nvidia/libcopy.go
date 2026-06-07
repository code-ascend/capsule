package nvidia

import (
	"debug/elf"
	"errors"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"capsule/internal/sys/log"
)

// CollectLibPaths picks NVIDIA-related libs from ldconfig output, sniffing libnv* to exclude lookalikes.
func CollectLibPaths(entries []LdEntry) []string {
	seen := map[string]struct{}{}
	add := func(p string) {
		if p == "" {
			return
		}
		seen[p] = struct{}{}
	}

	for _, e := range entries {
		lower := strings.ToLower(e.Soname + " " + e.Path)
		switch {
		case strings.Contains(lower, "nvidia"):
			add(e.Path)
		case strings.HasPrefix(strings.ToLower(e.Soname), "libcuda"):
			add(e.Path)
		case strings.HasPrefix(strings.ToLower(e.Soname), "libnv") && libContainsNvidia(e.Path):
			add(e.Path)
		}
	}

	out := make([]string, 0, len(seen))
	for p := range seen {
		out = append(out, p)
	}
	sort.Strings(out)
	return out
}

func libContainsNvidia(path string) bool {
	f, err := elf.Open(path)
	if err != nil {
		return false
	}
	defer func() { _ = f.Close() }()
	for _, sect := range []string{".rodata", ".dynstr", ".comment"} {
		s := f.Section(sect)
		if s == nil {
			continue
		}
		data, err := s.Data()
		if err != nil {
			continue
		}
		if strings.Contains(strings.ToLower(string(data)), "nvidia") {
			return true
		}
	}
	return false
}

func CopyLib(src, containerRoot string, layout LibLayout, driverVersion string) (string, error) {
	if shouldSkipVersionedLib(filepath.Base(src), driverVersion) {
		return "", nil
	}
	is64, err := elfIs64(src)
	if err != nil {
		log.Debug("elf detect failed; defaulting to 64-bit", "src", src, "err", err)
		is64 = true
	}
	dir := layout.Lib64
	if !is64 {
		dir = layout.Lib32
	}
	dst := filepath.Join(containerRoot, dir, filepath.Base(src))
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return "", err
	}
	if err := copyFollowSymlink(src, dst); err != nil {
		return "", err
	}
	return dst, nil
}

// shouldSkipVersionedLib drops other drivers' `.so.X.Y.Z` libs, keeping soname links untouched.
func shouldSkipVersionedLib(fname, version string) bool {
	_, suffix, found := strings.Cut(fname, ".so.")
	if !found {
		return false
	}
	if !looksLikeFullVersion(suffix) {
		return false
	}
	return suffix != version
}

func looksLikeFullVersion(s string) bool {
	dots := 0
	for _, r := range s {
		if r == '.' {
			dots++
			continue
		}
		if r < '0' || r > '9' {
			return false
		}
	}
	return dots >= 1
}

func elfIs64(path string) (bool, error) {
	f, err := elf.Open(path)
	if err != nil {
		return false, err
	}
	defer func() { _ = f.Close() }()
	return f.Class == elf.ELFCLASS64, nil
}

// copyFollowSymlink — equivalent of `cp -fL`.
func copyFollowSymlink(src, dst string) error {
	resolved, err := filepath.EvalSymlinks(src)
	if err != nil {
		return err
	}
	in, err := os.Open(resolved)
	if err != nil {
		return err
	}
	defer func() { _ = in.Close() }()
	st, err := in.Stat()
	if err != nil {
		return err
	}
	if !st.Mode().IsRegular() {
		return errors.New("not a regular file: " + resolved)
	}
	if err := os.RemoveAll(dst); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, st.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err = io.Copy(out, in); err != nil {
		_ = out.Close()
		return err
	}
	return out.Close()
}
