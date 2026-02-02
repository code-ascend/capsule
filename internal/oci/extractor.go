package oci

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"capsule/internal/config"
	"capsule/internal/log"
)

// imageManifest represents the manifest.json in an OCI tarball.
type imageManifest struct {
	Config   string   `json:"Config"`
	RepoTags []string `json:"RepoTags"`
	Layers   []string `json:"Layers"`
}

// Extract extracts an OCI image tarball to a rootfs directory.
func Extract(ctx context.Context, tarPath string) (string, error) {
	tmpDir, err := os.MkdirTemp(config.TempDir, config.TempPrefixImage)
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	rootfsDir := filepath.Join(tmpDir, config.RootfsDir)
	if err = os.MkdirAll(rootfsDir, 0755); err != nil {
		os.RemoveAll(tmpDir)
		return "", fmt.Errorf("failed to create rootfs directory: %w", err)
	}

	manifest, layerData, err := readTarball(tarPath)
	if err != nil {
		return "", fmt.Errorf("failed to read tarball: %w", err)
	}

	log.Debug("Found layers", "count", len(manifest.Layers))

	for i, layerPath := range manifest.Layers {
		if err = ctx.Err(); err != nil {
			return "", err
		}

		log.Debug("Extracting layer", "num", i+1, "total", len(manifest.Layers))

		data, ok := layerData[layerPath]
		if !ok {
			return "", fmt.Errorf("layer not found in tarball: %s", layerPath)
		}

		if err = extractLayer(data, rootfsDir); err != nil {
			return "", fmt.Errorf("failed to extract layer %d: %w", i+1, err)
		}
	}

	return rootfsDir, nil
}

// readTarball reads the manifest and all layer data from the tarball in a single pass.
func readTarball(tarPath string) (*imageManifest, map[string][]byte, error) {
	f, err := os.Open(tarPath)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to open tarball: %w", err)
	}
	defer f.Close()

	var manifest *imageManifest
	layerData := make(map[string][]byte)

	tr := tar.NewReader(f)
	for {
		hdr, errReader := tr.Next()
		if errReader == io.EOF {
			break
		}
		if errReader != nil {
			return nil, nil, fmt.Errorf("failed to read tar header: %w", errReader)
		}

		switch {
		case hdr.Name == "manifest.json":
			var manifests []imageManifest
			if err = json.NewDecoder(tr).Decode(&manifests); err != nil {
				return nil, nil, fmt.Errorf("failed to decode manifest: %w", err)
			}
			if len(manifests) == 0 {
				return nil, nil, fmt.Errorf("empty manifest")
			}
			manifest = &manifests[0]

		case strings.HasSuffix(hdr.Name, ".tar.gz") || strings.HasSuffix(hdr.Name, "/layer.tar"):
			data, err := io.ReadAll(tr)
			if err != nil {
				return nil, nil, fmt.Errorf("failed to read layer %s: %w", hdr.Name, err)
			}
			layerData[hdr.Name] = data
		}
	}

	if manifest == nil {
		return nil, nil, fmt.Errorf("manifest.json not found")
	}

	return manifest, layerData, nil
}

// extractLayer extracts a gzipped layer tar to the rootfs.
func extractLayer(data []byte, rootfsDir string) error {
	gr, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return fmt.Errorf("failed to create gzip reader: %w", err)
	}
	defer gr.Close()

	tr := tar.NewReader(gr)
	for {
		hdr, errReader := tr.Next()
		if errReader == io.EOF {
			break
		}
		if errReader != nil {
			return fmt.Errorf("failed to read tar entry: %w", errReader)
		}

		if err = extractEntry(tr, hdr, rootfsDir); err != nil {
			return err
		}
	}

	return nil
}

// extractEntry extracts a single tar entry to the rootfs.
func extractEntry(tr *tar.Reader, hdr *tar.Header, rootfsDir string) error {
	name := hdr.Name

	if strings.HasPrefix(filepath.Base(name), ".wh.") {
		target := filepath.Join(rootfsDir, filepath.Dir(name), strings.TrimPrefix(filepath.Base(name), ".wh."))
		os.RemoveAll(target)
		return nil
	}

	target := filepath.Join(rootfsDir, name)

	if err := os.MkdirAll(filepath.Dir(target), 0755); err != nil {
		return fmt.Errorf("failed to create parent directory for %s: %w", name, err)
	}

	switch hdr.Typeflag {
	case tar.TypeDir:
		if err := os.MkdirAll(target, os.FileMode(hdr.Mode)); err != nil {
			return fmt.Errorf("failed to create directory %s: %w", name, err)
		}

	case tar.TypeReg:
		if err := extractFile(tr, target, os.FileMode(hdr.Mode)); err != nil {
			return fmt.Errorf("failed to extract file %s: %w", name, err)
		}

	case tar.TypeSymlink:
		os.Remove(target)
		if err := os.Symlink(hdr.Linkname, target); err != nil {
			log.Debug("Failed to create symlink", "target", target, "link", hdr.Linkname, "error", err)
		}

	case tar.TypeLink:
		os.Remove(target)
		linkTarget := filepath.Join(rootfsDir, hdr.Linkname)
		if err := os.Link(linkTarget, target); err != nil {
			log.Debug("Failed to create hard link", "target", target, "link", hdr.Linkname, "error", err)
		}

	case tar.TypeChar, tar.TypeBlock, tar.TypeFifo:
		// Skip device nodes and FIFOs
	}

	return nil
}

// extractFile extracts a regular file from tar.
func extractFile(tr *tar.Reader, target string, mode os.FileMode) error {
	os.Remove(target)

	f, err := os.OpenFile(target, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, mode)
	if err != nil {
		return err
	}
	defer f.Close()

	if _, err = io.Copy(f, tr); err != nil {
		return err
	}

	return nil
}
