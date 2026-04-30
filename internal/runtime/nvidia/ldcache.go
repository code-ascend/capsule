package nvidia

import (
	"context"
	"os"

	"capsule/internal/runtime/bundle"
)

// RebuildLdCache runs ldconfig inside the merged root so newly dropped libs
// become resolvable. Best-effort.
func RebuildLdCache(ctx context.Context, b *bundle.Extractor, mergedRoot string) {
	cmd := b.Command(ctx, "bwrap",
		"--bind", mergedRoot, "/",
		"--dev-bind", "/dev", "/dev",
		"--proc", "/proc",
		"--",
		"ldconfig",
	)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	_ = cmd.Run()
}
