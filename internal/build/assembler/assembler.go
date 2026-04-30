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
	"capsule/internal/sys/log"
)

type Assembler struct{}

func NewAssembler() *Assembler { return &Assembler{} }

// Assemble writes runtime || binconfig JSON || squashfs || footer into outputPath.
func (a *Assembler) Assemble(_ context.Context, squashfsPath, outputPath string, cfg *config.Config) error {
	runtime, err := embed.GetRuntime()
	if err != nil {
		return err
	}

	binconfigJSON, err := binconfig.Marshal(buildBinconfig(cfg))
	if err != nil {
		return fmt.Errorf("marshal binconfig: %w", err)
	}

	squashfsInfo, err := os.Stat(squashfsPath)
	if err != nil {
		return fmt.Errorf("stat squashfs: %w", err)
	}

	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("open output: %w", err)
	}
	defer out.Close()

	if _, err = out.Write(runtime); err != nil {
		return fmt.Errorf("write runtime: %w", err)
	}
	if _, err = out.Write(binconfigJSON); err != nil {
		return fmt.Errorf("write binconfig: %w", err)
	}
	if err = appendFile(out, squashfsPath); err != nil {
		return err
	}
	if err = selfread.EncodeFooter(out, int64(len(binconfigJSON)), squashfsInfo.Size()); err != nil {
		return err
	}

	if info, errStat := out.Stat(); errStat == nil {
		log.Debug("capsule assembled",
			"runtime_size", len(runtime),
			"binconfig_size", len(binconfigJSON),
			"squashfs_size", squashfsInfo.Size(),
			"total", info.Size(),
		)
	}
	return nil
}

func buildBinconfig(cfg *config.Config) *binconfig.Config {
	apps := make([]binconfig.AppExport, len(cfg.Export.Apps))
	for i, a := range cfg.Export.Apps {
		apps[i] = binconfig.AppExport{
			Desktop:    a.Desktop,
			Icon:       a.Icon,
			NameSuffix: a.NameSuffix,
		}
	}
	return &binconfig.Config{
		Launch:       cfg.Launch,
		Compression:  cfg.Compression,
		UpdateScript: joinUpdateSteps(cfg.Update),
		Apps:         apps,
		Binaries:     cfg.Export.Binaries,
		EnvUnset:     cfg.Env.Unset,
		EnvSet:       cfg.Env.Set,
	}
}

func joinUpdateSteps(steps []config.InstallStep) string {
	if len(steps) == 0 {
		return ""
	}
	out := ""
	for i, s := range steps {
		if s.Run == "" {
			continue
		}
		if i > 0 && out != "" {
			out += "\n"
		}
		out += s.Run
	}
	return out
}

func appendFile(w io.Writer, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("open %s: %w", path, err)
	}
	defer f.Close()
	if _, err = io.Copy(w, f); err != nil {
		return fmt.Errorf("copy %s: %w", path, err)
	}
	return nil
}
