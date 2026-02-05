package rootfs

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"capsule/internal/log"
)

// Builder handles rootfs modification via bwrap
type Builder struct {
	rootfsPath string
	bwrapPath  string
}

// NewBuilder creates a new Builder instance
func NewBuilder(rootfsPath string) (*Builder, error) {
	bwrapPath, err := exec.LookPath("bwrap")
	if err != nil {
		return nil, fmt.Errorf("bwrap not found in PATH: %w", err)
	}

	return &Builder{
		rootfsPath: rootfsPath,
		bwrapPath:  bwrapPath,
	}, nil
}

// RunScript executes a shell script inside the rootfs using bwrap
func (b *Builder) RunScript(ctx context.Context, script string) error {
	if script == "" {
		return nil
	}

	log.Debug("Running script in container", "script_length", len(script))

	wrappedScript := "set -e\n" + script
	return b.runInContainer(ctx, "/bin/sh", "-c", wrappedScript)
}

// PrepareBindTargets creates placeholder files and directories
func (b *Builder) PrepareBindTargets() error {
	mediaDir := filepath.Join(b.rootfsPath, "media")
	if err := os.MkdirAll(mediaDir, 0755); err != nil {
		log.Debug("Failed to create /media", "error", err)
	}

	etcDir := filepath.Join(b.rootfsPath, "etc")
	if err := os.MkdirAll(etcDir, 0755); err != nil {
		return err
	}

	placeholderFiles := []struct {
		path    string
		content string
		perm    os.FileMode
	}{
		{"machine-id", "", 0444},
		{"localtime", "", 0644},
		{"hosts", "127.0.0.1 localhost\n", 0644},
		{"nsswitch.conf", "hosts: files dns\n", 0644},
	}

	for _, f := range placeholderFiles {
		fPath := filepath.Join(etcDir, f.path)
		if _, err := os.Stat(fPath); os.IsNotExist(err) {
			if err = os.WriteFile(fPath, []byte(f.content), f.perm); err != nil {
				log.Debug("Failed to create placeholder", "file", f.path, "error", err)
			} else {
				log.Debug("Created placeholder", "file", f.path)
			}
		}
	}

	return nil
}

// runInContainer runs a command inside the rootfs using bwrap
func (b *Builder) runInContainer(ctx context.Context, command string, args ...string) error {
	bwrapArgs := []string{
		"--bind", b.rootfsPath, "/",
		"--dev", "/dev",
		"--proc", "/proc",
		"--ro-bind", "/sys", "/sys",
		"--bind", "/tmp", "/tmp",
		"--tmpfs", "/run",
		"--dir", "/run/lock",
		"--ro-bind", "/etc/resolv.conf", "/etc/resolv.conf",
		"--setenv", "PATH", "/usr/local/sbin:/usr/local/bin:/usr/sbin:/usr/bin:/sbin:/bin",
		"--setenv", "HOME", "/root",
		"--setenv", "TMPDIR", "/tmp",
		"--setenv", "LC_ALL", "C",
		"--",
		command,
	}
	bwrapArgs = append(bwrapArgs, args...)

	cmd := exec.CommandContext(ctx, b.bwrapPath, bwrapArgs...)

	var stdout, stderr bytes.Buffer
	if log.IsDebug() {
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
	} else {
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
	}

	if err := cmd.Run(); err != nil {
		if !log.IsDebug() {
			log.Error("Command failed",
				"command", command,
				"args", args,
				"stdout", stdout.String(),
				"stderr", stderr.String(),
			)
		}
		return fmt.Errorf("command failed: %s %v: %w", command, args, err)
	}

	return nil
}
