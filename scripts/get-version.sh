#!/bin/bash

set -euo pipefail

VERSION=$1

TAG="v$VERSION"

echo -n "${VERSION}"

COMMIT_COUNT=$(git rev-list --count "${TAG}..HEAD" 2>/dev/null || echo "0")

if [ "$COMMIT_COUNT" != "0" ]; then
    echo -n ".${COMMIT_COUNT}"
fi
