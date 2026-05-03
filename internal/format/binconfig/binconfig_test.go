package binconfig

import (
	"reflect"
	"testing"
)

func TestRoundTrip(t *testing.T) {
	in := &Config{
		Launch:       "/usr/bin/foo",
		Compression:  "zstd",
		UpdateScript: "apt-get update\napt-get -y upgrade",
		Apps: []AppExport{
			{Desktop: "/usr/share/applications/a.desktop", Icon: "a", NameSuffix: " (capsule)"},
		},
		Binaries:  []string{"/usr/bin/foo", "/usr/bin/bar"},
		EnvUnset:  []string{"LD_PRELOAD"},
		EnvSet:    map[string]string{"FOO": "bar"},
		SourceRef: "https://example.org/recipe.yaml",
		SourceSHA: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
		BuiltAt:   "2026-05-03T18:06:09Z",
	}

	data, err := Marshal(in)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	out, err := Unmarshal(data)
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if !reflect.DeepEqual(in, out) {
		t.Fatalf("mismatch:\n in=%+v\nout=%+v", in, out)
	}
}

func TestUnmarshalEmpty(t *testing.T) {
	c, err := Unmarshal([]byte(`{"compression":"zstd"}`))
	if err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if c.Compression != "zstd" {
		t.Fatalf("compression=%q", c.Compression)
	}
	if len(c.Apps) != 0 || len(c.Binaries) != 0 || len(c.EnvSet) != 0 {
		t.Fatalf("expected empty slices/maps, got %+v", c)
	}
}
