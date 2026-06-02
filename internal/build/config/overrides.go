package config

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

const OverridesFile = ".capsule.overrides.yml"

// ApplyOverrides reads OverridesFile from rootfsPath and merges its YAML on top of c.
func (c *Config) ApplyOverrides(rootfsPath string) error {
	path := filepath.Join(rootfsPath, OverridesFile)
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read %s: %w", OverridesFile, err)
	}
	if err = yaml.Unmarshal(data, c); err != nil {
		return fmt.Errorf("parse %s: %w", OverridesFile, err)
	}
	c.setDefaults()
	return c.Validate()
}

// RemoveOverrides deletes OverridesFile from rootfsPath.
func RemoveOverrides(rootfsPath string) error {
	err := os.Remove(filepath.Join(rootfsPath, OverridesFile))
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	return nil
}
