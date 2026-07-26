#!/bin/sh
set -eu

version=v3.4.0
archive_sha256=8e617904b980fe3648a4b116808788fe50c99d2d495376cb7c0badbd8a564c47
project_root=$(CDPATH= cd -- "$(dirname "$0")/.." && pwd)
work_dir=$(mktemp -d)
trap 'rm -rf "$work_dir"' EXIT

curl -fsSL "https://github.com/ryanoasis/nerd-fonts/releases/download/${version}/NerdFontsSymbolsOnly.zip" -o "$work_dir/symbols.zip"
printf '%s  %s\n' "$archive_sha256" "$work_dir/symbols.zip" | shasum -a 256 -c -
unzip -q "$work_dir/symbols.zip" SymbolsNerdFontMono-Regular.ttf LICENSE -d "$work_dir"

python3 -m venv "$work_dir/venv"
"$work_dir/venv/bin/pip" install --quiet "fonttools[woff]==4.59.0" "brotli==1.1.0"
"$work_dir/venv/bin/pyftsubset" "$work_dir/SymbolsNerdFontMono-Regular.ttf" \
  --output-file="$project_root/public/fonts/symbols-nerd-font-mono-subset.woff2" \
  --flavor=woff2 \
  --unicodes="U+E0A0-E0D7,U+EA60-EC1E" \
  --layout-features='*' \
  --no-hinting
cp "$work_dir/LICENSE" "$project_root/public/fonts/NERD-FONTS-LICENSE"
