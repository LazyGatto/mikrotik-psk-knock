# script/ — build & release toolchain

Scripts for building, signing, notarizing and packaging the menu-bar app. The
patterns here are adapted (our own scripts) from the MIT-licensed
[OpenUsage](https://github.com/robinebers/openusage) build tooling — attribution
kept per its license.

Most of these land together with the **app target** (which arrives with the UI
mockups) and require a **Developer ID** certificate + notarytool credentials.
They are enumerated now so the structure is ready.

Planned:

- `build_app.sh` — `swift build -c release --arch arm64 --arch x86_64`, then
  assemble `mkpk.app/Contents/{MacOS,Info.plist,Resources}` (LSUIElement=true).
- `sign.sh` — codesign with hardened runtime + `release.entitlements`
  (see `macos-signing-notarization` skill).
- `notarize.sh` — `xcrun notarytool submit … --wait` then `stapler staple`.
- `make_dmg.sh` — drag-to-Applications DMG, signed + notarized + stapled.
- `embed_sparkle.sh` — bundle Sparkle + generate/sign the appcast entry.
- `render_icloud_entitlements.sh` — inject the team prefix into the keychain
  access group at build time (the prefix isn't known until signing).

Entitlements templates live here: `release.entitlements` (keychain-access-group
for iCloud Keychain sync).
