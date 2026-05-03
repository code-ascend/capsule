package session

import (
	"context"
	"fmt"
	"os"

	"capsule/internal/runtime/bundle"
	"capsule/internal/runtime/mount"
	"capsule/internal/runtime/nvidia"
	"capsule/internal/runtime/overlay"
	"capsule/internal/runtime/userfiles"
	"capsule/internal/runtime/workspace"
	"capsule/internal/sys/log"
)

// Options are CLI-driven knobs that affect a single session.
type Options struct {
	NoOverlay  bool
	NoNvidia   bool
	SquashFuse string
}

// Session bundles per-invocation runtime resources.
type Session struct {
	selfPath  string
	offset    int64
	opts      Options
	workspace *workspace.Workspace
	bundle    *bundle.Extractor
	mounter   *mount.Mounter
}

// Overlay describes an active unionfs layer.
type Overlay struct {
	Loc      *overlay.Locator
	RootPath string
}

// Open prepares the workspace and bundle for capsulePath.
func Open(capsulePath string, squashfsOffset int64, opts Options) (*Session, error) {
	ws, err := workspace.New(capsulePath)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	bun := bundle.New(ws.UtilsPath())
	return &Session{
		selfPath:  capsulePath,
		offset:    squashfsOffset,
		opts:      opts,
		workspace: ws,
		bundle:    bun,
		mounter:   &mount.Mounter{Bundle: bun, SquashFuse: opts.SquashFuse},
	}, nil
}

// Bundle returns the per-session util extractor.
func (s *Session) Bundle() *bundle.Extractor { return s.bundle }

// Workspace returns the per-session workspace.
func (s *Session) Workspace() *workspace.Workspace { return s.workspace }

// MountRoot extracts utils and FUSE-mounts the squashfs rootfs.
func (s *Session) MountRoot(ctx context.Context) (string, error) {
	if err := s.bundle.Extract(); err != nil {
		return "", fmt.Errorf("extract utils: %w", err)
	}
	mnt := s.workspace.MntPath()
	if err := s.mounter.Squashfs(ctx, s.selfPath, s.offset, mnt); err != nil {
		return "", err
	}
	s.workspace.AddCleanup(func() error { return mount.Unmount(mnt) })
	return mnt, nil
}

// EnableOverlay mounts a writable unionfs over rootPath.
func (s *Session) EnableOverlay(ctx context.Context, rootPath string) (*Overlay, error) {
	if s.opts.NoOverlay {
		return nil, nil
	}
	loc := overlay.New(s.selfPath)
	if err := loc.EnsureDirs(); err != nil {
		return nil, err
	}

	if host, err := userfiles.LookupHost(); err != nil {
		log.Debug("host user files lookup failed", "error", err)
	} else if err = host.EnsureOverlayUser(rootPath, loc.EtcDir()); err != nil {
		log.Debug("overlay user files init failed", "error", err)
	}

	relaxed := os.Getuid() != 0
	if err := s.mounter.Overlay(ctx, loc.Upper(), rootPath, loc.Merged(), relaxed); err != nil {
		log.Warn("unionfs overlay disabled", "error", err)
		return nil, nil
	}
	s.workspace.AddCleanup(func() error { return mount.Unmount(loc.Merged()) })

	if !s.opts.NoNvidia {
		if err := nvidia.Setup(ctx, s.bundle, loc.Merged(), loc.VersionMarker("nvidia_version")); err != nil {
			log.Warn("nvidia setup failed", "error", err)
		}
	}
	return &Overlay{Loc: loc, RootPath: loc.Merged()}, nil
}

// StageMergedUserFiles writes merged passwd/group/shadow for read-only roots.
func (s *Session) StageMergedUserFiles(rootPath string) (string, error) {
	host, err := userfiles.LookupHost()
	if err != nil {
		return "", err
	}
	if err = host.MergeFromRoot(rootPath, s.workspace.EtcPath()); err != nil {
		return "", err
	}
	return s.workspace.EtcPath(), nil
}

// Close drops the session sentinel and tears down shared mounts when last.
func (s *Session) Close() {
	s.workspace.Cleanup()
}
