package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"capsule/internal/build/assembler"
	"capsule/internal/build/config"
	"capsule/internal/build/rootfs"
	"capsule/internal/build/squashfs"
	"capsule/internal/i18n"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"
	"capsule/internal/version"

	"github.com/containers/buildah"
	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
	"go.podman.io/storage/pkg/unshare"
)

type buildContext struct {
	cfg          *config.Config
	builder      *rootfs.Builder
	rootfsPath   string
	squashfsPath string
	tempDirs     []string
}

func main() {
	if buildah.InitReexec() {
		return
	}
	unshare.MaybeReexecUsingUserNamespace(false)
	os.Exit(run())
}

func run() int {
	i18n.Setup()
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rootCmd := &cli.Command{
		Name:    "capsule",
		Version: version.Version,
		Usage:   gotext.Get("Create portable Linux containers from OCI images"),
		Description: gotext.Get(`capsule is a tool for creating portable Linux containers as single ELF executables.
It reads a YAML config file specifying the image and commands, then produces a self-contained binary.`),
		Commands: []*cli.Command{
			{
				Name:        "build",
				Usage:       gotext.Get("Build a portable container from an OCI image"),
				ArgsUsage:   "[config.yaml]",
				Description: gotext.Get("Build a portable container from an OCI image using a YAML config file."),
				Action:      runBuild,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   gotext.Get("Path to YAML config file"),
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   gotext.Get("Output file path (overrides config)"),
					},
					&cli.StringFlag{
						Name:  "compression",
						Usage: gotext.Get("SquashFS compression: zstd, lz4, gzip, xz (overrides config)"),
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   gotext.Get("Verbose output"),
					},
				},
			},
		},
	}

	if err := rootCmd.Run(ctx, os.Args); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, gotext.Get("Interrupted"))
			return exitcode.Interrupted
		}
		fmt.Fprintf(os.Stderr, "%s: %v\n", gotext.Get("Error"), err)
		return exitcode.Error
	}
	return exitcode.OK
}

func runBuild(ctx context.Context, cmd *cli.Command) error {
	verbose := cmd.Bool("verbose")
	log.Init(verbose)

	configPath := cmd.Args().First()
	if configPath == "" {
		configPath = cmd.String("config")
	}
	if configPath == "" {
		return errors.New(gotext.Get("config file required. Usage: capsule build <config.yaml> or capsule build -c <config.yaml>"))
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to load config"), err)
	}

	if output := cmd.String("output"); output != "" {
		cfg.Output = output
	}
	if compression := cmd.String("compression"); compression != "" {
		cfg.Compression = compression
	}

	log.Debug("Build configuration",
		"image", cfg.Image,
		"output", cfg.Output,
		"compression", cfg.Compression,
		"install_steps", len(cfg.Install),
	)

	b := &buildContext{cfg: cfg}
	defer b.cleanup()

	if err = b.prepareRootfs(ctx); err != nil {
		return err
	}
	if err = b.runCommands(ctx); err != nil {
		return err
	}
	if err = b.applyRootfsOverrides(); err != nil {
		return err
	}
	if err = b.createSquashFS(ctx); err != nil {
		return err
	}
	if err = b.assemble(ctx); err != nil {
		return err
	}

	return nil
}

func (b *buildContext) cleanup() {
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

func (b *buildContext) prepareRootfs(ctx context.Context) error {
	log.Info(gotext.Get("Step 1/4: Pulling image and preparing rootfs"), "image", b.cfg.Image)
	builder, err := rootfs.NewBuilder(ctx, b.cfg.Image)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to prepare rootfs"), err)
	}
	b.builder = builder
	b.rootfsPath = builder.RootfsPath()
	log.Debug("Rootfs mounted", "path", b.rootfsPath)
	return nil
}

func (b *buildContext) runCommands(ctx context.Context) error {
	if len(b.cfg.Install) > 0 {
		log.Info(gotext.Get("Step 2/4: Running install commands"))
		for i, step := range b.cfg.Install {
			log.Info(gotext.Get("Running step"), "num", i+1, "total", len(b.cfg.Install), "name", step.Name)
			if err := b.builder.RunScript(ctx, step.Run); err != nil {
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

// applyRootfsOverrides merges /.capsule.overrides.yml from the rootfs into the build config.
func (b *buildContext) applyRootfsOverrides() error {
	if err := b.cfg.ApplyOverrides(b.rootfsPath); err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to apply rootfs overrides"), err)
	}
	if err := config.RemoveOverrides(b.rootfsPath); err != nil {
		log.Debug("Failed to remove overrides file", "error", err)
	}
	return nil
}

func (b *buildContext) createSquashFS(ctx context.Context) error {
	log.Info(gotext.Get("Step 3/4: Creating SquashFS image"))
	tmpDir, err := os.MkdirTemp(config.TempDir, "capsule-build-")
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to create temp dir"), err)
	}
	b.tempDirs = append(b.tempDirs, tmpDir)

	compressor := squashfs.NewCompressor(b.cfg.Compression)
	squashfsPath, err := compressor.Compress(ctx, b.rootfsPath, tmpDir)
	if err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to create squashfs"), err)
	}
	b.squashfsPath = squashfsPath
	log.Debug("SquashFS created", "path", squashfsPath)
	return nil
}

func (b *buildContext) assemble(ctx context.Context) error {
	log.Info(gotext.Get("Step 4/4: Assembling final binary"))

	if dir := filepath.Dir(b.cfg.Output); dir != "." {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("%s: %w", gotext.Get("failed to create output directory"), err)
		}
	}

	a := assembler.NewAssembler()
	if err := a.Assemble(ctx, b.squashfsPath, b.cfg.Output, b.cfg); err != nil {
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
