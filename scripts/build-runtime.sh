#!/bin/sh
# Build the in-capsule runtime binary that gets embedded into the final ELF.
#
# Output: internal/build/embed/files/capsule-runtime
#
# Run this before `go build ./cmd/capsule`. The runtime is built static (no
# CGO) so it can run unmodified inside any user namespace the capsule lands in.
set -eu

cd "$(dirname "$0")/.."

OUT="internal/build/embed/files/capsule-runtime"
mkdir -p "$(dirname "$OUT")"

CGO_ENABLED=0 \
    go build \
    -tags 'osusergo netgo' \
    -ldflags '-s -w' \
    -trimpath \
    -o "$OUT" \
    ./cmd/capsule-runtime

chmod +x "$OUT"

size=$(stat -c%s "$OUT" 2>/dev/null || stat -f%z "$OUT")
mb=$(LC_ALL=C awk "BEGIN { printf \"%.2f\", $size/1048576 }")
echo "capsule-runtime: $OUT ($mb MB)"
