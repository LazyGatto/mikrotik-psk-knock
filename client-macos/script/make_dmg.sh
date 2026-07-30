#!/usr/bin/env bash
# Pack a macOS .app into a compressed, drag-to-Applications DMG (no external deps).
# Usage: make_dmg.sh <path/to/App.app> <output.dmg> [volume-name]
set -euo pipefail

APP="${1:?usage: make_dmg.sh App.app out.dmg [volname]}"
OUT="${2:?usage: make_dmg.sh App.app out.dmg [volname]}"
VOL="${3:-mkpk}"

[ -d "$APP" ] || { echo "make_dmg: no such .app: $APP" >&2; exit 1; }

STAGE="$(mktemp -d)"
trap 'rm -rf "$STAGE"' EXIT
cp -R "$APP" "$STAGE/"
ln -s /Applications "$STAGE/Applications"   # drag-to-install target

rm -f "$OUT"
hdiutil create -volname "$VOL" -srcfolder "$STAGE" -ov -format UDZO "$OUT" >/dev/null

# Optionally sign the DMG (gold standard: signed + notarized + stapled). Set
# MKPK_SIGN_ID to a "Developer ID Application: …" identity to enable; then the DMG
# still needs notarize.sh. Left unset → an unsigned DMG (fine for local/dev use).
if [ -n "${MKPK_SIGN_ID:-}" ] && [ "${MKPK_SIGN_ID}" != "-" ]; then
  codesign --force --sign "$MKPK_SIGN_ID" --timestamp "$OUT"
  echo "✓ $OUT  (signed: $MKPK_SIGN_ID — run notarize.sh next)"
else
  echo "✓ $OUT"
fi
