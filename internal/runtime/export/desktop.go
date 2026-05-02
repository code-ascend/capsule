package export

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func transformDesktop(src, dst, capsulePath, iconOverride, nameSuffix string) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open desktop %s: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer out.Close()

	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 1024*1024), 1024*1024)

	w := bufio.NewWriter(out)
	defer w.Flush()

	for scan.Scan() {
		line := scan.Text()
		switch {
		case strings.HasPrefix(line, "Exec="):
			cmd := strings.TrimPrefix(line, "Exec=")
			first, rest := splitFirstWord(cmd)
			first = strings.Trim(first, `"`)
			if rest == "" {
				fmt.Fprintf(w, "Exec=%s %s\n", capsulePath, first)
			} else {
				fmt.Fprintf(w, "Exec=%s %s %s\n", capsulePath, first, rest)
			}
		case strings.HasPrefix(line, "TryExec="):
		case line == "DBusActivatable=true":
		case strings.HasPrefix(line, "Icon=") && iconOverride != "":
			fmt.Fprintf(w, "Icon=%s\n", iconOverride)
		case (strings.HasPrefix(line, "Name=") || strings.HasPrefix(line, "Name[")) && nameSuffix != "":
			fmt.Fprintln(w, line+nameSuffix)
		default:
			fmt.Fprintln(w, line)
		}
	}
	return scan.Err()
}

func splitFirstWord(s string) (string, string) {
	s = strings.TrimSpace(s)
	first, rest, found := strings.Cut(s, " ")
	if !found {
		return s, ""
	}
	return first, strings.TrimSpace(rest)
}

// parseDesktopIcon returns the Icon= value from the [Desktop Entry] section, or "" on any error.
func parseDesktopIcon(path string) string {
	f, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer f.Close()

	scan := bufio.NewScanner(f)
	scan.Buffer(make([]byte, 1024*1024), 1024*1024)

	inEntry := false
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			inEntry = line == "[Desktop Entry]"
			continue
		}
		if inEntry && strings.HasPrefix(line, "Icon=") {
			return strings.TrimSpace(strings.TrimPrefix(line, "Icon="))
		}
	}
	return ""
}
