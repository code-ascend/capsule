package store

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"capsule/internal/sys/log"
	"capsule/internal/sys/mountinfo"

	"go.podman.io/storage"
	"go.podman.io/storage/pkg/homedir"
	"go.podman.io/storage/pkg/unshare"
	"golang.org/x/sys/unix"
)

// Options returns capsule's private store options.
func Options() (storage.StoreOptions, error) {
	opts, err := storage.DefaultStoreOptions()
	if err != nil {
		return opts, fmt.Errorf("load storage options: %w", err)
	}
	if opts.GraphRoot, opts.RunRoot, err = roots(); err != nil {
		return opts, err
	}
	opts.ImageStore = ""
	// Native rootless overlay where the kernel supports it; storage falls back to fuse-overlayfs on its own,
	// and keeps using it for stores that were created with it.
	opts.GraphDriverName = "overlay"
	return opts, nil
}

// Open opens capsule's private build store.
func Open() (storage.Store, error) {
	opts, err := Options()
	if err != nil {
		return nil, err
	}
	store, err := storage.GetStore(opts)
	if err != nil {
		return nil, fmt.Errorf("init containers storage at %s: %w (hint: ensure /etc/subuid and /etc/subgid have an entry for $USER)", opts.GraphRoot, err)
	}
	return store, nil
}

// Clean removes capsule's private build store (GraphRoot and RunRoot).
func Clean() error {
	opts, err := Options()
	if err != nil {
		return err
	}
	if store, err := storage.GetStore(opts); err == nil {
		if containers, err := store.Containers(); err == nil {
			for _, container := range containers {
				_, _ = store.Unmount(container.ID, true)
			}
		}
		if layers, err := store.Layers(); err == nil {
			for _, layer := range layers {
				_, _ = store.Unmount(layer.ID, true)
			}
		}
		_, _ = store.Shutdown(true)
	}
	unmountTree(opts.GraphRoot)
	for _, dir := range []string{opts.GraphRoot, opts.RunRoot} {
		if err := os.RemoveAll(dir); err != nil {
			return fmt.Errorf("remove %s: %w", dir, err)
		}
	}
	return nil
}

// roots returns the graph and run roots of capsule's private store: per-user
// XDG directories when rootless, system directories when running as root.
func roots() (graphRoot, runRoot string, err error) {
	if !unshare.IsRootless() || unshare.GetRootlessUID() == 0 {
		return "/var/lib/capsule/storage", "/run/capsule", nil
	}
	dataHome, err := homedir.GetDataHome()
	if err != nil {
		return "", "", fmt.Errorf("resolve XDG data home: %w", err)
	}
	runtimeDir, err := homedir.GetRuntimeDir()
	if err != nil {
		return "", "", fmt.Errorf("resolve XDG runtime dir: %w", err)
	}
	return filepath.Join(dataHome, "capsule", "storage"), filepath.Join(runtimeDir, "capsule"), nil
}

// unmountTree lazily unmounts every mount point at or under root, deepest first.
func unmountTree(root string) {
	all, err := mountinfo.Points()
	if err != nil {
		log.Debug("read mountinfo failed", "error", err)
		return
	}
	var points []string
	for _, point := range all {
		if point == root || strings.HasPrefix(point, root+string(os.PathSeparator)) {
			points = append(points, point)
		}
	}
	sort.Slice(points, func(i, j int) bool { return len(points[i]) > len(points[j]) })
	for _, point := range points {
		if err := unix.Unmount(point, unix.MNT_DETACH); err != nil {
			log.Debug("unmount failed", "path", point, "error", err)
		}
	}
}
