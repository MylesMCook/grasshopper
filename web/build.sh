#!/bin/sh
set -eu

root=$(CDPATH= cd -- "$(dirname -- "$0")/.." && pwd)
output="$root/web/dist"
rm -rf -- "$output"
mkdir -p "$output/fonts" "$output/view"
cp "$root/web/public/index.html" "$root/web/public/connect.js" "$root/web/public/_headers" "$root/web/public/404.html" "$output/"
cp "$root/web/public/view/index.html" "$output/view/"
cp "$root/docs/site.css" "$root/docs/theme.js" "$output/"
cp "$root/docs/fonts/newsreader-latin.woff2" "$root/docs/fonts/geist-mono-latin.woff2" "$output/fonts/"
