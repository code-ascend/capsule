package mount

import "bytes"

type mountinfoScanner struct {
	data []byte
	pos  int
	line []byte
}

func mountinfoScan(data []byte) *mountinfoScanner {
	return &mountinfoScanner{data: data}
}

func (s *mountinfoScanner) next() bool {
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
func (s *mountinfoScanner) point() string {
	field := 0
	start := 0
	for i := 0; i <= len(s.line); i++ {
		if i == len(s.line) || s.line[i] == ' ' {
			if field == 4 {
				return string(s.line[start:i])
			}
			field++
			start = i + 1
		}
	}
	return ""
}
