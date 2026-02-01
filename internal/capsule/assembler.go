package capsule

import (
	"context"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"capsule/internal/config"
	"capsule/internal/embed"
	"capsule/internal/log"
)

// Assembler handles final ELF binary assembly.
type Assembler struct {
	cc string
}

// NewAssembler creates a new Assembler instance.
func NewAssembler(cc string) *Assembler {
	return &Assembler{cc: cc}
}

// binaryComponents holds all data needed for the final binary.
type binaryComponents struct {
	launcher []byte
	bash     []byte
	runtime  []byte
	utils    []byte
}

// Assemble creates the final ELF binary from components.
func (a *Assembler) Assemble(ctx context.Context, squashfsPath, outputPath, launch string) error {
	components, binCfg, err := a.loadComponents(launch)
	if err != nil {
		return err
	}

	tmpDir, err := os.MkdirTemp("", config.TempPrefixAssemble)
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	components.runtime, err = a.generateRuntime(binCfg)
	if err != nil {
		return err
	}

	components.launcher, err = a.compileLauncher(ctx, tmpDir, binCfg)
	if err != nil {
		return err
	}

	return a.writeBinary(outputPath, squashfsPath, components)
}

// loadComponents loads embedded bash and utils, creates BinaryConfig.
func (a *Assembler) loadComponents(launch string) (*binaryComponents, *BinaryConfig, error) {
	bashData, err := embed.GetBash()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get embedded bash: %w", err)
	}

	utilsData, err := embed.GetUtils()
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get embedded utils: %w", err)
	}

	binCfg := &BinaryConfig{
		InitSize:   config.InitPaddedSize,
		BashSize:   int64(len(bashData)),
		ScriptSize: config.ScriptPaddedSize,
		UtilsSize:  int64(len(utilsData)),
		Launch:     launch,
	}

	log.Debug("Component sizes",
		"init_padded", binCfg.InitSize,
		"bash", binCfg.BashSize,
		"script_padded", binCfg.ScriptSize,
		"utils", binCfg.UtilsSize,
	)

	return &binaryComponents{bash: bashData, utils: utilsData}, binCfg, nil
}

// generateRuntime generates the runtime.sh script.
func (a *Assembler) generateRuntime(binCfg *BinaryConfig) ([]byte, error) {
	gen := NewTemplateGenerator("runtime.sh", embed.RuntimeShTemplate)
	data, err := gen.Generate(binCfg)
	if err != nil {
		return nil, fmt.Errorf("failed to generate runtime script: %w", err)
	}

	if int64(len(data)) > config.ScriptPaddedSize {
		return nil, fmt.Errorf("runtime script too large: %d > %d", len(data), config.ScriptPaddedSize)
	}

	return data, nil
}

// compileLauncher compiles the C launcher binary.
func (a *Assembler) compileLauncher(ctx context.Context, tmpDir string, binCfg *BinaryConfig) ([]byte, error) {
	launcherPath := filepath.Join(tmpDir, "init")

	compiler := NewCompiler(a.cc)
	if err := compiler.Compile(ctx, binCfg, launcherPath); err != nil {
		return nil, fmt.Errorf("failed to compile launcher: %w", err)
	}

	data, err := os.ReadFile(launcherPath)
	if err != nil {
		return nil, fmt.Errorf("failed to read launcher: %w", err)
	}

	if int64(len(data)) > config.InitPaddedSize {
		return nil, fmt.Errorf("launcher too large: %d > %d", len(data), config.InitPaddedSize)
	}

	return data, nil
}

// writeBinary writes all components to the final binary file.
func (a *Assembler) writeBinary(outputPath, squashfsPath string, c *binaryComponents) error {
	out, err := os.OpenFile(outputPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0755)
	if err != nil {
		return fmt.Errorf("failed to create output file: %w", err)
	}
	defer out.Close()

	if err = writeWithPadding(out, c.launcher, config.InitPaddedSize, "init"); err != nil {
		return err
	}
	if err = writeData(out, c.bash, "bash"); err != nil {
		return err
	}
	if err = writeWithPadding(out, c.runtime, config.ScriptPaddedSize, "runtime"); err != nil {
		return err
	}
	if err = writeData(out, c.utils, "utils"); err != nil {
		return err
	}
	if err = writeFile(out, squashfsPath, "squashfs"); err != nil {
		return err
	}

	if info, errStat := out.Stat(); errStat == nil {
		log.Debug("Final binary assembled",
			"size", info.Size(),
			"size_mb", fmt.Sprintf("%.2f", float64(info.Size())/(1024*1024)),
		)
	}

	return nil
}

// writeWithPadding writes data with padding to reach the target size.
func writeWithPadding(w io.Writer, data []byte, paddedSize int64, name string) error {
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write %s: %w", name, err)
	}
	padding := make([]byte, paddedSize-int64(len(data)))
	if _, err := w.Write(padding); err != nil {
		return fmt.Errorf("failed to write %s padding: %w", name, err)
	}
	return nil
}

// writeData writes data without padding.
func writeData(w io.Writer, data []byte, name string) error {
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("failed to write %s: %w", name, err)
	}
	return nil
}

// writeFile copies a file to the writer.
func writeFile(w io.Writer, path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("failed to open %s: %w", name, err)
	}
	defer f.Close()

	if _, err = io.Copy(w, f); err != nil {
		return fmt.Errorf("failed to write %s: %w", name, err)
	}
	return nil
}
