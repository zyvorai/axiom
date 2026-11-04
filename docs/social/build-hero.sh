#!/usr/bin/env bash
# Render docs/social/axiom-hero.html (1200x630 @2x) to a JPEG for the README, LinkedIn and X.
set -euo pipefail
HERE="$(cd "$(dirname "$0")" && pwd)"
OUT="${1:-$HERE/axiom-hero.jpg}"
CHROME="${CHROME:-/Applications/Google Chrome.app/Contents/MacOS/Google Chrome}"
PNG="$(mktemp "${TMPDIR:-/tmp}/axiom-hero.XXXXXX.png")"; trap 'rm -f "$PNG"' EXIT
"$CHROME" --headless=new --disable-gpu --hide-scrollbars --force-device-scale-factor=2 \
  --window-size=1200,630 --screenshot="$PNG" "file://$HERE/axiom-hero.html" >/dev/null 2>&1
sips -s format jpeg -s formatOptions 92 "$PNG" --out "$OUT" >/dev/null
echo "wrote $OUT"
