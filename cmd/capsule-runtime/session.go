package main

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

type session struct {
	state     *appState
	workspace *workspace.Workspace
	bundle    *bundle.Extractor
	mounter   *mount.Mounter
	opts      runOptions
}

type mountResult struct {
	RootPath string
}

type overlayResult struct {
	Loc      *overlay.Locator
	RootPath string
}

func openSession(state *appState, opts runOptions) (*session, error) {
	ws, err := workspace.New(state.selfPath)
	if err != nil {
		return nil, fmt.Errorf("workspace: %w", err)
	}
	bun := bundle.New(ws.UtilsPath())
	s := &session{
		state:     state,
		workspace: ws,
		bundle:    bun,
		mounter:   &mount.Mounter{Bundle: bun, SquashFuse: opts.SquashFuse},
		opts:      opts,
	}
	ws.AddCleanup(func() error { return mount.Unmount(ws.MntPath()) })
	return s, nil
}

func (s *session) mountRoot(ctx context.Context) (*mountResult, error) {
	if err := s.bundle.Extract(); err != nil {
		return nil, fmt.Errorf("extract utils: %w", err)
	}
	if err := s.mounter.Squashfs(ctx, s.state.selfPath, s.state.layout.SquashfsOffset, s.workspace.MntPath()); err != nil {
		return nil, err
	}
	return &mountResult{RootPath: s.workspace.MntPath()}, nil
}

// enableOverlay returns (nil, nil) when overlay is disabled or unionfs fails;
// caller falls back to the read-only mountResult.
func (s *session) enableOverlay(ctx context.Context, m *mountResult) (*overlayResult, error) {
	if s.opts.NoOverlay {
		return nil, nil
	}
	loc := overlay.New(s.state.selfPath)
	if err := loc.EnsureDirs(); err != nil {
		return nil, err
	}

	if host, err := userfiles.LookupHost(); err == nil {
		if err = host.EnsureOverlayUser(m.RootPath, loc.EtcDir()); err != nil {
			log.Debug("overlay user files init failed", "error", err)
		}
	}

	relaxed := os.Getuid() != 0
	if err := s.mounter.Overlay(ctx, loc.Upper(), m.RootPath, loc.Merged(), relaxed); err != nil {
		log.Warn("unionfs overlay disabled", "error", err)
		return nil, nil
	}
	s.workspace.AddCleanup(func() error { return mount.Unmount(loc.Merged()) })

	if !s.opts.NoNvidia {
		if err := nvidia.Setup(ctx, s.bundle, loc.Merged(), loc.VersionMarker("nvidia_version")); err != nil {
			log.Warn("nvidia setup failed", "error", err)
		}
	}
	return &overlayResult{Loc: loc, RootPath: loc.Merged()}, nil
}

// stageMergedUserFiles is the no-overlay path; with overlay up the merged
// files already live in the upper.
func (s *session) stageMergedUserFiles(m *mountResult) (string, error) {
	host, err := userfiles.LookupHost()
	if err != nil {
		return "", err
	}
	if err = host.MergeFromRoot(m.RootPath, s.workspace.EtcPath()); err != nil {
		return "", err
	}
	return s.workspace.EtcPath(), nil
}

func (s *session) close() {
	s.workspace.Cleanup()
}
