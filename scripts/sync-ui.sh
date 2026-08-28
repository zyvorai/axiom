#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)

(cd "$ROOT/frontend" && npm install --legacy-peer-deps && npx craco build)

rm -rf "$ROOT/internal/ui/dist"
mkdir -p "$ROOT/internal/ui/dist"
cp -r "$ROOT/frontend/build/." "$ROOT/internal/ui/dist/"
echo "Axiom UI built and synchronized."
