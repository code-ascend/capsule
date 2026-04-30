package binconfig

import "encoding/json"

type AppExport struct {
	Desktop    string `json:"desktop"`
	Icon       string `json:"icon,omitempty"`
	NameSuffix string `json:"name_suffix,omitempty"`
}

type Config struct {
	Launch       string            `json:"launch,omitempty"`
	Compression  string            `json:"compression"`
	UpdateScript string            `json:"update_script,omitempty"`
	Apps         []AppExport       `json:"apps,omitempty"`
	Binaries     []string          `json:"binaries,omitempty"`
	EnvUnset     []string          `json:"env_unset,omitempty"`
	EnvSet       map[string]string `json:"env_set,omitempty"`
}

func Marshal(c *Config) ([]byte, error) { return json.Marshal(c) }

func Unmarshal(data []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
