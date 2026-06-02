package mount

import "testing"

func TestUnescapeOctal(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"/mnt", "/mnt"},
		{`/mnt\040point`, "/mnt point"},
		{`/tab\011here`, "/tab\there"},
		{`/back\134slash`, `/back\slash`},
		{`/a\040b\040c`, "/a b c"},
		{`/trailing\04`, `/trailing\04`}, // malformed (2 digits) — left as-is
	}
	for _, c := range cases {
		if got := unescapeOctal(c.in); got != c.want {
			t.Errorf("unescapeOctal(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
