#!/bin/sh
# Regenerate the gettext .pot template and merge into existing .po files.
set -eu

cd "$(dirname "$0")/.."

POT="internal/i18n/default.pot"
PO_DIR="internal/i18n/po"
XGOTEXT="github.com/Tom5521/xgotext@v1.2.0"

go run "$XGOTEXT" --output "$POT"
msguniq --use-first -o "$POT" "$POT"

for po in "$PO_DIR"/*/default.po; do
    [ -f "$po" ] || continue
    msgmerge --backup=off -U "$po" "$POT"
    echo "merged $po"
done
