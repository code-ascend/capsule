package capsule

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"capsule/internal/config"
	"capsule/internal/embed"
	"capsule/internal/log"
)

// Compiler handles C code compilation
type Compiler struct {
	cc string
}

// NewCompiler creates a new Compiler instance
func NewCompiler(cc string) *Compiler {
	return &Compiler{cc: cc}
}

// Compile compiles the launcher with specific component sizes
func (c *Compiler) Compile(ctx context.Context, binCfg *BinaryConfig, outputPath string) error {
	tmpDir, err := os.MkdirTemp("", config.TempPrefixCompile)
	if err != nil {
		return fmt.Errorf("failed to create temp directory: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	const sourceFile = "init.c"
	sourcePath := filepath.Join(tmpDir, sourceFile)

	gen := NewTemplateGenerator(sourceFile, embed.InitCTemplate)
	sourceCode, err := gen.Generate(binCfg)
	if err != nil {
		return fmt.Errorf("failed to generate launcher source: %w", err)
	}

	if err = os.WriteFile(sourcePath, sourceCode, 0644); err != nil {
		return fmt.Errorf("failed to write launcher source: %w", err)
	}

	compilerPath, err := exec.LookPath(c.cc)
	if err != nil {
		compilerPath, err = exec.LookPath("gcc")
		if err != nil {
			return fmt.Errorf("no C compiler found (tried %s and gcc)", c.cc)
		}
	}

	args := []string{
		"-static",
		"-Os",
		"-s",
		"-o", outputPath,
		sourcePath,
	}

	log.Debug("Compiling launcher", "compiler", compilerPath, "args", args)

	cmd := exec.CommandContext(ctx, compilerPath, args...)

	var stderr bytes.Buffer
	if log.IsDebug() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stderr = &stderr
	}

	if err = cmd.Run(); err != nil {
		if !log.IsDebug() {
			log.Error("Compiler failed", "stderr", stderr.String())
		}
		return fmt.Errorf("compilation failed: %w", err)
	}

	return nil
}
