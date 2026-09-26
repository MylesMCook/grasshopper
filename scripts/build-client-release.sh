#!/bin/sh
set -eu

if [ "$#" -ne 2 ]; then
	echo "usage: build-client-release.sh VERSION OUTPUT_DIRECTORY" >&2
	exit 2
fi

version=$1
expected=$(cat "$(dirname "$0")/../VERSION")
if [ "$version" != "$expected" ]; then
	echo "release version is $expected, not $version" >&2
	exit 2
fi
output=$2
root=$(CDPATH='' cd -- "$(dirname -- "$0")/.." && pwd)
mkdir -p "$output"
output=$(CDPATH='' cd -- "$output" && pwd)

cd "$root"
for target in darwin-arm64 windows-amd64 linux-amd64; do
	archive="$output/grasshopper-client-$target-$version.zip"
	if [ -e "$archive" ]; then
		echo "archive already exists: $archive" >&2
		exit 1
	fi
done

for target in darwin-arm64 windows-amd64 linux-amd64; do
	case "$target" in
		darwin-arm64) os=darwin; arch=arm64; suffix= ;;
		windows-amd64) os=windows; arch=amd64; suffix=.exe ;;
		linux-amd64) os=linux; arch=amd64; suffix= ;;
	esac
	archive="$output/grasshopper-client-$target-$version.zip"
	binary="$output/grasshopper-$target$suffix"
	GOOS=$os GOARCH=$arch CGO_ENABLED=0 go build -trimpath -buildvcs=false -ldflags="-X main.clientVersion=$version" -o "$binary" ./cmd/grasshopper
	go run ./cmd/grasshopper-go-bundle -client "$binary" -client-plugins -target "$target" -plugin-version "$version" -output "$archive"
	printf '%s\n' "$archive"
done
