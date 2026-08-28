#!/usr/bin/env sh
set -eu
ROOT=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$ROOT/internal/ui/dist/fonts"
cp "$ROOT/web/index.html" "$ROOT/internal/ui/dist/index.html"
cp "$ROOT/web/styles.css" "$ROOT/internal/ui/dist/styles.css"
cp "$ROOT/web/app.js" "$ROOT/internal/ui/dist/app.js"
cp "$ROOT/web/lucide-icons.js" "$ROOT/internal/ui/dist/lucide-icons.js"
cp "$ROOT/web/fonts/"*.woff2 "$ROOT/internal/ui/dist/fonts/"
echo "Axiom UI synchronized."
