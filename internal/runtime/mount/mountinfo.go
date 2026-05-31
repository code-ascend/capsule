package mount

import (
	"bytes"
	"strings"
)

type mountInfoScanner struct {
	data []byte
	pos  int
	line []byte
}

func mountInfoScan(data []byte) *mountInfoScanner {
	return &mountInfoScanner{data: data}
}

func (s *mountInfoScanner) next() bool {
	if s.pos >= len(s.data) {
		return false
	}
	end := bytes.IndexByte(s.data[s.pos:], '\n')
	if end < 0 {
		s.line = s.data[s.pos:]
		s.pos = len(s.data)
	} else {
		s.line = s.data[s.pos : s.pos+end]
		s.pos += end + 1
	}
	return true
}

// point returns the 5th space-separated field of mountinfo (the mount point).
func (s *mountInfoScanner) point() string {
	field := 0
	start := 0
	for i := 0; i <= len(s.line); i++ {
		if i == len(s.line) || s.line[i] == ' ' {
			if field == 4 {
				return unescapeOctal(s.line[start:i])
			}
			field++
			start = i + 1
		}
	}
	return ""
}

// unescapeOctal decodes mountinfo octal escapes
func unescapeOctal(b []byte) string {
	if bytes.IndexByte(b, '\\') < 0 {
		return string(b)
	}
	var sb strings.Builder
	sb.Grow(len(b))
	for i := 0; i < len(b); i++ {
		if b[i] == '\\' && i+3 < len(b) {
			if c, ok := parseOctal(b[i+1], b[i+2], b[i+3]); ok {
				sb.WriteByte(c)
				i += 3
				continue
			}
		}
		sb.WriteByte(b[i])
	}
	return sb.String()
}

func parseOctal(a, b, c byte) (byte, bool) {
	if a < '0' || a > '7' || b < '0' || b > '7' || c < '0' || c > '7' {
		return 0, false
	}
	return (a-'0')<<6 | (b-'0')<<3 | (c - '0'), true
}
