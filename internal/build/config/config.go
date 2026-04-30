package config

import (
	"fmt"
	"os"
	"os/user"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

// AppExport represents an app to export (.desktop + icon)
type AppExport struct {
	Desktop    string `yaml:"desktop"`
	Icon       string `yaml:"icon"`
	NameSuffix string `yaml:"name_suffix"`
}

// Export represents export configuration for apps and binaries
type Export struct {
	Apps     []AppExport `yaml:"apps"`
	Binaries []string    `yaml:"binaries"`
}

// Env represents environment variable configuration
type Env struct {
	Unset []string          `yaml:"unset"`
	Set   map[string]string `yaml:"set"`
}

// InstallStep represents a build step with name and commands
type InstallStep struct {
	Name string `yaml:"name"`
	Run  string `yaml:"run"`
}

// Config represents the build configuration from YAML
type Config struct {
	Image       string        `yaml:"image"`
	Output      string        `yaml:"output"`
	Compression string        `yaml:"compression"`
	Install     []InstallStep `yaml:"install"`
	Update      []InstallStep `yaml:"update"`
	Launch      string        `yaml:"launch"`
	Export      Export        `yaml:"export"`
	Env         Env           `yaml:"env"`
	HostExec    bool          `yaml:"host_exec"`
}

// Load reads and parses a YAML config file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	cfg := Config{HostExec: true}
	if err = yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg.setDefaults()

	if err = cfg.Validate(); err != nil {
		return nil, err
	}

	return &cfg, nil
}

// setDefaults applies default values for optional fields
func (c *Config) setDefaults() {
	if c.Output == "" {
		c.Output = "./container"
	}
	c.Output = expandTilde(c.Output)
	if c.Compression == "" {
		c.Compression = "zstd"
	}
}

// expandTilde expands ~ to user's home directory
func expandTilde(path string) string {
	if !strings.HasPrefix(path, "~/") {
		return path
	}
	if sudoUser := os.Getenv("SUDO_USER"); sudoUser != "" {
		if u, err := user.Lookup(sudoUser); err == nil {
			return filepath.Join(u.HomeDir, path[2:])
		}
	}
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, path[2:])
	}
	return path
}

// Validate checks that all required fields are set and valid
func (c *Config) Validate() error {
	if c.Image == "" {
		return fmt.Errorf("image is required")
	}

	validCompressions := map[string]bool{
		"zstd": true,
		"lz4":  true,
		"gzip": true,
		"xz":   true,
	}
	if !validCompressions[c.Compression] {
		return fmt.Errorf("invalid compression: %s (valid: zstd, lz4, gzip, xz)", c.Compression)
	}

	for i := range c.Install {
		c.Install[i].Run = strings.TrimRight(c.Install[i].Run, "\n")
	}

	for i := range c.Update {
		c.Update[i].Run = strings.TrimRight(c.Update[i].Run, "\n")
	}

	return nil
}
