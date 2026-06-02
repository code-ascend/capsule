package userns

import (
	"errors"
	"os"
	"os/user"
	"strings"

	"github.com/leonelquinteros/gotext"
)

// Preflight returns an actionable error when the current user has no subuid/subgid range for rootless builds.
func Preflight() error {
	if os.Geteuid() == 0 {
		return nil
	}
	u, err := user.Current()
	if err != nil {
		return nil
	}
	if subIDConfigured("/etc/subuid", u) && subIDConfigured("/etc/subgid", u) {
		return nil
	}
	return errors.New(notConfiguredMessage(u.Username))
}

// notConfiguredMessage explains the missing rootless setup with distro-specific fix steps.
func notConfiguredMessage(username string) string {
	var b strings.Builder
	b.WriteString(gotext.Get("rootless build needs user-namespace mapping, which is not configured for user %q.", username))
	b.WriteString("\n" + gotext.Get("Fix:"))
	if isALT() {
		b.WriteString("\n  control newuidmap public")
		b.WriteString("\n  control newgidmap public")
	} else {
		b.WriteString("\n  " + gotext.Get("ensure newuidmap/newgidmap are installed (shadow-utils / uidmap)"))
	}
	b.WriteString("\n  usermod --add-subuids 100000-165535 --add-subgids 100000-165535 " + username)
	b.WriteString("\n" + gotext.Get("Then run the build again."))
	return b.String()
}

func subIDConfigured(path string, u *user.User) bool {
	data, err := os.ReadFile(path)
	if err != nil {
		return false
	}
	return hasSubIDEntry(data, u.Username, u.Uid)
}

func hasSubIDEntry(data []byte, username, uid string) bool {
	for line := range strings.SplitSeq(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		owner, _, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		if owner == username || owner == uid {
			return true
		}
	}
	return false
}

func isALT() bool {
	data, err := os.ReadFile("/etc/os-release")
	if err != nil {
		return false
	}
	return isALTRelease(data)
}

func isALTRelease(data []byte) bool {
	return osReleaseField(data, "ID") == "altlinux" ||
		strings.Contains(osReleaseField(data, "ID_LIKE"), "altlinux")
}

func osReleaseField(data []byte, key string) string {
	prefix := key + "="
	for line := range strings.SplitSeq(string(data), "\n") {
		if v, ok := strings.CutPrefix(strings.TrimSpace(line), prefix); ok {
			return strings.Trim(v, `"'`)
		}
	}
	return ""
}
