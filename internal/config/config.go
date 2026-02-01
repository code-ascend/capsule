package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config represents the build configuration from YAML
type Config struct {
	Image       string `yaml:"image"`
	Output      string `yaml:"output"`
	Compression string `yaml:"compression"`
	CC          string `yaml:"cc"`
	Commands    string `yaml:"commands"`
	Launch      string `yaml:"launch"`
}

// Load reads and parses a YAML config file
func Load(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	var cfg Config
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
	if c.Compression == "" {
		c.Compression = "zstd"
	}
	if c.CC == "" {
		c.CC = "gcc"
	}
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

	c.Commands = strings.TrimRight(c.Commands, "\n")

	return nil
}
