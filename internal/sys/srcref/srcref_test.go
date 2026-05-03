package srcref

import (
	"path/filepath"
	"testing"
)

func TestIsRemote(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"", false},
		{"http://example.org/x.yaml", true},
		{"https://example.org/x.yaml", true},
		{"HTTP://example.org/x.yaml", false},
		{"/abs/path", false},
		{"relative/path.yaml", false},
		{"ftp://example.org/x", false},
	}
	for _, c := range cases {
		if got := IsRemote(c.in); got != c.want {
			t.Errorf("IsRemote(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}

func TestNormalize(t *testing.T) {
	url := "https://example.org/x.yaml"
	if got := Normalize(url); got != url {
		t.Errorf("Normalize(%q) = %q, want unchanged", url, got)
	}

	abs := Normalize("relative/path.yaml")
	if !filepath.IsAbs(abs) {
		t.Errorf("Normalize(relative) = %q, want absolute path", abs)
	}

	want, _ := filepath.Abs("/already/abs")
	if got := Normalize("/already/abs"); got != want {
		t.Errorf("Normalize(/already/abs) = %q, want %q", got, want)
	}
}
