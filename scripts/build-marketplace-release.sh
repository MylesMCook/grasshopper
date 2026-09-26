#!/bin/sh
set -eu

if [ "$#" -ne 1 ]; then
	echo "usage: build-marketplace-release.sh OUTPUT_DIRECTORY" >&2
	exit 2
fi

root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$1"
output=$(CDPATH='' cd -- "$1" && pwd)
version=$(cat "$root/VERSION")
archive="$output/grasshopper-marketplace-$version.zip"
if [ -e "$archive" ]; then
	echo "archive already exists: $archive" >&2
	exit 1
fi
staged=$(mktemp -d "$output/.grasshopper-marketplace.XXXXXX")
trap 'rm -rf "$staged"' EXIT HUP INT TERM

cd "$root"
for target in darwin-arm64 windows-amd64 linux-amd64; do
	case "$target" in
		darwin-arm64) os=darwin; arch=arm64; suffix= ;;
		windows-amd64) os=windows; arch=amd64; suffix=.exe ;;
		linux-amd64) os=linux; arch=amd64; suffix= ;;
	esac
	GOOS="$os" GOARCH="$arch" CGO_ENABLED=0 go build -trimpath -buildvcs=false \
		-ldflags="-s -w -X main.clientVersion=$version" \
		-o "$staged/grasshopper-$target$suffix" ./cmd/grasshopper
done

go run ./cmd/grasshopper-go-bundle \
	-marketplace-plugins -plugin-version "$version" \
	-client-macos "$staged/grasshopper-darwin-arm64" \
	-client-windows "$staged/grasshopper-windows-amd64.exe" \
	-client-linux "$staged/grasshopper-linux-amd64" \
	-output "$archive"
printf '%s\n' "$archive"
