#!/bin/sh
# Build the in-capsule runtime binary that gets embedded into the final ELF.
#
# Output: internal/build/embed/files/capsule-runtime
set -eu

cd "$(dirname "$0")/.."

OUT="internal/build/embed/files/capsule-runtime"
mkdir -p "$(dirname "$OUT")"

CGO_ENABLED=0 \
    go build \
    -tags 'osusergo netgo' \
    -ldflags "-s -w ${CAPSULE_LDFLAGS:-}" \
    -trimpath \
    -o "$OUT" \
    ./cmd/capsule-runtime

chmod +x "$OUT"

size=$(stat -c%s "$OUT" 2>/dev/null || stat -f%z "$OUT")
mb=$(LC_ALL=C awk "BEGIN { printf \"%.2f\", $size/1048576 }")
echo "capsule-runtime: $OUT ($mb MB)"
