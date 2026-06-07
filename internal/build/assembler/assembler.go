package assembler

import (
	"context"
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

type Assembler struct{}

func NewAssembler() *Assembler { return &Assembler{} }

// Assemble writes runtime || binconfig JSON || squashfs || footer to outputPath via an atomic ".new" rename.
func (a *Assembler) Assemble(_ context.Context, squashfsPath, outputPath string, cfg *config.Config, meta config.BuildMeta) error {
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
	if err := writeCapsule(tmpPath, runtime, binConfigJSON, squashfsPath); err != nil {
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

func writeCapsule(path string, runtime, binConfigJSON []byte, squashfsPath string) (err error) {
	out, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("close output: %w", cerr)
		}
	}()

	if _, err = out.Write(runtime); err != nil {
		return fmt.Errorf("write runtime: %w", err)
	}
	if _, err = out.Write(binConfigJSON); err != nil {
		return fmt.Errorf("write binconfig: %w", err)
	}
	squashfsSize, err := appendFile(out, squashfsPath)
	if err != nil {
		return err
	}
	return selfread.EncodeFooter(out, int64(len(binConfigJSON)), squashfsSize)
}

// appendFile copies path into w and returns the number of bytes written.
func appendFile(w io.Writer, path string) (int64, error) {
	f, err := os.Open(path)
	if err != nil {
		return 0, fmt.Errorf("open %s: %w", path, err)
	}
	defer func() { _ = f.Close() }()
	n, err := io.Copy(w, f)
	if err != nil {
		return 0, fmt.Errorf("copy %s: %w", path, err)
	}
	return n, nil
}
