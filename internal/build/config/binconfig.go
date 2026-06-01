package config

import (
	"strings"

	"capsule/internal/format/binconfig"
)

// BuildMeta is build-time provenance recorded into a capsule's binconfig.
type BuildMeta struct {
	SourceRef string
	SourceSHA string
	BuiltAt   string
}

// ToBinConfig projects the build config and provenance into the embedded runtime config.
func (c *Config) ToBinConfig(meta BuildMeta) *binconfig.Config {
	apps := make([]binconfig.AppExport, len(c.Export.Apps))
	for i, a := range c.Export.Apps {
		apps[i] = binconfig.AppExport{
			Desktop:    a.Desktop,
			Icon:       a.Icon,
			NameSuffix: a.NameSuffix,
		}
	}
	return &binconfig.Config{
		Launch:       c.Launch,
		Compression:  c.Compression,
		UpdateScript: joinUpdateSteps(c.Update),
		Apps:         apps,
		Binaries:     c.Export.Binaries,
		EnvUnset:     c.Env.Unset,
		EnvSet:       c.Env.Set,
		HostExec:     c.HostExec,
		Sandbox:      binconfig.Sandbox(c.Sandbox),
		SourceRef:    meta.SourceRef,
		SourceSHA:    meta.SourceSHA,
		BuiltAt:      meta.BuiltAt,
	}
}

func joinUpdateSteps(steps []InstallStep) string {
	runs := make([]string, 0, len(steps))
	for _, s := range steps {
		if s.Run != "" {
			runs = append(runs, s.Run)
		}
	}
	return strings.Join(runs, "\n")
}
