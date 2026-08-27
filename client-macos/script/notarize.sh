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

# `notarytool submit --wait` keeps one long request open while Apple processes
# the upload, and that request is what breaks: three releases have now died with
# NSURLError -1001 ("request timed out") *after* "Successfully uploaded file",
# while the endpoint answered a plain curl from the same machine in 0.4s and
# Apple's status page reported the notary service healthy.
#
# So: upload once (that part has never failed), then poll with short, separate
# `notarytool info` calls. A poll that times out is retried instead of failing
# the release, and a submission Apple has already accepted is never re-uploaded.
notary() { xcrun notarytool "$@" --keychain-profile "$PROFILE" ${KEYCHAIN_ARGS[@]+"${KEYCHAIN_ARGS[@]}"}; }

# jsonField <field> — pulls one value out of notarytool's JSON without needing a
# parser; the output is flat and the values are quoted strings.
jsonField() { sed -n 's/.*"'"$1"'":[[:space:]]*"\([^"]*\)".*/\1/p' | head -1; }

submit() {  # <path-to-submit>
  local out id status
  echo "▸ Submitting $1 to the notary service (profile: $PROFILE)"
  out="$(notary submit "$1" --no-wait --output-format json)" || {
    echo "✗ upload to the notary service failed" >&2
    echo "$out" >&2
    return 1
  }
  id="$(printf '%s' "$out" | jsonField id)"
  [[ -n "$id" ]] || { echo "✗ no submission id in: $out" >&2; return 1; }
  echo "  submission id: $id"

  # ~20 minutes of patience: notarization normally takes a couple of minutes,
  # and a stuck poll costs one iteration rather than the whole release.
  local i=0 fails=0
  while [[ "$i" -lt 80 ]]; do
    i=$((i + 1))
    sleep 15
    if ! out="$(notary info "$id" --output-format json 2>&1)"; then
      fails=$((fails + 1))
      echo "  … poll $i failed (${fails} in a row) — retrying"
      [[ "$fails" -lt 10 ]] || { echo "✗ ten polls in a row failed" >&2; return 1; }
      continue
    fi
    fails=0
    status="$(printf '%s' "$out" | jsonField status)"
    case "$status" in
      Accepted)
        echo "  ✓ Accepted after $((i * 15))s"
        return 0
        ;;
      "In Progress"|"") ;;   # keep waiting
      *)
        echo "✗ notarization $status — log follows" >&2
        notary log "$id" >&2 || true
        return 1
        ;;
    esac
  done
  echo "✗ notarization did not finish in 20 minutes (submission $id)" >&2
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
