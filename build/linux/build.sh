#!/bin/sh
# build/linux/build.sh <version> <out-dir>
#
# Builds the Linux agent and packages it as a .deb and an .rpm from
# build/linux/nfpm.yaml (F12 §8). Runs from anywhere; paths are taken
# from the repository root.
#
# **Where this runs is decided by what it produces, and it checks that
# rather than trusting where it ran.** A cgo binary takes a symbol
# version from whichever glibc it was linked against, and one linked on a
# newer distribution than the oldest supported will not start there.
# The oldest supported is Ubuntu 24.04 (F12 §3.1), glibc 2.39; the
# release job runs this inside an ubuntu:24.04 container for that
# reason, and the last step below refuses a binary that needs anything
# newer — so running it somewhere else fails here rather than on a
# person's machine.
set -eu

if [ $# -ne 2 ]; then
    echo "usage: $0 <version> <out-dir>" >&2
    exit 2
fi
version=$1
out=$2

root=$(cd "$(dirname "$0")/../.." && pwd)
cd "$root"
case "$out" in /*) ;; *) out="$root/$out" ;; esac

nfpm=${NFPM:-nfpm}
stage=dist/linux/stage
rm -rf "$stage"
mkdir -p "$stage" "$out"

commit=$(git rev-parse --short HEAD 2>/dev/null || echo none)
build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)

echo "building liro-bridge ($version, $commit)"
CGO_ENABLED=1 GOOS=linux GOARCH=amd64 go build -trimpath \
    -ldflags "-s -w -X main.version=$version -X main.commit=$commit -X main.buildDate=$build_date" \
    -o "$stage/liro-bridge" ./cmd/liro-bridge

# The highest GLIBC_ symbol version the binary requires. sort -V puts
# 2.9 before 2.10, which a plain sort would not.
glibc_floor=2.39
needed=$(objdump -T "$stage/liro-bridge" | grep -o 'GLIBC_[0-9][0-9.]*' | sed 's/GLIBC_//' | sort -uV | tail -1)
echo "the binary needs glibc $needed; the oldest supported distribution has $glibc_floor"
if [ "$(printf '%s\n%s\n' "$needed" "$glibc_floor" | sort -V | tail -1)" != "$glibc_floor" ]; then
    echo "error: this binary needs glibc $needed, newer than $glibc_floor (Ubuntu 24.04); build it in the ubuntu:24.04 container" >&2
    exit 1
fi

go run ./scripts/genlinuxicons --out "$stage/icons" >/dev/null

deb="$out/liro-bridge_${version}_amd64.deb"
rpm="$out/liro-bridge-${version}.x86_64.rpm"
LIRO_VERSION=$version "$nfpm" package --config build/linux/nfpm.yaml --packager deb --target "$deb"
LIRO_VERSION=$version "$nfpm" package --config build/linux/nfpm.yaml --packager rpm --target "$rpm"
