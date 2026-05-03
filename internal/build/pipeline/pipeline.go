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

type ctx struct {
	cfg          *config.Config
	builder      *rootfs.Builder
	rootfsPath   string
	squashfsPath string
	tempDirs     []string
}

// Run runs the four build steps end-to-end.
func Run(c context.Context, cfg *config.Config, meta assembler.BuildMeta) error {
	b := &ctx{cfg: cfg}
	defer b.cleanup()

	if err := b.prepareRootfs(c); err != nil {
		return err
	}
	if err := b.runCommands(c); err != nil {
		return err
	}
	if err := b.applyRootfsOverrides(); err != nil {
		return err
	}
	if err := b.createSquashFS(c); err != nil {
		return err
	}
	return b.assemble(c, meta)
}

func (b *ctx) cleanup() {
	if b.builder != nil {
		b.builder.Cleanup()
		b.builder = nil
	}
	for _, dir := range b.tempDirs {
		if err := os.RemoveAll(dir); err != nil {
			log.Debug("Failed to cleanup temp dir", "path", dir, "error", err)
		}
	}
}

func (b *ctx) prepareRootfs(c context.Context) error {
	log.Info(gotext.Get("Step 1/4: Pulling image and preparing rootfs"), "image", b.cfg.Image)
	builder, err := rootfs.NewBuilder(c, b.cfg.Image)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to prepare rootfs"), err)
	}
	b.builder = builder
	b.rootfsPath = builder.RootfsPath()
	log.Debug("Rootfs mounted", "path", b.rootfsPath)
	return nil
}

func (b *ctx) runCommands(c context.Context) error {
	if len(b.cfg.Install) > 0 {
		log.Info(gotext.Get("Step 2/4: Running install commands"))
		for i, step := range b.cfg.Install {
			log.Info(gotext.Get("Running step"), "num", i+1, "total", len(b.cfg.Install), "name", step.Name)
			if err := b.builder.RunScript(c, step.Run); err != nil {
				return fmt.Errorf("%s: %w", gotext.Get("step %q failed", step.Name), err)
			}
		}
	} else {
		log.Info(gotext.Get("Step 2/4: Skipping commands (none specified)"))
	}

	if err := b.builder.PrepareBindTargets(); err != nil {
		log.Debug("Warning: failed to prepare bind targets", "error", err)
	}
	return nil
}

// applyRootfsOverrides merges /.capsule.overrides.yml into the build config.
func (b *ctx) applyRootfsOverrides() error {
	if err := b.cfg.ApplyOverrides(b.rootfsPath); err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to apply rootfs overrides"), err)
	}
	if err := config.RemoveOverrides(b.rootfsPath); err != nil {
		log.Debug("Failed to remove overrides file", "error", err)
	}
	return nil
}

func (b *ctx) createSquashFS(c context.Context) error {
	log.Info(gotext.Get("Step 3/4: Creating SquashFS image"))
	tmpDir, err := os.MkdirTemp(config.TempDir, "capsule-build-")
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to create temp dir"), err)
	}
	b.tempDirs = append(b.tempDirs, tmpDir)

	compressor := squashfs.NewCompressor(b.cfg.Compression)
	squashfsPath, err := compressor.Compress(c, b.rootfsPath, tmpDir)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to create squashfs"), err)
	}
	b.squashfsPath = squashfsPath
	log.Debug("SquashFS created", "path", squashfsPath)
	return nil
}

func (b *ctx) assemble(c context.Context, meta assembler.BuildMeta) error {
	log.Info(gotext.Get("Step 4/4: Assembling final binary"))

	if dir := filepath.Dir(b.cfg.Output); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("%s: %w", gotext.Get("failed to create output directory"), err)
		}
	}

	a := assembler.NewAssembler()
	if err := a.Assemble(c, b.squashfsPath, b.cfg.Output, b.cfg, meta); err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to assemble binary"), err)
	}

	info, err := os.Stat(b.cfg.Output)
	if err == nil {
		log.Info(gotext.Get("Build complete"), "output", b.cfg.Output, "size_mb", fmt.Sprintf("%.2f", float64(info.Size())/(1024*1024)))
	} else {
		log.Info(gotext.Get("Build complete"), "output", b.cfg.Output)
	}
	return nil
}
