// capsule
// Copyright (C) 2026 Дмитрий Удалов dmitry@udalov.online
//
// This program is free software: you can redistribute it and/or modify
// it under the terms of the GNU General Public License as published by
// the Free Software Foundation, either version 3 of the License, or
// (at your option) any later version.
//
// This program is distributed in the hope that it will be useful,
// but WITHOUT ANY WARRANTY; without even the implied warranty of
// MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE.  See the
// GNU General Public License for more details.
//
// You should have received a copy of the GNU General Public License
// along with this program.  If not, see <http://www.gnu.org/licenses/>.

package buildcli

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
	"capsule/internal/build/store"
	"capsule/internal/sys/exitcode"
	"capsule/internal/sys/log"
	"capsule/internal/sys/srcref"
	"capsule/internal/sys/userns"

	"github.com/leonelquinteros/gotext"
	"go.podman.io/storage/pkg/unshare"
)

// build runs the build pipeline for a YAML config.
func build(ctx context.Context, configPath, output, compression string) error {
	if configPath == "" {
		return errors.New(gotext.Get("config file required. Usage: capsule build <config.yaml> or capsule build -c <config.yaml>"))
	}
	if err := prepareRootlessEnv(); err != nil {
		return err
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

// rebuild rebuilds an installed capsule from its recorded source.
func rebuild(ctx context.Context, c manager.Capsule) error {
	cfg, rawYAML, err := config.Load(c.Cfg.SourceRef)
	if err != nil {
		return fmt.Errorf("load source: %w", err)
	}
	cfg.Output = c.Path
	return pipeline.Run(ctx, cfg, makeBuildMeta(c.Cfg.SourceRef, rawYAML))
}

// cleanStorage wipes capsule's private build store.
func cleanStorage() error {
	if err := prepareRootlessEnv(); err != nil {
		return err
	}
	if err := store.Clean(); err != nil {
		return fmt.Errorf("%s: %w", gotext.Get("failed to clean build storage"), err)
	}
	exitcode.Notice(gotext.Get("Build storage cleaned"))
	return nil
}

// updateInstalled rebuilds installed capsules.
func updateInstalled(ctx context.Context, names []string, opts manager.UpdateOpts, extraRoots []string) error {
	if err := prepareRootlessEnv(); err != nil {
		return err
	}
	return manager.NewManager(extraRoots...).Update(ctx, names, opts, rebuild)
}

func loadBuildConfig(path string) (*config.Config, []byte, error) {
	cfg, rawYAML, err := config.Load(path)
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

func prepareRootlessEnv() error {
	if err := userns.Preflight(); err != nil {
		return err
	}
	unshare.MaybeReexecUsingUserNamespace(false)
	return nil
}
