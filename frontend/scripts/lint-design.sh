#!/usr/bin/env sh
# Design-token guard: the UI must use the tokens in src/styles/tokens.css
# (via Tailwind token classes, shell classes or var(--…)), never literal colours.
#   1. hex colour literals                      (pages/components JS + all CSS)
#   2. rgb()/rgba()/hsl()/hsla()/oklch() values (all src JS + CSS; hsl(var(--x)) is fine)
#   3. raw Tailwind palette classes / bg-white|black (pages, components)
# styles/tokens.css is the only file allowed to define colours. Terminal/xterm
# colours live in tokens (--terminal-*, --tok-*), see lib/terminalTheme.js.
set -eu
cd "$(dirname "$0")/.."
fail=0
# lines that are only comments never count
not_comment='^\S+:[0-9]+:\s*(//|\*|/\*)'

hex=$( { grep -rnE "#[0-9a-fA-F]{3,8}\b" src/pages src/components --include='*.jsx' --include='*.js'
         grep -rnE "#[0-9a-fA-F]{3,8}\b" src --include='*.css' --exclude=tokens.css; } | grep -vE "$not_comment" | grep -vE "/\*.*#[0-9a-fA-F]{3,8}.*\*/" || true)
if [ -n "$hex" ]; then echo "✗ hex colour literals (add/use a token in src/styles/tokens.css):"; echo "$hex"; fail=1; fi

fn=$(grep -rnE "\b(rgba?|hsla?|oklch|oklab)\(" src --include='*.css' --include='*.jsx' --include='*.js' --exclude=tokens.css \
       | grep -vE "$not_comment" | grep -vE "hsl\(var\(--" | grep -vE "/\*.*(rgba?|hsla?)\(.*\*/" || true)
if [ -n "$fn" ]; then echo "✗ rgb()/hsl() colour literals (add/use a token in src/styles/tokens.css):"; echo "$fn"; fail=1; fi

pal=$(grep -rnE "\b(bg|text|border|ring|fill|stroke|from|to|via|divide|outline|accent|shadow)-(slate|zinc|gray|neutral|stone|red|rose|orange|amber|yellow|lime|green|emerald|teal|cyan|sky|blue|indigo|violet|purple|fuchsia|pink)-[0-9]{2,3}\b|\b(bg|text|border|ring)-(white|black)(/[0-9]+|\b)" src/pages src/components --include='*.jsx' || true)
if [ -n "$pal" ]; then echo "✗ raw Tailwind palette classes (use tone-*, soft-*, term-*, ink-* or shadcn tokens):"; echo "$pal"; fail=1; fi

if [ "$fail" -eq 0 ]; then echo "✓ design lint clean"; fi
exit "$fail"
