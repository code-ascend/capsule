package nvidia

import (
	"context"
	"fmt"

	"capsule/internal/runtime/bundle"
	"capsule/internal/sys/log"
)

func Setup(ctx context.Context, b *bundle.Extractor, mergedRoot, markerPath string) error {
	if !IsAvailable() {
		return nil
	}
	if IsCached(markerPath) {
		log.Debug("nvidia driver already cached")
		return nil
	}
	version, err := HostDriverVersion()
	if err != nil {
		return err
	}
	log.Info("nvidia setting up driver", "version", version)

	entries, err := RunLdConfig()
	if err != nil {
		return fmt.Errorf("ldconfig: %w", err)
	}

	if err := CleanUpper(mergedRoot); err != nil {
		log.Debug("nvidia clean upper failed", "err", err)
	}

	layout := DetectLayout(mergedRoot)

	count := 0
	for _, p := range CollectLibPaths(entries) {
		dst, err := CopyLib(p, mergedRoot, layout, version)
		if err != nil {
			log.Debug("nvidia lib copy failed", "src", p, "err", err)
			continue
		}
		if dst != "" {
			count++
		}
	}
	log.Debug("nvidia libs copied", "count", count)

	CopyConfigs(mergedRoot, version)
	CopyEGLVendor(mergedRoot)
	CopyEGLPlatform(mergedRoot)
	CopyVulkanFallbacks(mergedRoot)
	CopyWineDLSS(mergedRoot)
	CopyWaylandServerLib(mergedRoot, layout, entries)
	CopyDRIVAAPI(mergedRoot, layout)
	CopyGBM(mergedRoot, layout)
	CopyALTNonStandard(mergedRoot)
	CopyALTMesaDRI(mergedRoot)

	RebuildLdCache(ctx, b, mergedRoot)
	return WriteCacheMarker(markerPath)
}
