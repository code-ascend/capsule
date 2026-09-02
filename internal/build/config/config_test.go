package config

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const minimalYAML = `image: registry.altlinux.org/sisyphus/base:latest
output: ./out
compression: zstd
`

func TestLoadFromDisk(t *testing.T) {
	path := filepath.Join(t.TempDir(), "c.yaml")
	if err := os.WriteFile(path, []byte(minimalYAML), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, _, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Image != "registry.altlinux.org/sisyphus/base:latest" {
		t.Errorf("Image=%q", cfg.Image)
	}
	if cfg.Compression != "zstd" {
		t.Errorf("Compression=%q", cfg.Compression)
	}
	if !cfg.HostExec {
		t.Errorf("HostExec must default to true")
	}
}

func TestBindsCarriedToBinConfig(t *testing.T) {
	yaml := minimalYAML + "binds:\n  - ~/.local/share/myapp:/data\n  - /srv/shared\n"
	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	bc := cfg.ToBinConfig(BuildMeta{})
	want := []string{"~/.local/share/myapp:/data", "/srv/shared"}
	if len(bc.Binds) != len(want) {
		t.Fatalf("Binds=%v, want %v", bc.Binds, want)
	}
	for i := range want {
		if bc.Binds[i] != want[i] {
			t.Errorf("Binds[%d]=%q, want %q", i, bc.Binds[i], want[i])
		}
	}
}

func TestNoOverlayNoNvidiaCarriedToBinConfig(t *testing.T) {
	yaml := minimalYAML + "no_overlay: true\nno_nvidia: true\n"
	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	bc := cfg.ToBinConfig(BuildMeta{})
	if !bc.NoOverlay || !bc.NoNvidia {
		t.Errorf("NoOverlay=%v NoNvidia=%v, want both true", bc.NoOverlay, bc.NoNvidia)
	}
}

func TestOnStartCarriedToBinConfig(t *testing.T) {
	yaml := minimalYAML + "on_start:\n  - run: mkdir -p /tmp/myapp\n  - run: touch /tmp/myapp/ready\n"
	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	bc := cfg.ToBinConfig(BuildMeta{})
	want := "mkdir -p /tmp/myapp\ntouch /tmp/myapp/ready"
	if bc.StartScript != want {
		t.Errorf("StartScript=%q, want %q", bc.StartScript, want)
	}
}

func TestMetadataCarriedToBinConfig(t *testing.T) {
	yaml := minimalYAML + "metadata:\n  vendor: acme\n  gui:\n    packages:\n      - firefox\n"
	cfg, err := LoadFromBytes([]byte(yaml))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	bc := cfg.ToBinConfig(BuildMeta{})
	if bc.Metadata["vendor"] != "acme" {
		t.Errorf("Metadata[vendor]=%v, want acme", bc.Metadata["vendor"])
	}
	gui, ok := bc.Metadata["gui"].(map[string]any)
	if !ok {
		t.Fatalf("Metadata[gui]=%T, want map", bc.Metadata["gui"])
	}
	pkgs, ok := gui["packages"].([]any)
	if !ok || len(pkgs) != 1 || pkgs[0] != "firefox" {
		t.Errorf("Metadata[gui][packages]=%v, want [firefox]", gui["packages"])
	}
}

func TestMetadataSizeLimit(t *testing.T) {
	yaml := minimalYAML + "metadata:\n  blob: " + strings.Repeat("x", MaxMetadataBytes) + "\n"
	if _, err := LoadFromBytes([]byte(yaml)); err == nil {
		t.Fatal("expected error on oversized metadata")
	}
}

func TestMetadataMergedFromOverrides(t *testing.T) {
	cfg, err := LoadFromBytes([]byte(minimalYAML + "metadata:\n  vendor: acme\n  keep: true\n"))
	if err != nil {
		t.Fatalf("LoadFromBytes: %v", err)
	}
	rootfs := t.TempDir()
	overrides := "metadata:\n  vendor: overridden\n  extra: 1\n"
	if err := os.WriteFile(filepath.Join(rootfs, OverridesFile), []byte(overrides), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := cfg.ApplyOverrides(rootfs); err != nil {
		t.Fatalf("ApplyOverrides: %v", err)
	}
	if cfg.Metadata["vendor"] != "overridden" {
		t.Errorf("Metadata[vendor]=%v, want overridden", cfg.Metadata["vendor"])
	}
	if cfg.Metadata["keep"] != true {
		t.Errorf("Metadata[keep]=%v, want true", cfg.Metadata["keep"])
	}
	if cfg.Metadata["extra"] != 1 {
		t.Errorf("Metadata[extra]=%v, want 1", cfg.Metadata["extra"])
	}
}

func TestSandboxValidation(t *testing.T) {
	base := minimalYAML + "sandbox: isolated\n"
	if _, err := LoadFromBytes([]byte(base)); err != nil {
		t.Fatalf("valid sandbox rejected: %v", err)
	}
	if _, err := LoadFromBytes([]byte(minimalYAML + "sandbox: bogus\n")); err == nil {
		t.Fatal("expected error on invalid sandbox mode")
	}
	if _, err := LoadFromBytes([]byte(minimalYAML)); err != nil {
		t.Fatalf("empty sandbox should be allowed: %v", err)
	}
}

func TestLoadFromHTTP(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(minimalYAML))
	}))
	defer srv.Close()

	cfg, _, err := Load(srv.URL + "/c.yaml")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Image != "registry.altlinux.org/sisyphus/base:latest" {
		t.Errorf("Image=%q", cfg.Image)
	}
}

func TestLoadHTTPNotFound(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.NotFound(w, r)
	}))
	defer srv.Close()

	if _, _, err := Load(srv.URL + "/missing.yaml"); err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestLoadHTMLResponseRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>not yaml</body></html>"))
	}))
	defer srv.Close()

	_, _, err := Load(srv.URL + "/page.yaml")
	if err == nil {
		t.Fatal("expected error on HTML response")
	}
	if !strings.Contains(err.Error(), "raw") {
		t.Errorf("error should hint raw URL, got: %v", err)
	}
}

func TestLoadHTTPSPrefixDetected(t *testing.T) {
	_, _, err := Load("http://127.0.0.1:1/x.yaml")
	if err == nil {
		t.Fatal("expected dial error")
	}
	if _, statErr := os.Stat("http://127.0.0.1:1/x.yaml"); statErr == nil {
		t.Skip("path-as-file exists — inconclusive")
	}
}
