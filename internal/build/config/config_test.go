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
	cfg, err := Load(path)
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

	cfg, err := Load(srv.URL + "/c.yaml")
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

	if _, err := Load(srv.URL + "/missing.yaml"); err == nil {
		t.Fatal("expected error on 404")
	}
}

func TestLoadHTMLResponseRejected(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		_, _ = w.Write([]byte("<html><body>not yaml</body></html>"))
	}))
	defer srv.Close()

	_, err := Load(srv.URL + "/page.yaml")
	if err == nil {
		t.Fatal("expected error on HTML response")
	}
	if !strings.Contains(err.Error(), "raw") {
		t.Errorf("error should hint raw URL, got: %v", err)
	}
}

func TestLoadHTTPSPrefixDetected(t *testing.T) {
	_, err := Load("http://127.0.0.1:1/x.yaml")
	if err == nil {
		t.Fatal("expected dial error")
	}
	if _, statErr := os.Stat("http://127.0.0.1:1/x.yaml"); statErr == nil {
		t.Skip("path-as-file exists — inconclusive")
	}
}
