#!/usr/bin/env bash
set -euo pipefail
[[ "$(uname -s)" == Darwin ]] || { echo "DMG requires macOS" >&2; exit 1; }
root="$(cd "$(dirname "$0")/.." && pwd)"
cd "$root"
output="${1:-dist/release-macos}"
version="${2:-$(tr -d '[:space:]' < VERSION)}"
force="${3:-false}"
go_bin="${GO_BIN:-go}"
[[ "$version" =~ ^[0-9]+\.[0-9]+\.[0-9]+$ ]] || { echo "Invalid version" >&2; exit 1; }
mkdir -p "$output"
output="$(cd "$output" && pwd)"
asset="$output/ProxyStart-macos-universal.dmg"
if [[ -e "$asset" && "$force" != true ]]; then echo "DMG exists; use Force" >&2; exit 1; fi
# A dedicated output directory prevents accidentally uploading build debris.
python3 - "$output" <<'PY'
from pathlib import Path
import sys
unexpected=[p.name for p in Path(sys.argv[1]).iterdir() if p.name!="ProxyStart-macos-universal.dmg"]
if unexpected: raise SystemExit(f"Unexpected release files: {unexpected}")
PY
if [[ "${SKIP_TESTS:-0}" != 1 ]]; then "$go_bin" test ./...; "$go_bin" vet ./...; fi
work="$(mktemp -d "${TMPDIR:-/tmp}/proxy-start-dmg.XXXXXX")"
mounted=false
cleanup() {
    if [[ "$mounted" == true ]]; then hdiutil detach "$work/mount" -quiet || true; fi
    if [[ "$work" == "${TMPDIR:-/tmp}"/proxy-start-dmg.* ]]; then rm -rf "$work"; fi
}
trap cleanup EXIT
app="$work/stage/Proxy Start.app"
mkdir -p "$app/Contents/MacOS" "$app/Contents/Resources" "$work/icons.iconset" "$work/mount"
for arch in arm64 amd64; do
    GOOS=darwin GOARCH="$arch" CGO_ENABLED=0 "$go_bin" build -trimpath -tags mygo_noinspector -ldflags="-s -w" -o "$work/proxy-start-$arch" ./cmd/proxy-start
done
lipo -create "$work/proxy-start-arm64" "$work/proxy-start-amd64" -output "$app/Contents/MacOS/proxy-start"
lipo "$app/Contents/MacOS/proxy-start" -verify_arch arm64 x86_64
chmod 755 "$app/Contents/MacOS/proxy-start"
for size in 16 32 128 256 512; do
    cp "assets/proxy-start-$size.png" "$work/icons.iconset/icon_${size}x${size}.png"
    double=$((size * 2))
    cp "assets/proxy-start-$double.png" "$work/icons.iconset/icon_${size}x${size}@2x.png"
done
iconutil -c icns "$work/icons.iconset" -o "$app/Contents/Resources/proxy-start.icns"
python3 - "$app" "$version" <<'PY'
from pathlib import Path
import plistlib, sys
contents=Path(sys.argv[1])/"Contents"
data=dict(CFBundleDisplayName="Proxy Start",CFBundleName="Proxy Start",CFBundleIdentifier="com.proxystart.desktop",
          CFBundleExecutable="proxy-start",CFBundlePackageType="APPL",CFBundleInfoDictionaryVersion="6.0",
          CFBundleVersion=sys.argv[2],CFBundleShortVersionString=sys.argv[2],CFBundleIconFile="proxy-start.icns",
          LSMinimumSystemVersion="12.0",NSHighResolutionCapable=True)
(contents/"Info.plist").write_bytes(plistlib.dumps(data))
(contents/"PkgInfo").write_bytes(b"APPL????")
PY
# Local ad-hoc signature supports Apple Silicon; no Developer ID/notarization is claimed.
codesign --force --sign - "$app"
codesign --verify --deep --strict "$app"
ln -s /Applications "$work/stage/Applications"
hdiutil create -quiet -volname "Proxy Start" -srcfolder "$work/stage" -fs HFS+ -format UDZO "$work/ProxyStart.dmg"
hdiutil verify "$work/ProxyStart.dmg" >/dev/null
hdiutil attach -readonly -nobrowse -mountpoint "$work/mount" "$work/ProxyStart.dmg" -quiet
mounted=true
python3 - "$work/mount" "$version" <<'PY'
from pathlib import Path
import os, plistlib, stat, subprocess, sys
mount=Path(sys.argv[1]); app=mount/"Proxy Start.app"; contents=app/"Contents"
assert set(os.listdir(mount))=={"Proxy Start.app","Applications"},os.listdir(mount)
assert os.readlink(mount/"Applications")=="/Applications"
files={str(p.relative_to(app)) for p in app.rglob("*") if p.is_file()}
assert files=={"Contents/Info.plist","Contents/PkgInfo","Contents/MacOS/proxy-start","Contents/Resources/proxy-start.icns","Contents/_CodeSignature/CodeResources"},files
binary=contents/"MacOS/proxy-start"
assert binary.stat().st_mode & stat.S_IXUSR
assert plistlib.loads((contents/"Info.plist").read_bytes())["CFBundleShortVersionString"]==sys.argv[2]
subprocess.run(["lipo",str(binary),"-verify_arch","arm64","x86_64"],check=True)
subprocess.run(["codesign","--verify","--deep","--strict",str(app)],check=True)
PY
"$work/mount/Proxy Start.app/Contents/MacOS/proxy-start" --check --config "$work/settings.json" >"$work/check.json"
python3 - "$work/check.json" "$version" <<'PY'
import json, sys
data=json.load(open(sys.argv[1]))
assert data["Version"]==sys.argv[2], data
assert data["ProxyPort"]==7897, data
PY
hdiutil detach "$work/mount" -quiet
mounted=false
cp "$work/ProxyStart.dmg" "$asset"
echo "Verified universal DMG: $asset"
shasum -a 256 "$asset"
