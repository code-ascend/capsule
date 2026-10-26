package assembler

import (
	"bytes"
	"fmt"
	"io"
	"os"

	"capsule/internal/build/config"
	"capsule/internal/build/embed"
	"capsule/internal/format/binconfig"
	"capsule/internal/format/selfread"
	"capsule/internal/sys/fsutil"
	"capsule/internal/sys/log"
)

// Assemble writes runtime || binconfig JSON || squashfs || footer to outputPath via an atomic ".new" rename.
func Assemble(squashfsPath, outputPath string, cfg *config.Config, meta config.BuildMeta) error {
	runtime, err := embed.GetRuntime()
	if err != nil {
		return err
	}

	binConfigJSON, err := binconfig.Marshal(cfg.ToBinConfig(meta))
	if err != nil {
		return fmt.Errorf("marshal binconfig: %w", err)
	}

	squashfsInfo, err := os.Stat(squashfsPath)
	if err != nil {
		return fmt.Errorf("stat squashfs: %w", err)
	}

	tmpPath := outputPath + ".new"
	prefix := io.MultiReader(bytes.NewReader(runtime), bytes.NewReader(binConfigJSON))
	if err := selfread.WriteCapsule(tmpPath, prefix, int64(len(binConfigJSON)), squashfsPath); err != nil {
		_ = os.Remove(tmpPath)
		return err
	}

	origUID, origGID, hadOwner := fsutil.Owner(outputPath)
	if err := os.Rename(tmpPath, outputPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("atomic replace: %w", err)
	}
	if hadOwner {
		if err := os.Chown(outputPath, origUID, origGID); err != nil {
			log.Debug("preserve owner failed", "error", err)
		}
	}

	if info, errStat := os.Stat(outputPath); errStat == nil {
		log.Debug("capsule assembled",
			"runtime_size", len(runtime),
			"binconfig_size", len(binConfigJSON),
			"squashfs_size", squashfsInfo.Size(),
			"total", info.Size(),
		)
	}
	return nil
}
