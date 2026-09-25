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

# Change the HTML asset hashes when _headers changes; otherwise Wrangler can
# reuse the previous asset manifest and leave the old headers at the edge.
policy_revision=$(cksum < "$output/_headers")
for page in "$output/index.html" "$output/view/index.html" "$output/404.html"; do
  printf '\n<!-- Policy revision: %s -->\n' "$policy_revision" >> "$page"
done
