package squashfs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"capsule/internal/build/config"
	"capsule/internal/format/mksquashfs"
	"capsule/internal/sys/log"
	"capsule/internal/sys/lowprio"
)

// Compress creates a SquashFS image from a directory and writes it under outputDir.
func Compress(ctx context.Context, rootfsPath, outputDir, compression string) (string, error) {
	mksquashfsPath, err := exec.LookPath("mksquashfs")
	if err != nil {
		return "", fmt.Errorf("mksquashfs not found in PATH: %w", err)
	}

	log.Debug("Using mksquashfs", "path", mksquashfsPath)

	outputPath := filepath.Join(outputDir, config.ImageSquashfs)

	_ = os.Remove(outputPath)

	args := mksquashfs.Args(rootfsPath, outputPath, compression)
	if !log.IsDebug() {
		args = append(args, "-quiet")
	}

	log.Debug("Running mksquashfs", "args", args)

	cmd := exec.CommandContext(ctx, mksquashfsPath, args...)

	var stderr bytes.Buffer
	if log.IsDebug() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = &stderr
	}

	if err = lowprio.Run(cmd); err != nil {
		if !log.IsDebug() {
			log.Error("mksquashfs failed", "stderr", stderr.String())
		}
		return "", fmt.Errorf("mksquashfs failed: %w", err)
	}

	info, _ := os.Stat(outputPath)
	if info != nil {
		log.Debug("SquashFS created", "size_mb", float64(info.Size())/(1024*1024))
	}

	return outputPath, nil
}
