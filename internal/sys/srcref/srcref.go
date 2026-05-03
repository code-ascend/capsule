package srcref

import (
	"path/filepath"
	"strings"
)

// IsRemote reports whether ref is an HTTP(S) URL.
func IsRemote(ref string) bool {
	return strings.HasPrefix(ref, "http://") || strings.HasPrefix(ref, "https://")
}

// Normalize returns remote refs as-is, local paths as absolute.
func Normalize(ref string) string {
	if IsRemote(ref) {
		return ref
	}
	if abs, err := filepath.Abs(ref); err == nil {
		return abs
	}
	return ref
}
