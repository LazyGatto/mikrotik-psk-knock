#!/usr/bin/env bash
# Assemble MkpkApp.app from the SwiftPM build — an agent (LSUIElement) menu-bar
# app bundle with Info.plist, the SwiftPM resource bundle (Bundle.module) and an
# AppIcon. This is the dev/unsigned bundle; sign.sh + notarize.sh come later.
#
# Usage:
#   script/build_app.sh                 # release, native arch
#   MKPK_UNIVERSAL=1 script/build_app.sh # universal (arm64 + x86_64)
#   MKPK_VERSION=1.2.3 MKPK_BUNDLE_ID=… script/build_app.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"      # client-macos/
cd "$ROOT"

APP_NAME="mkpk"                     # display name
EXE="MkpkApp"                       # executable / target name
RES_BUNDLE="mkpk-client_MkpkApp.bundle"
BUNDLE_ID="${MKPK_BUNDLE_ID:-dev.mkpk.client}"   # placeholder — set real id before signing
VERSION="${MKPK_VERSION:-0.0.0-dev}"             # never a release version without owner sign-off
BUILD_NUM="$(git rev-list --count HEAD 2>/dev/null || echo 1)"

ARCH_FLAGS=(--arch arm64)
if [[ "${MKPK_UNIVERSAL:-0}" == "1" ]]; then
  ARCH_FLAGS=(--arch arm64 --arch x86_64)
fi

echo "▸ Building $EXE (release) ${ARCH_FLAGS[*]}"
swift build -c release --product "$EXE" "${ARCH_FLAGS[@]}"
BIN_DIR="$(swift build -c release --product "$EXE" "${ARCH_FLAGS[@]}" --show-bin-path)"

APP="$ROOT/.build/$APP_NAME.app"
CONTENTS="$APP/Contents"
echo "▸ Assembling $APP"
rm -rf "$APP"
mkdir -p "$CONTENTS/MacOS" "$CONTENTS/Resources"

cp "$BIN_DIR/$EXE" "$CONTENTS/MacOS/$EXE"

# SwiftPM resource bundle (icons via Bundle.module).
if [[ -d "$BIN_DIR/$RES_BUNDLE" ]]; then
  cp -R "$BIN_DIR/$RES_BUNDLE" "$CONTENTS/Resources/"
else
  echo "⚠︎ resource bundle $RES_BUNDLE not found in $BIN_DIR — icons will fall back to SF Symbol"
fi

# App icon: build an .icns from the 256px brand tile (best-effort).
ICON_SRC="$ROOT/Sources/MkpkApp/Resources/icon-dark.png"
if command -v iconutil >/dev/null && command -v sips >/dev/null && [[ -f "$ICON_SRC" ]]; then
  ICONSET="$(mktemp -d)/AppIcon.iconset"
  mkdir -p "$ICONSET"
  for spec in "16:16x16" "32:16x16@2x" "32:32x32" "64:32x32@2x" "128:128x128" "256:128x128@2x" "256:256x256" "512:256x256@2x" "512:512x512"; do
    px="${spec%%:*}"; name="${spec##*:}"
    sips -z "$px" "$px" "$ICON_SRC" --out "$ICONSET/icon_${name}.png" >/dev/null 2>&1 || true
  done
  if iconutil -c icns "$ICONSET" -o "$CONTENTS/Resources/AppIcon.icns" 2>/dev/null; then
    ICON_KEY='<key>CFBundleIconFile</key><string>AppIcon</string>'
  else
    echo "⚠︎ iconutil failed — bundling without an app icon"; ICON_KEY=''
  fi
else
  ICON_KEY=''
fi

cat > "$CONTENTS/Info.plist" <<PLIST
<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
  <key>CFBundleName</key><string>$APP_NAME</string>
  <key>CFBundleDisplayName</key><string>$APP_NAME</string>
  <key>CFBundleIdentifier</key><string>$BUNDLE_ID</string>
  <key>CFBundleExecutable</key><string>$EXE</string>
  <key>CFBundlePackageType</key><string>APPL</string>
  <key>CFBundleShortVersionString</key><string>$VERSION</string>
  <key>CFBundleVersion</key><string>$BUILD_NUM</string>
  <key>LSMinimumSystemVersion</key><string>13.0</string>
  <key>LSUIElement</key><true/>
  <key>NSHighResolutionCapable</key><true/>
  $ICON_KEY
  <key>NSHumanReadableCopyright</key><string>mkpk · Knock first</string>
</dict>
</plist>
PLIST

printf 'APPL????' > "$CONTENTS/PkgInfo"

echo "✓ Built $APP  (id=$BUNDLE_ID version=$VERSION build=$BUILD_NUM)"
echo "  open \"$APP\"   # launch the bundled app"
