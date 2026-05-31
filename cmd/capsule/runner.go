package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"capsule/internal/build/config"
	"capsule/internal/build/manager"
	"capsule/internal/build/pipeline"
	"capsule/internal/sys/log"
	"capsule/internal/sys/srcref"

	"github.com/leonelquinteros/gotext"
	"github.com/urfave/cli/v3"
)

// Runner groups build CLI commands as methods.
type Runner struct{}

func NewRunner() *Runner {
	return &Runner{}
}

// wrap adapts a Runner-shaped action to cli.ActionFunc.
func (r *Runner) wrap(fn func(context.Context, *cli.Command, *Runner) error) cli.ActionFunc {
	return func(ctx context.Context, cmd *cli.Command) error {
		return fn(ctx, cmd, r)
	}
}

// Build runs the build pipeline for a YAML config.
func (r *Runner) Build(ctx context.Context, configPath, output, compression string) error {
	if configPath == "" {
		return errors.New(gotext.Get("config file required. Usage: capsule build <config.yaml> or capsule build -c <config.yaml>"))
	}

	cfg, rawYAML, err := loadBuildConfig(configPath)
	if err != nil {
		return err
	}
	if output != "" {
		cfg.Output = output
	}
	if compression != "" {
		cfg.Compression = compression
	}

	log.Debug("Build configuration",
		"image", cfg.Image,
		"output", cfg.Output,
		"compression", cfg.Compression,
		"install_steps", len(cfg.Install),
	)

	meta := makeBuildMeta(srcref.Normalize(configPath), rawYAML)
	return pipeline.Run(ctx, cfg, meta)
}

// Rebuild rebuilds an installed capsule from its recorded source.
func (r *Runner) Rebuild(ctx context.Context, c manager.Capsule) error {
	rawYAML, err := config.ReadSource(c.Cfg.SourceRef)
	if err != nil {
		return fmt.Errorf("fetch source: %w", err)
	}
	cfg, err := config.LoadFromBytes(rawYAML)
	if err != nil {
		return fmt.Errorf("parse source: %w", err)
	}
	cfg.Output = c.Path
	return pipeline.Run(ctx, cfg, makeBuildMeta(c.Cfg.SourceRef, rawYAML))
}

// List prints installed capsules.
func (r *Runner) List(extraRoots []string) error {
	return manager.NewManager(extraRoots...).List()
}

// UpdateInstalled rebuilds installed capsules.
func (r *Runner) UpdateInstalled(ctx context.Context, names []string, opts manager.UpdateOpts, extraRoots []string) error {
	return manager.NewManager(extraRoots...).Update(ctx, names, opts, r.Rebuild)
}

func loadBuildConfig(path string) (*config.Config, []byte, error) {
	rawYAML, err := config.ReadSource(path)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", gotext.Get("failed to read config"), err)
	}
	cfg, err := config.LoadFromBytes(rawYAML)
	if err != nil {
		return nil, nil, fmt.Errorf("%s: %w", gotext.Get("failed to load config"), err)
	}
	return cfg, rawYAML, nil
}

// makeBuildMeta builds binconfig provenance metadata.
func makeBuildMeta(ref string, raw []byte) config.BuildMeta {
	sum := sha256.Sum256(raw)
	return config.BuildMeta{
		SourceRef: ref,
		SourceSHA: hex.EncodeToString(sum[:]),
		BuiltAt:   time.Now().UTC().Format(time.RFC3339),
	}
}
