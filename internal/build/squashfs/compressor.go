package squashfs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"capsule/internal/build/config"
	"capsule/internal/sys/log"
)

// Compressor handles SquashFS image creation
type Compressor struct {
	compression string
}

// NewCompressor creates a new Compressor instance
func NewCompressor(compression string) *Compressor {
	return &Compressor{compression: compression}
}

// Compress creates a SquashFS image from a directory and writes it under outputDir.
func (c *Compressor) Compress(ctx context.Context, rootfsPath, outputDir string) (string, error) {
	mksquashfsPath, err := exec.LookPath("mksquashfs")
	if err != nil {
		return "", fmt.Errorf("mksquashfs not found in PATH: %w", err)
	}

	log.Debug("Using mksquashfs", "path", mksquashfsPath)

	outputPath := filepath.Join(outputDir, config.ImageSquashfs)

	os.Remove(outputPath)

	args := []string{
		rootfsPath,
		outputPath,
		"-comp", c.compression,
		"-noappend",
		"-no-xattrs",
	}

	switch c.compression {
	case "zstd":
		// Larger block size for better sequential read, max compression
		args = append(args, "-b", "1M", "-Xcompression-level", "19")
	case "xz":
		args = append(args, "-b", "1M", "-Xbcj", "x86")
	case "lz4":
		// Smaller block for random access, high compression mode
		args = append(args, "-b", "256K", "-Xhc")
	case "gzip":
		args = append(args, "-b", "1M")
	}

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

	if err = cmd.Run(); err != nil {
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
