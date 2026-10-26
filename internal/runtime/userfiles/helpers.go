package userfiles

import (
	"fmt"
	"os"
	"slices"
	"strconv"
	"strings"
)

func readLines(path string) ([]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text := strings.TrimRight(string(data), "\n")
	if text == "" {
		return nil, nil
	}
	return strings.Split(text, "\n"), nil
}

func writeLines(path string, lines []string, mode os.FileMode) error {
	tmp := path + ".tmp"
	body := strings.Join(lines, "\n") + "\n"
	if err := os.WriteFile(tmp, []byte(body), mode); err != nil {
		return fmt.Errorf("write %s: %w", tmp, err)
	}
	if err := os.Chmod(tmp, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		return fmt.Errorf("rename %s: %w", path, err)
	}
	return nil
}

// dropEntries removes lines named name or, when id is set, carrying id in the third field.
func dropEntries(lines []string, name, id string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		fields := strings.SplitN(l, ":", 4)
		if fields[0] == name || (id != "" && len(fields) >= 3 && fields[2] == id) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// appendUserToGroup adds the user to an existing container group, or is a no-op if the group is missing.
func appendUserToGroup(lines []string, group, user string, hostGID int) []string {
	prefix := group + ":"
	for i, l := range lines {
		if !strings.HasPrefix(l, prefix) {
			continue
		}
		fields := strings.SplitN(l, ":", 4)
		if len(fields) < 4 {
			return lines
		}
		if fields[2] == strconv.Itoa(hostGID) {
			return lines
		}
		members := fields[3]
		if members != "" && slices.Contains(strings.Split(members, ","), user) {
			return lines
		}
		if members == "" {
			members = user
		} else {
			members = members + "," + user
		}
		lines[i] = fields[0] + ":" + fields[1] + ":" + fields[2] + ":" + members
		return lines
	}
	return lines
}
