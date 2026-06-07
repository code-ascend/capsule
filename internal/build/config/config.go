package config

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/user"
	"path/filepath"
	"strings"
	"time"

	"capsule/internal/format/binconfig"
	"capsule/internal/sys/srcref"

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
	Sandbox     string        `yaml:"sandbox"`
}

// Load reads and parses a YAML config from a local path or http(s):// URL.
func Load(path string) (*Config, error) {
	data, err := ReadSource(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	return LoadFromBytes(data)
}

// LoadFromBytes parses YAML bytes with the same defaults and validation as Load.
func LoadFromBytes(data []byte) (*Config, error) {
	cfg := Config{HostExec: true}
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse yaml: %w", err)
	}
	cfg.setDefaults()
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	return &cfg, nil
}

// ReadSource fetches src from disk or HTTP(S).
func ReadSource(src string) ([]byte, error) {
	if srcref.IsRemote(src) {
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Get(src)
		if err != nil {
			return nil, err
		}
		defer func() { _ = resp.Body.Close() }()
		if resp.StatusCode/100 != 2 {
			return nil, fmt.Errorf("HTTP %d %s", resp.StatusCode, resp.Status)
		}
		ct := resp.Header.Get("Content-Type")
		if strings.HasPrefix(ct, "text/html") {
			return nil, fmt.Errorf("URL returned HTML, not YAML — use the raw URL")
		}
		return io.ReadAll(resp.Body)
	}
	return os.ReadFile(src)
}

// setDefaults applies default values for optional fields
func (c *Config) setDefaults() {
	if c.Output == "" {
		c.Output = "./capsule"
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

var validCompressions = map[string]bool{
	"zstd": true,
	"lz4":  true,
	"gzip": true,
	"xz":   true,
}

// Validate checks that all required fields are set and valid
func (c *Config) Validate() error {
	if c.Image == "" {
		return fmt.Errorf("image is required")
	}

	if !validCompressions[c.Compression] {
		return fmt.Errorf("invalid compression: %s (valid: zstd, lz4, gzip, xz)", c.Compression)
	}

	if c.Sandbox != "" {
		if _, err := binconfig.ParseSandbox(c.Sandbox); err != nil {
			return err
		}
	}

	for i := range c.Install {
		c.Install[i].Run = strings.TrimRight(c.Install[i].Run, "\n")
	}

	for i := range c.Update {
		c.Update[i].Run = strings.TrimRight(c.Update[i].Run, "\n")
	}

	return nil
}
