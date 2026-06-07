package manager

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"capsule/internal/format/binconfig"
	"capsule/internal/format/selfread"
)

func TestClassify(t *testing.T) {
	tmp := t.TempDir()
	existing := filepath.Join(tmp, "yaml")
	if err := os.WriteFile(existing, []byte("x"), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}

	cases := []struct {
		ref  string
		want SourceKind
	}{
		{"", SourceUnknown},
		{"https://example.org/foo.yaml", SourceExternal},
		{"http://example.org/foo.yaml", SourceExternal},
		{existing, SourceLocalPresent},
		{filepath.Join(tmp, "missing.yaml"), SourceLocalMissing},
	}
	for _, c := range cases {
		got := classify(c.ref)
		if got != c.want {
			t.Errorf("classify(%q) = %v, want %v", c.ref, got, c.want)
		}
	}
}

func TestScan(t *testing.T) {
	dir := t.TempDir()

	yamlPath := filepath.Join(dir, "recipe.yaml")
	if err := os.WriteFile(yamlPath, []byte("image: x\n"), 0o644); err != nil {
		t.Fatalf("write yaml: %v", err)
	}
	writeFakeCapsule(t, filepath.Join(dir, "alpha"), &binconfig.Config{
		Compression: "zstd",
		SourceRef:   yamlPath,
		BuiltAt:     "2026-05-04T00:00:00Z",
	})

	writeFakeCapsule(t, filepath.Join(dir, "beta"), &binconfig.Config{
		Compression: "zstd",
		SourceRef:   "https://example.org/beta.yaml",
	})

	writeFakeCapsule(t, filepath.Join(dir, "gamma"), &binconfig.Config{
		Compression: "zstd",
		SourceRef:   filepath.Join(dir, "deleted.yaml"),
	})

	writeFakeCapsule(t, filepath.Join(dir, "delta"), &binconfig.Config{
		Compression: "zstd",
	})

	if err := os.WriteFile(filepath.Join(dir, "notacapsule"), bytes.Repeat([]byte{0x55}, 4096), 0o755); err != nil {
		t.Fatalf("write noncapsule: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dir, "readme.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatalf("write text: %v", err)
	}

	mgr := &Manager{roots: []scanRoot{{path: dir, user: true}}}
	caps := mgr.Scan()
	if len(caps) != 4 {
		t.Fatalf("want 4 capsules, got %d: %+v", len(caps), names(caps))
	}

	want := map[string]SourceKind{
		"alpha": SourceLocalPresent,
		"beta":  SourceExternal,
		"gamma": SourceLocalMissing,
		"delta": SourceUnknown,
	}
	for _, c := range caps {
		base := filepath.Base(c.Path)
		k, ok := want[base]
		if !ok {
			t.Errorf("unexpected capsule: %s", base)
			continue
		}
		if c.Kind != k {
			t.Errorf("%s: kind = %v, want %v", base, c.Kind, k)
		}
	}
}

func names(caps []Capsule) []string {
	out := make([]string, 0, len(caps))
	for _, c := range caps {
		out = append(out, filepath.Base(c.Path))
	}
	return out
}

// writeFakeCapsule writes a minimum file that passes selfread.IsCapsule
func writeFakeCapsule(t *testing.T, path string, cfg *binconfig.Config) {
	t.Helper()
	jsn, err := binconfig.Marshal(cfg)
	if err != nil {
		t.Fatalf("marshal cfg: %v", err)
	}
	var buf bytes.Buffer
	buf.Write(bytes.Repeat([]byte{0xCC}, 1024))
	buf.Write(jsn)
	if err := selfread.EncodeFooter(&buf, int64(len(jsn)), 0); err != nil {
		t.Fatalf("encode footer: %v", err)
	}
	if err := os.WriteFile(path, buf.Bytes(), 0o755); err != nil {
		t.Fatalf("write: %v", err)
	}
}
