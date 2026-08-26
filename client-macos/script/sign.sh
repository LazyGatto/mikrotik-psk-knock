#!/usr/bin/env bash
# Deep-sign the assembled mkpk.app with a Developer ID Application identity +
# hardened runtime + entitlements — ready for notarize.sh.
#
# Prereq: build_app.sh produced .build/mkpk.app and the Developer ID Application
# cert is in the login keychain.
#
# Usage:
#   script/sign.sh [path/to/App.app]
#   MKPK_SIGN_ID="Developer ID Application: … (TEAMID)" MKPK_TEAM_ID=TEAMID script/sign.sh
set -euo pipefail

HERE="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$HERE/.." && pwd)"      # client-macos/

APP="${1:-$ROOT/.build/mkpk.app}"
[[ -d "$APP" ]] || { echo "✗ app not found: $APP  (run build_app.sh first)"; exit 1; }

# Developer ID Application identity + team. Override via env for a different org.
SIGN_ID="${MKPK_SIGN_ID:-Developer ID Application: EDINY GOROD, OOO (R2M77TY8U9)}"
TEAM_ID="${MKPK_TEAM_ID:-R2M77TY8U9}"

# Optional dedicated keychain (CI on mac-ci-01): pass its path so codesign does
# not fall through to the login keychain, which re-locks with the screen and
# hangs a headless job on an invisible password prompt.
KEYCHAIN_ARGS=()
if [[ -n "${MKPK_KEYCHAIN:-}" ]]; then
  KEYCHAIN_ARGS=(--keychain "$MKPK_KEYCHAIN")
fi

# The keychain-access-groups entitlement (for iCloud Keychain sync) is only honored
# on macOS when the app embeds a matching provisioning profile — otherwise amfi
# refuses to launch it (error 163). So the entitlement is applied ONLY when a
# Developer ID provisioning profile is provided; without one we sign entitlement-
# free (the app launches; the iCloud-sync toggle gracefully falls back).
#   MKPK_PROVISION_PROFILE=/path/to/mkpk.provisionprofile   # enables the keychain group
PROFILE="${MKPK_PROVISION_PROFILE:-$HERE/mkpk.provisionprofile}"
ENT_ARGS=()
if [[ -f "$PROFILE" ]]; then
  cp "$PROFILE" "$APP/Contents/embedded.provisionprofile"
  ENT_SRC="$HERE/release.entitlements"
  ENT="$(mktemp -t mkpk-ent).plist"
  trap 'rm -f "$ENT"' EXIT
  sed "s/__TEAM_PREFIX__/${TEAM_ID}./g" "$ENT_SRC" > "$ENT"
  ENT_ARGS=(--entitlements "$ENT")
  echo "▸ Provisioning profile: $PROFILE (keychain-access-groups enabled)"
else
  echo "▸ No provisioning profile — signing without keychain-access-groups (iCloud sync off)"
fi

echo "▸ Signing $APP"
echo "  identity: $SIGN_ID"
[[ -n "${MKPK_KEYCHAIN:-}" ]] && echo "  keychain: $MKPK_KEYCHAIN"

# 0) Sparkle helpers first (deepest inside-out level): Autoupdate, *.xpc and
#    the nested Updater.app inside Sparkle.framework. Without their own
#    Developer ID signatures + secure timestamps, notarization rejects the
#    bundle once the framework is embedded. Scoped to Contents/Frameworks.
if [[ -d "$APP/Contents/Frameworks" ]]; then
  while IFS= read -r -d '' item; do
    codesign --force --options runtime --timestamp ${KEYCHAIN_ARGS[@]+"${KEYCHAIN_ARGS[@]}"} --sign "$SIGN_ID" "$item"
  done < <(find "$APP/Contents/Frameworks" \( -name "*.xpc" -o -name "*.app" -o -name "Autoupdate" \) -print0)
fi

# 1) Sign nested bundles/dylibs first (inside-out) — any nested code must carry
#    its own signature before the outer seal. The SwiftPM resource bundle lives at
#    BOTH the .app root (Bundle.module lookup) and Contents/Resources; sign every
#    copy. Resource-only bundles get sealed too so --verify --deep --strict passes.
while IFS= read -r -d '' item; do
  codesign --force --options runtime --timestamp ${KEYCHAIN_ARGS[@]+"${KEYCHAIN_ARGS[@]}"} --sign "$SIGN_ID" "$item"
done < <(find "$APP" \( -name "*.bundle" -o -name "*.dylib" -o -name "*.framework" \) -print0)

# 2) Sign the executable + the outer app bundle with hardened runtime (+ entitlements
#    when a provisioning profile authorized them).
codesign --force --options runtime --timestamp \
  ${ENT_ARGS[@]+"${ENT_ARGS[@]}"} \
  ${KEYCHAIN_ARGS[@]+"${KEYCHAIN_ARGS[@]}"} \
  --sign "$SIGN_ID" "$APP"

echo "▸ Verifying"
codesign --verify --deep --strict --verbose=2 "$APP"
echo "  — entitlements —"
codesign --display --entitlements - "$APP" 2>/dev/null | sed -n '1,40p' || true
echo "✓ Signed.  Next: make_dmg.sh then notarize.sh <dmg>"
