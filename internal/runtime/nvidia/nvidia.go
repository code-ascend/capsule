package nvidia

import (
	"context"

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

	// Strip prior files so a driver upgrade replaces them cleanly.
	_ = CleanUpper(mergedRoot)

	layout := DetectLayout(mergedRoot)

	entries, err := RunLdConfig()
	if err != nil {
		log.Warn("ldconfig run failed", "err", err)
		return nil
	}

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
