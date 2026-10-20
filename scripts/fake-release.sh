#!/bin/sh
# Prepare both download layouts; serve the printed root separately on loopback.
set -eu
if [ "$#" -ne 3 ]; then
    echo 'Usage: scripts/fake-release.sh <dist> <root> <tag>' >&2
    exit 1
fi
dist=$1
root=$2
tag=$3
case "$tag" in v[0-9]*.[0-9]*.[0-9]*) ;;
    *) echo 'Expected a release tag such as v0.1.0' >&2; exit 1 ;;
esac
case "$tag" in */*) echo 'Release tag must not contain slashes' >&2; exit 1 ;; esac
# Validate inputs before touching either layout.
assets='tollglass-linux-amd64 tollglass-linux-arm64 tollglass-darwin-amd64 tollglass-darwin-arm64 tollglass-windows-amd64.exe tollglass-windows-arm64.exe checksums.txt'
for asset in $assets; do
    [ -f "$dist/$asset" ] || { echo "Missing release file: $asset" >&2; exit 1; }
done
for layout in "$root/releases/latest/download" "$root/releases/download/$tag"; do
    mkdir -p "$layout"
    for asset in $assets; do cp "$dist/$asset" "$layout/$asset"; done
done
(cd "$root" && pwd -P)
