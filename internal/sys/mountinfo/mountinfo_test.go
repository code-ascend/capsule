package mountinfo

import "testing"

func TestUnescape(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/mnt", "/mnt"},
		{`/mnt\040point`, "/mnt point"},
		{`/tab\011here`, "/tab\there"},
		{`/back\134slash`, `/back\slash`},
		{`/a\040b\040c`, "/a b c"},
		{`/trailing\04`, `/trailing\04`},   // malformed (2 digits) — left as-is
		{`/over\777flow`, `/over\777flow`}, // > 0xFF — left as-is, no byte wrap
	}
	for _, c := range cases {
		if got := unescape(c.in); got != c.want {
			t.Errorf("unescape(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
