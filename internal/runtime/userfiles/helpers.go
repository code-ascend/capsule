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

func filterByField3NotEqual(lines []string, num int) []string {
	target := strconv.Itoa(num)
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		fields := strings.SplitN(l, ":", 4)
		if len(fields) >= 3 && fields[2] == target {
			continue
		}
		out = append(out, l)
	}
	return out
}

func dropPrefix(lines []string, prefix string) []string {
	out := make([]string, 0, len(lines))
	for _, l := range lines {
		if strings.HasPrefix(l, prefix) {
			continue
		}
		out = append(out, l)
	}
	return out
}

// appendUserToGroup is a no-op when the group is missing; we only add a member
// when the container already declares that group.
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
