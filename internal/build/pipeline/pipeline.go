package pipeline

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"capsule/internal/build/assembler"
	"capsule/internal/build/config"
	"capsule/internal/build/rootfs"
	"capsule/internal/build/squashfs"
	"capsule/internal/sys/log"

	"github.com/leonelquinteros/gotext"
)

type state struct {
	cfg          *config.Config
	builder      *rootfs.Builder
	rootfsPath   string
	squashfsPath string
	tempDirs     []string
}

// Run runs the four build steps end-to-end.
func Run(ctx context.Context, cfg *config.Config, meta config.BuildMeta) error {
	s := &state{cfg: cfg}
	defer s.cleanup()

	if err := s.prepareRootfs(ctx); err != nil {
		return err
	}
	if err := s.runCommands(ctx); err != nil {
		return err
	}
	if err := s.applyRootfsOverrides(); err != nil {
		return err
	}
	if err := s.createSquashFS(ctx); err != nil {
		return err
	}
	return s.assemble(ctx, meta)
}

func (s *state) cleanup() {
	if s.builder != nil {
		s.builder.Cleanup()
		s.builder = nil
	}
	for _, dir := range s.tempDirs {
		if err := os.RemoveAll(dir); err != nil {
			log.Debug("Failed to cleanup temp dir", "path", dir, "error", err)
		}
	}
}

func (s *state) prepareRootfs(ctx context.Context) error {
	log.Info(gotext.Get("Step 1/4: Pulling image and preparing rootfs"), "image", s.cfg.Image)
	builder, err := rootfs.NewBuilder(ctx, s.cfg.Image)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to prepare rootfs"), err)
	}
	s.builder = builder
	s.rootfsPath = builder.RootfsPath()
	log.Debug("Rootfs mounted", "path", s.rootfsPath)
	return nil
}

func (s *state) runCommands(ctx context.Context) error {
	if len(s.cfg.Install) > 0 {
		log.Info(gotext.Get("Step 2/4: Running install commands"))
		for i, step := range s.cfg.Install {
			log.Info(gotext.Get("Running step"), "num", i+1, "total", len(s.cfg.Install), "name", step.Name)
			if err := s.builder.RunScript(ctx, step.Run); err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("step %q failed", step.Name), err)
			}
		}
	} else {
		log.Info(gotext.Get("Step 2/4: Skipping commands (none specified)"))
	}

	if err := s.builder.PrepareBindTargets(); err != nil {
		log.Debug("Warning: failed to prepare bind targets", "error", err)
	}
	return nil
}

// applyRootfsOverrides merges /.capsule.overrides.yml into the build config.
func (s *state) applyRootfsOverrides() error {
	if err := s.cfg.ApplyOverrides(s.rootfsPath); err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to apply rootfs overrides"), err)
	}
	if err := config.RemoveOverrides(s.rootfsPath); err != nil {
		log.Debug("Failed to remove overrides file", "error", err)
	}
	return nil
}

func (s *state) createSquashFS(ctx context.Context) error {
	log.Info(gotext.Get("Step 3/4: Creating SquashFS image"))
	tmpDir, err := os.MkdirTemp(config.TempDir, "capsule-build-")
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to create temp dir"), err)
	}
	s.tempDirs = append(s.tempDirs, tmpDir)

	compressor := squashfs.NewCompressor(s.cfg.Compression)
	squashfsPath, err := compressor.Compress(ctx, s.rootfsPath, tmpDir)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to create squashfs"), err)
	}
	s.squashfsPath = squashfsPath
	log.Debug("SquashFS created", "path", squashfsPath)
	return nil
}

func (s *state) assemble(ctx context.Context, meta config.BuildMeta) error {
	log.Info(gotext.Get("Step 4/4: Assembling final binary"))

	if dir := filepath.Dir(s.cfg.Output); dir != "." {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			return fmt.Errorf("%s: %w", gotext.Get("failed to create output directory"), err)
		}
	}

	a := assembler.NewAssembler()
	if err := a.Assemble(ctx, s.squashfsPath, s.cfg.Output, s.cfg, meta); err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to assemble binary"), err)
	}

	info, err := os.Stat(s.cfg.Output)
	if err == nil {
		log.Info(gotext.Get("Build complete"), "output", s.cfg.Output, "size_mb", fmt.Sprintf("%.2f", float64(info.Size())/(1024*1024)))
	} else {
		log.Info(gotext.Get("Build complete"), "output", s.cfg.Output)
	}
	return nil
}
