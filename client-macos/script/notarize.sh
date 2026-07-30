#!/usr/bin/env bash
# Notarize + staple a signed .app or .dmg via notarytool, then assess Gatekeeper.
#
# Prereq: a stored keychain profile (once):
#   xcrun notarytool store-credentials mkpk-notary \
#     --apple-id "<apple-id>" --team-id R2M77TY8U9 --password <app-specific-password>
#
# Usage:
#   script/notarize.sh path/to/mkpk.app          # zips, submits, staples the .app
#   script/notarize.sh path/to/mkpk_x.y.z.dmg    # submits, staples the .dmg
#   script/notarize.sh <file> <keychain-profile> # default profile: mkpk-notary
set -euo pipefail

FILE="${1:?usage: notarize.sh <file.app|file.dmg> [keychain-profile]}"
PROFILE="${2:-mkpk-notary}"
[[ -e "$FILE" ]] || { echo "✗ not found: $FILE"; exit 1; }

submit() {  # <path-to-submit>
  echo "▸ Submitting $1 to the notary service (profile: $PROFILE)"
  xcrun notarytool submit "$1" --keychain-profile "$PROFILE" --wait
}

case "$FILE" in
  *.app)
    # notarytool wants an archive, not a raw .app — ditto to a zip and submit that,
    # but staple the .app itself (the ticket attaches to the bundle).
    ZIP="$(mktemp -d)/$(basename "$FILE").zip"
    /usr/bin/ditto -c -k --keepParent "$FILE" "$ZIP"
    submit "$ZIP"
    rm -f "$ZIP"
    echo "▸ Stapling the app bundle"
    xcrun stapler staple "$FILE"
    xcrun stapler validate "$FILE"
    echo "▸ Gatekeeper assessment"
    spctl -a -vv "$FILE" 2>&1 || true
    ;;
  *.dmg)
    submit "$FILE"
    echo "▸ Stapling the dmg"
    xcrun stapler staple "$FILE"
    xcrun stapler validate "$FILE"
    echo "▸ Gatekeeper assessment (install context)"
    spctl -a -vv -t install "$FILE" 2>&1 || true
    ;;
  *)
    echo "✗ unsupported file type: $FILE (expected .app or .dmg)"; exit 1 ;;
esac

echo "✓ Notarized + stapled: $FILE"
