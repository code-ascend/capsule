package mountinfo

import (
	"bufio"
	"os"
	"strings"
)

// Points returns all current mount points, with kernel octal escapes decoded.
func Points() ([]string, error) {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()

	var points []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) >= 5 {
			points = append(points, unescape(fields[4]))
		}
	}
	return points, scanner.Err()
}

// IsMounted reports whether point is currently a mount point.
func IsMounted(point string) bool {
	if point == "" {
		return false
	}
	points, _ := Points()
	for _, p := range points {
		if p == point {
			return true
		}
	}
	return false
}

// unescape decodes the kernel's octal escapes
func unescape(s string) string {
	if !strings.ContainsRune(s, '\\') {
		return s
	}
	var b strings.Builder
	b.Grow(len(s))
	for i := 0; i < len(s); i++ {
		if s[i] == '\\' && i+4 <= len(s) &&
			isOctalDigit(s[i+1]) && isOctalDigit(s[i+2]) && isOctalDigit(s[i+3]) {
			n := int(s[i+1]-'0')<<6 | int(s[i+2]-'0')<<3 | int(s[i+3]-'0')
			if n <= 0xFF {
				b.WriteByte(byte(n))
				i += 3
				continue
			}
		}
		b.WriteByte(s[i])
	}
	return b.String()
}

func isOctalDigit(c byte) bool { return c >= '0' && c <= '7' }
