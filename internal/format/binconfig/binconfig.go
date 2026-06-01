package binconfig

import (
	"encoding/json"
	"fmt"
)

// Sandbox selects how much of the host the capsule shares with its runtime.
type Sandbox string

const (
	// SandboxShared binds host mounts (/run, /mnt, /media) as-is and shares the network and PID namespaces (max compatibility).
	SandboxShared Sandbox = "shared"
	// SandboxIsolated gives a private writable /run with only user/dbus sockets, hides host /mnt and /media behind tmpfs, and unshares PIDs.
	SandboxIsolated Sandbox = "isolated"
	// SandboxStrict is SandboxIsolated plus an unshared (offline) network namespace.
	SandboxStrict Sandbox = "strict"

	// DefaultSandbox applies when neither the flag nor the embedded config selects a mode.
	DefaultSandbox = SandboxShared
)

// Valid reports whether s is a known sandbox mode.
func (s Sandbox) Valid() bool {
	switch s {
	case SandboxShared, SandboxIsolated, SandboxStrict:
		return true
	}
	return false
}

// ParseSandbox validates v and returns the matching mode.
func ParseSandbox(v string) (Sandbox, error) {
	s := Sandbox(v)
	if !s.Valid() {
		return "", fmt.Errorf("invalid sandbox mode %q (valid: shared, isolated, strict)", v)
	}
	return s, nil
}

// HostExecSocketEnv carries the abstract UNIX socket name to the in-capsule client.
const HostExecSocketEnv = "CAPSULE_HOST_SOCKET"

// InsideEnv marks that the current process runs inside a capsule sandbox.
const InsideEnv = "CAPSULE_INSIDE"

// HostExecCommand is the canonical in-capsule client name.
const HostExecCommand = "capsule-host-exec"

// HostExecForwardedAliases are commands proxied to the host; PTY is forced off for them (glib bug #2695).
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
	Sandbox      Sandbox           `json:"sandbox,omitempty"`

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
