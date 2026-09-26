#!/usr/bin/env sh
# Design-token guard: pages/components must use the tokens in src/styles/tokens.css
# (via Tailwind token classes or shell classes), never literal colours.
#   - hex colour literals            -> add/use a token
#   - raw Tailwind palette classes   -> use tone-*/soft-*/term-*/ink-* / shadcn tokens
# Terminal/xterm colours live in tokens (--terminal-*, --tok-*), see lib/terminalTheme.js.
set -eu
cd "$(dirname "$0")/.."
DIRS="src/pages src/components"
fail=0

hex=$(grep -rnE "#[0-9a-fA-F]{3,8}\b" $DIRS --include='*.jsx' --include='*.js' | grep -vE "^\S+:[0-9]+:\s*(//|\*|/\*)" || true)
if [ -n "$hex" ]; then
  echo "✗ hex colour literals (use a token from src/styles/tokens.css):"; echo "$hex"; fail=1
fi

pal=$(grep -rnE "\b(bg|text|border|ring|fill|stroke|from|to|via|divide|outline|accent|shadow)-(slate|zinc|gray|neutral|stone|red|rose|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink)-[0-9]{2,3}\b|\b(bg|text|border|ring)-(white|black)(/[0-9]+|\b)" $DIRS --include='*.jsx' || true)
if [ -n "$pal" ]; then
  echo "✗ raw Tailwind palette classes (use tone-*, soft-*, term-*, ink-* or shadcn tokens):"; echo "$pal"; fail=1
fi

if [ "$fail" -eq 0 ]; then echo "✓ design lint clean"; fi
exit "$fail"
