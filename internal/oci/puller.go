package oci

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"capsule/internal/config"
	"capsule/internal/log"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/tarball"
)

// Pull downloads an OCI image and returns the path to the tarball.
func Pull(ctx context.Context, imageRef string) (string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return "", fmt.Errorf("invalid image reference: %w", err)
	}

	log.Debug("Pulling image", "ref", ref.String())

	tmpDir, err := os.MkdirTemp("", config.TempPrefixImage)
	if err != nil {
		return "", fmt.Errorf("failed to create temp directory: %w", err)
	}

	success := false
	defer func() {
		if !success {
			os.RemoveAll(tmpDir)
		}
	}()

	img, err := remote.Image(ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	)
	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w", err)
	}

	if digest, err := img.Digest(); err == nil {
		log.Debug("Image digest", "digest", digest.String())
	}

	tarPath := filepath.Join(tmpDir, config.ImageTar)
	if err = tarball.WriteToFile(tarPath, ref, img); err != nil {
		return "", fmt.Errorf("failed to save image: %w", err)
	}

	if info, err := os.Stat(tarPath); err == nil {
		log.Debug("Image saved", "size_mb", fmt.Sprintf("%.2f", float64(info.Size())/(1024*1024)))
	}

	success = true
	return tarPath, nil
}
