package binconfig

import "encoding/json"

// HostExecSocketEnv carries the abstract UNIX socket name from the runtime
// to the in-capsule capsule-host-exec client.
const HostExecSocketEnv = "CAPSULE_HOST_SOCKET"

// InsideEnv marks that the current process runs inside a capsule sandbox.
const InsideEnv = "CAPSULE_INSIDE"

// HostExecCommand is the canonical in-capsule client name.
const HostExecCommand = "capsule-host-exec"

// HostExecForwardedAliases are the commands the runtime ELF is bound under to
// transparently proxy to the host. PTY is forced off for them (glib bug #2695).
var HostExecForwardedAliases = []string{"xdg-open", "gio", "flatpak"}

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
	HostExec     bool              `json:"host_exec,omitempty"`

	SourceRef string `json:"source_ref,omitempty"`
	SourceSHA string `json:"source_sha,omitempty"`
	BuiltAt   string `json:"built_at,omitempty"`
}

func Marshal(c *Config) ([]byte, error) { return json.Marshal(c) }

func Unmarshal(data []byte) (*Config, error) {
	var c Config
	if err := json.Unmarshal(data, &c); err != nil {
		return nil, err
	}
	return &c, nil
}
