package nvidia

import (
	"bufio"
	"bytes"
	"os/exec"
	"strings"

	"capsule/internal/runtime/fsutil"
)

// LdEntry is one parsed line of `ldconfig -p`:
//
//	libfoo.so.1 (libc6,x86-64) => /usr/lib/.../libfoo.so.1
type LdEntry struct {
	Soname string
	Tag    string
	Path   string
}

// Is64Bit defaults to true on missing tag (matches bash fallback).
func (e LdEntry) Is64Bit() bool {
	if e.Tag == "" {
		return true
	}
	return strings.Contains(e.Tag, "x86-64") || strings.Contains(e.Tag, "x86_64") || strings.Contains(e.Tag, "ELF-64")
}

func ParseLdConfig(data []byte) []LdEntry {
	var out []LdEntry
	scan := bufio.NewScanner(bytes.NewReader(data))
	scan.Buffer(make([]byte, 1024*1024), 1024*1024)
	first := true
	for scan.Scan() {
		line := strings.TrimSpace(scan.Text())
		if first {
			first = false
			continue
		}
		if line == "" {
			continue
		}
		if entry, ok := parseLine(line); ok {
			out = append(out, entry)
		}
	}
	return out
}

func parseLine(line string) (LdEntry, bool) {
	leftRaw, pathRaw, ok := strings.Cut(line, "=>")
	if !ok {
		return LdEntry{}, false
	}
	left := strings.TrimSpace(leftRaw)
	path := strings.TrimSpace(pathRaw)
	if path == "" {
		return LdEntry{}, false
	}
	soname := left
	tag := ""
	if open := strings.Index(left, "("); open != -1 {
		if close := strings.Index(left[open:], ")"); close != -1 {
			soname = strings.TrimSpace(left[:open])
			tag = strings.TrimSpace(left[open+1 : open+close])
		}
	}
	return LdEntry{Soname: soname, Tag: tag, Path: path}, true
}

func RunLdConfig() ([]LdEntry, error) {
	out, err := exec.Command(findLdconfig(), "-p").Output()
	if err != nil {
		return nil, err
	}
	return ParseLdConfig(out), nil
}

func findLdconfig() string {
	for _, p := range []string{"/sbin/ldconfig", "/usr/sbin/ldconfig", "/usr/bin/ldconfig"} {
		if fsutil.IsExecutable(p) {
			return p
		}
	}
	return "ldconfig"
}
