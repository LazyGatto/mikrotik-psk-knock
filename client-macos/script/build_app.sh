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
BUNDLE_ID="${MKPK_BUNDLE_ID:-ru.eg23.mkpk.client}"   # matches the keychain-access-group in release.entitlements
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

# Embed SPM-linked frameworks (Sparkle). Universal builds expose them at
# $BIN_DIR/Frameworks, single-arch builds directly in $BIN_DIR. Without this
# the app dies at launch: dyld cannot resolve @rpath/Sparkle.framework.
for FW_DIR in "$BIN_DIR/Frameworks" "$BIN_DIR"; do
  for FW in "$FW_DIR"/*.framework; do
    [[ -d "$FW" ]] || continue
    mkdir -p "$CONTENTS/Frameworks"
    cp -R "$FW" "$CONTENTS/Frameworks/"
  done
  [[ -d "$CONTENTS/Frameworks" ]] && break
done
if [[ -d "$CONTENTS/Frameworks" ]]; then
  install_name_tool -add_rpath "@executable_path/../Frameworks" "$CONTENTS/MacOS/$EXE"
  echo "✓ Embedded frameworks: $(ls "$CONTENTS/Frameworks" | tr '\n' ' ')"
fi

# Bundled brand icons. The generated Bundle.module accessor resolves the resource
# bundle at the .app ROOT, but content beside Contents/ makes the app unsignable
# ("unsealed contents present in the bundle root"). So the packaged app instead
# FLATTENS the PNGs straight into Contents/Resources and loads them via
# Bundle.main (see Brand.loadPNG); Bundle.module stays a dev-only (`swift run`)
# fallback. Source PNGs come from the built resource bundle, or the source tree.
ICON_DIR="$BIN_DIR/$RES_BUNDLE"
[[ -d "$ICON_DIR" ]] || ICON_DIR="$ROOT/Sources/MkpkApp/Resources"
copied=0
for png in icon-dark icon-light; do
  if [[ -f "$ICON_DIR/$png.png" ]]; then
    cp "$ICON_DIR/$png.png" "$CONTENTS/Resources/$png.png"; copied=$((copied+1))
  fi
done
[[ $copied -eq 2 ]] || echo "⚠︎ brand PNGs not fully found in $ICON_DIR — logo will fall back to SF Symbol"

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

# Sparkle update feed. Written ONLY when the EdDSA public key is provided
# (release/CI builds): dev bundles get no feed keys, so the in-app updater and
# the Settings button stay off. The feed lives on the public GitHub mirror —
# never point it at the GitLab host (public repo scrub policy).
SPARKLE_FEED_URL="${MKPK_SPARKLE_FEED_URL:-https://github.com/LazyGatto/mikrotik-psk-knock/releases/latest/download/appcast.xml}"
SPARKLE_KEYS=''
if [[ -n "${MKPK_SPARKLE_ED_PUBLIC_KEY:-}" ]]; then
  SPARKLE_KEYS="<key>SUFeedURL</key><string>$SPARKLE_FEED_URL</string>
  <key>SUPublicEDKey</key><string>$MKPK_SPARKLE_ED_PUBLIC_KEY</string>"
  echo "▸ Sparkle feed enabled: $SPARKLE_FEED_URL"
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
  $SPARKLE_KEYS
  <key>NSHumanReadableCopyright</key><string>mkpk · Knock first</string>
</dict>
</plist>
PLIST

printf 'APPL????' > "$CONTENTS/PkgInfo"

# Ad-hoc sign (free, no Apple account): gives a stable code identity so local
# notifications / Keychain accept the app. Real distribution/iCloud needs a
# Developer ID signature (sign.sh). Override the identity with MKPK_SIGN_ID.
SIGN_ID="${MKPK_SIGN_ID:--}"   # "-" = ad-hoc
if command -v codesign >/dev/null; then
  if codesign --force --sign "$SIGN_ID" --timestamp=none "$APP" >/dev/null 2>&1; then
    echo "✓ Signed ($([ "$SIGN_ID" = "-" ] && echo ad-hoc || echo "$SIGN_ID"))"
  else
    echo "⚠︎ codesign failed — notifications/Keychain may reject the app"
  fi
fi

echo "✓ Built $APP  (id=$BUNDLE_ID version=$VERSION build=$BUILD_NUM)"
echo "  open \"$APP\"   # launch the bundled app"
