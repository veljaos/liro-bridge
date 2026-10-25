#!/bin/sh
# F13 §6: the macOS application bundle, built on a Mac (CI's macOS
# runner). Not signed and not notarised — see F13 §6 for what that costs
# a person (Gatekeeper's first-launch refusal) and what it needs (an
# Apple Developer ID, the owner's).
#
#   build/macos/build.sh <version> [outdir]
#
# Produces <outdir>/Liro Bridge.app with a universal (arm64 + x86_64)
# binary, and <outdir>/liro-bridge-<version>-macos.zip of it, made with
# ditto so the bundle's attributes survive.
set -eu
version=${1:?usage: build.sh <version> [outdir]}
out=${2:-dist/macos}
root=$(cd "$(dirname "$0")/../.." && pwd)
mkdir -p "$out"
out=$(cd "$out" && pwd)
app="$out/Liro Bridge.app"

rm -rf "$app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources"

commit=$(git -C "$root" rev-parse --short HEAD 2>/dev/null || echo unknown)
build_date=$(date -u +%Y-%m-%dT%H:%M:%SZ)
ldflags="-s -w -X main.version=$version -X main.commit=$commit -X main.buildDate=$build_date"
for arch in arm64 amd64; do
	clangarch=$arch
	[ "$arch" = amd64 ] && clangarch=x86_64
	CGO_ENABLED=1 GOOS=darwin GOARCH=$arch CC="clang -arch $clangarch" \
		go build -trimpath -ldflags "$ldflags" -o "$out/liro-bridge-$arch" "$root/cmd/liro-bridge"
done
lipo -create -output "$app/Contents/MacOS/liro-bridge" "$out/liro-bridge-arm64" "$out/liro-bridge-amd64"
rm -f "$out/liro-bridge-arm64" "$out/liro-bridge-amd64"

sed "s/@VERSION@/$version/g" "$root/build/macos/Info.plist.in" > "$app/Contents/Info.plist"
plutil -lint "$app/Contents/Info.plist"

# The icon: the .ico's own frames (scripts/genlinuxicons copies them
# out byte for byte), made into an .icns by iconutil. The .ico has no
# frame above 256, so 128 is scaled down from 256 and 512 and 1024 are
# scaled up from it by sips — soft at Retina sizes in Finder until
# scripts/genicon draws them (F13 §6).
frames="$out/frames"
rm -rf "$frames"
(cd "$root" && go run ./scripts/genlinuxicons --out "$frames" >/dev/null)
iconset="$out/liro-bridge.iconset"
rm -rf "$iconset"
mkdir -p "$iconset"
src256="$frames/256x256/apps/liro-bridge.png"
cp "$frames/16x16/apps/liro-bridge.png" "$iconset/icon_16x16.png"
cp "$frames/32x32/apps/liro-bridge.png" "$iconset/icon_16x16@2x.png"
cp "$frames/32x32/apps/liro-bridge.png" "$iconset/icon_32x32.png"
cp "$frames/64x64/apps/liro-bridge.png" "$iconset/icon_32x32@2x.png"
sips -z 128 128 "$src256" --out "$iconset/icon_128x128.png" >/dev/null
cp "$src256" "$iconset/icon_128x128@2x.png"
cp "$src256" "$iconset/icon_256x256.png"
sips -z 512 512 "$src256" --out "$iconset/icon_256x256@2x.png" >/dev/null
sips -z 512 512 "$src256" --out "$iconset/icon_512x512.png" >/dev/null
sips -z 1024 1024 "$src256" --out "$iconset/icon_512x512@2x.png" >/dev/null
iconutil -c icns "$iconset" -o "$app/Contents/Resources/liro-bridge.icns"
rm -rf "$iconset" "$frames"

(cd "$out" && rm -f "liro-bridge-$version-macos.zip" && ditto -c -k --keepParent "Liro Bridge.app" "liro-bridge-$version-macos.zip")
echo "built $app and $out/liro-bridge-$version-macos.zip"
