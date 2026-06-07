package export

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func transformDesktop(src, dst, capsulePath, iconOverride, nameSuffix string) (err error) {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("open desktop %s: %w", src, err)
	}
	defer func() { _ = in.Close() }()

	if err = os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("open %s: %w", dst, err)
	}
	defer func() {
		if cerr := out.Close(); cerr != nil && err == nil {
			err = fmt.Errorf("write %s: %w", dst, cerr)
		}
	}()

	scan := bufio.NewScanner(in)
	scan.Buffer(make([]byte, 1024*1024), 1024*1024)

	w := bufio.NewWriter(out)

	for scan.Scan() {
		line := scan.Text()
		switch {
		case strings.HasPrefix(line, "Exec="):
			cmd := strings.TrimPrefix(line, "Exec=")
			first, rest := splitFirstWord(cmd)
			first = strings.Trim(first, `"`)
			if rest == "" {
				_, _ = fmt.Fprintf(w, "Exec=%s %s\n", capsulePath, first)
			} else {
				_, _ = fmt.Fprintf(w, "Exec=%s %s %s\n", capsulePath, first, rest)
			}
		case strings.HasPrefix(line, "TryExec="):
		case line == "DBusActivatable=true":
		case strings.HasPrefix(line, "Icon=") && iconOverride != "":
			_, _ = fmt.Fprintf(w, "Icon=%s\n", iconOverride)
		case (strings.HasPrefix(line, "Name=") || strings.HasPrefix(line, "Name[")) && nameSuffix != "":
			_, _ = fmt.Fprintln(w, line+nameSuffix)
		default:
			_, _ = fmt.Fprintln(w, line)
		}
	}
	if err = scan.Err(); err != nil {
		return err
	}
	if err = w.Flush(); err != nil {
		return fmt.Errorf("write %s: %w", dst, err)
	}
	return nil
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
	defer func() { _ = f.Close() }()

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
