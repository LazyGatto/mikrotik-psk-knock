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

# Optional dedicated keychain (CI on mac-ci-01): the notary profile lives there
# rather than in the login keychain, which is locked for headless jobs.
KEYCHAIN_ARGS=()
if [[ -n "${MKPK_KEYCHAIN:-}" ]]; then
  KEYCHAIN_ARGS=(--keychain "$MKPK_KEYCHAIN")
fi

# Apple's notary endpoint times out on us now and then (NSURLError -1001 while
# polling a submission that is in fact progressing). It has cost two releases so
# far, each fixed by pressing retry by hand — so retry here instead. A resubmit
# is safe: notarization has no side effect on the artifact, and stapling happens
# only after a submission actually succeeds.
submit() {  # <path-to-submit>
  local attempt
  for attempt in 1 2 3; do
    echo "▸ Submitting $1 to the notary service (profile: $PROFILE, attempt $attempt/3)"
    if xcrun notarytool submit "$1" --keychain-profile "$PROFILE" \
        ${KEYCHAIN_ARGS[@]+"${KEYCHAIN_ARGS[@]}"} --wait; then
      return 0
    fi
    if [[ "$attempt" -lt 3 ]]; then
      echo "  ! notarization attempt $attempt failed — retrying in 30s"
      sleep 30
    fi
  done
  echo "✗ notarization failed three times — see the log above" >&2
  return 1
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
