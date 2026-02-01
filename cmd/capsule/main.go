package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"capsule/internal/capsule"
	"capsule/internal/config"
	"capsule/internal/log"
	"capsule/internal/oci"
	"capsule/internal/rootfs"
	"capsule/internal/squashfs"

	"github.com/urfave/cli/v3"
)

type buildContext struct {
	cfg          *config.Config
	tempDirs     []string
	imgPath      string
	rootfsPath   string
	squashfsPath string
}

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	rootCmd := &cli.Command{
		Name:  "capsule",
		Usage: "Create portable Linux containers from OCI images",
		Description: `capsule is a tool for creating portable Linux containers as single ELF executables.
It reads a YAML config file specifying the image and commands, then produces a self-contained binary.`,
		Commands: []*cli.Command{
			{
				Name:        "build",
				Usage:       "Build a portable container from an OCI image",
				ArgsUsage:   "[config.yaml]",
				Description: `Build a portable container from an OCI image using a YAML config file.`,
				Action:      runBuild,
				Flags: []cli.Flag{
					&cli.StringFlag{
						Name:    "config",
						Aliases: []string{"c"},
						Usage:   "Path to YAML config file",
					},
					&cli.StringFlag{
						Name:    "output",
						Aliases: []string{"o"},
						Usage:   "Output file path (overrides config)",
					},
					&cli.StringFlag{
						Name:  "compression",
						Usage: "SquashFS compression (overrides config)",
					},
					&cli.StringFlag{
						Name:  "cc",
						Usage: "C compiler for launcher (overrides config)",
					},
					&cli.BoolFlag{
						Name:    "verbose",
						Aliases: []string{"v"},
						Usage:   "Verbose output",
					},
				},
			},
		},
	}

	if err := rootCmd.Run(ctx, os.Args); err != nil {
		if ctx.Err() != nil {
			fmt.Fprintln(os.Stderr, "Interrupted")
			os.Exit(130)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func runBuild(ctx context.Context, cmd *cli.Command) error {
	verbose := cmd.Bool("verbose")
	log.Init(verbose)

	configPath := cmd.Args().First()
	if configPath == "" {
		configPath = cmd.String("config")
	}
	if configPath == "" {
		return fmt.Errorf("config file required. Usage: capsule build <config.yaml> or capsule build -c <config.yaml>")
	}

	if os.Getuid() != 0 {
		return fmt.Errorf("build command requires root privileges. Run with: sudo %s build %s", os.Args[0], configPath)
	}

	cfg, err := config.Load(configPath)
	if err != nil {
		return fmt.Errorf("failed to load config: %w", err)
	}

	if output := cmd.String("output"); output != "" {
		cfg.Output = output
	}
	if compression := cmd.String("compression"); compression != "" {
		cfg.Compression = compression
	}
	if cc := cmd.String("cc"); cc != "" {
		cfg.CC = cc
	}

	log.Debug("Build configuration",
		"image", cfg.Image,
		"output", cfg.Output,
		"compression", cfg.Compression,
		"cc", cfg.CC,
		"has_commands", cfg.Commands != "",
	)

	b := &buildContext{cfg: cfg}
	defer b.cleanup()

	if err = b.pullImage(ctx); err != nil {
		return err
	}
	if err = b.extractRootfs(ctx); err != nil {
		return err
	}
	if err = b.runCommands(ctx); err != nil {
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
	for _, dir := range b.tempDirs {
		if err := os.RemoveAll(dir); err != nil {
			log.Debug("Failed to cleanup temp dir", "path", dir, "error", err)
		}
	}
}

func (b *buildContext) pullImage(ctx context.Context) error {
	log.Info("Step 1/5: Pulling OCI image")
	imgPath, err := oci.Pull(ctx, b.cfg.Image)
	if err != nil {
		return fmt.Errorf("failed to pull image: %w", err)
	}
	b.tempDirs = append(b.tempDirs, filepath.Dir(imgPath))
	b.imgPath = imgPath
	log.Debug("Image pulled", "path", imgPath)
	return nil
}

func (b *buildContext) extractRootfs(ctx context.Context) error {
	log.Info("Step 2/5: Extracting rootfs")
	rootfsPath, err := oci.Extract(ctx, b.imgPath)
	if err != nil {
		return fmt.Errorf("failed to extract rootfs: %w", err)
	}
	b.rootfsPath = rootfsPath
	log.Debug("Rootfs extracted", "path", rootfsPath)
	return nil
}

func (b *buildContext) runCommands(ctx context.Context) error {
	builder, err := rootfs.NewBuilder(b.rootfsPath)
	if err != nil {
		return fmt.Errorf("failed to create builder: %w", err)
	}

	if b.cfg.Commands != "" {
		log.Info("Step 3/5: Running commands")
		if err = builder.RunScript(ctx, b.cfg.Commands); err != nil {
			return fmt.Errorf("failed to run commands: %w", err)
		}
	} else {
		log.Info("Step 3/5: Skipping commands (none specified)")
	}

	if err = builder.PrepareBindTargets(); err != nil {
		log.Debug("Warning: failed to prepare bind targets", "error", err)
	}
	return nil
}

func (b *buildContext) createSquashFS(ctx context.Context) error {
	log.Info("Step 4/5: Creating SquashFS image")
	compressor := squashfs.NewCompressor(b.cfg.Compression)
	squashfsPath, err := compressor.Compress(ctx, b.rootfsPath)
	if err != nil {
		return fmt.Errorf("failed to create squashfs: %w", err)
	}
	b.squashfsPath = squashfsPath
	log.Debug("SquashFS created", "path", squashfsPath)
	return nil
}

func (b *buildContext) assemble(ctx context.Context) error {
	log.Info("Step 5/5: Assembling final binary")
	assembler := capsule.NewAssembler(b.cfg.CC)
	if err := assembler.Assemble(ctx, b.squashfsPath, b.cfg.Output, b.cfg.Launch); err != nil {
		return fmt.Errorf("failed to assemble binary: %w", err)
	}

	info, err := os.Stat(b.cfg.Output)
	if err == nil {
		log.Info("Build complete", "output", b.cfg.Output, "size_mb", fmt.Sprintf("%.2f", float64(info.Size())/(1024*1024)))
	} else {
		log.Info("Build complete", "output", b.cfg.Output)
	}
	return nil
}
