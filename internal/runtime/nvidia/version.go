package nvidia

import (
	"os"
	"strings"
)

const kernelVersionFile = "/sys/module/nvidia/version"

func IsAvailable() bool {
	_, err := os.Stat(kernelVersionFile)
	return err == nil
}

func HostDriverVersion() (string, error) {
	data, err := os.ReadFile(kernelVersionFile)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

func IsCached(markerPath string) bool {
	got, err := os.ReadFile(markerPath)
	if err != nil {
		return false
	}
	want, err := HostDriverVersion()
	if err != nil {
		return false
	}
	return strings.TrimSpace(string(got)) == want
}

func WriteCacheMarker(markerPath string) error {
	v, err := HostDriverVersion()
	if err != nil {
		return err
	}
	return os.WriteFile(markerPath, []byte(v), 0o644)
}
