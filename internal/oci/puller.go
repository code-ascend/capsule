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

	desc, err := remote.Head(ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	)
	if err != nil {
		log.Debug("HEAD request failed, trying full pull", "error", err)
		return pullWithoutCache(ctx, ref)
	}

	digest := desc.Digest.Hex
	log.Debug("Image digest", "digest", digest[:12])

	cachePath := filepath.Join(config.CacheDir, digest+".tar")
	if info, err := os.Stat(cachePath); err == nil && info.Size() > 0 {
		log.Info("Using cached image", "digest", digest[:12], "size_mb", fmt.Sprintf("%.2f", float64(info.Size())/(1024*1024)))
		return cachePath, nil
	}

	if err = os.MkdirAll(config.CacheDir, 0755); err != nil {
		log.Debug("Failed to create cache dir, using temp", "error", err)
		return pullWithoutCache(ctx, ref)
	}

	img, err := remote.Image(ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
	)
	if err != nil {
		return "", fmt.Errorf("failed to pull image: %w", err)
	}

	tmpPath := cachePath + ".tmp"
	defer os.Remove(tmpPath)

	if err = tarball.WriteToFile(tmpPath, ref, img); err != nil {
		log.Debug("Failed to write cache, using temp", "error", err)
		return pullWithoutCache(ctx, ref)
	}

	if err = os.Rename(tmpPath, cachePath); err != nil {
		log.Debug("Failed to rename cache file, using temp", "error", err)
		return pullWithoutCache(ctx, ref)
	}

	if info, err := os.Stat(cachePath); err == nil {
		log.Info("Image cached", "digest", digest[:12], "size_mb", fmt.Sprintf("%.2f", float64(info.Size())/(1024*1024)))
	}

	return cachePath, nil
}

// pullWithoutCache downloads image to temp directory (fallback)
func pullWithoutCache(ctx context.Context, ref name.Reference) (string, error) {
	tmpDir, err := os.MkdirTemp(config.TempDir, config.TempPrefixImage)
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
