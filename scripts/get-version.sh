#!/bin/bash

set -euo pipefail

BASE_VERSION="0.1.0"

TAG=$(git describe --tags --abbrev=0 --match 'v*' 2>/dev/null || true)

if [ -n "$TAG" ]; then
    VERSION="${TAG#v}"
    VERSION="${VERSION%-alt*}"
    COMMIT_COUNT=$(git rev-list --count "${TAG}..HEAD" 2>/dev/null || echo "0")
else
    VERSION="$BASE_VERSION"
    COMMIT_COUNT=$(git rev-list --count HEAD 2>/dev/null || echo "0")
fi

echo -n "${VERSION}"

if [ "$COMMIT_COUNT" != "0" ]; then
    echo -n ".${COMMIT_COUNT}"
fi