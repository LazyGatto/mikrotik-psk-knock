# SKILL: sign, notarize, distribute (menu-bar app, non-App-Store)

Downloaded unsigned/un-notarized apps hit Gatekeeper. For distribution outside
the App Store you need a **Developer ID Application** certificate (paid Apple
Developer account) to sign + notarize. Prerequisites: `Developer ID Application`
cert in the login keychain, a Team ID, and an app-specific password (or an API
key) for notarytool.

## Sign

Sign inside-out (frameworks/helpers first, then the .app), with hardened runtime
and the app's entitlements:

```sh
codesign --force --options runtime --timestamp \
  --entitlements script/release.entitlements \
  --sign "Developer ID Application: NAME (TEAMID)" \
  MyApp.app
codesign --verify --deep --strict --verbose=2 MyApp.app
```

Hardened runtime (`--options runtime`) is required for notarization.

## Notarize + staple

Notarize the zipped app (or the DMG), then staple the ticket so it works offline:

```sh
ditto -c -k --keepParent MyApp.app MyApp.zip
xcrun notarytool submit MyApp.zip \
  --apple-id "$APPLE_ID" --team-id "$TEAM_ID" --password "$APP_PW" --wait
xcrun stapler staple MyApp.app
```

## DMG

`create-dmg` (or `hdiutil`) to make a drag-to-Applications DMG; sign + notarize +
staple the DMG too. Ship the stapled DMG.

## Without a cert (interim)

Until there's a Developer ID, unsigned builds run locally after clearing the
quarantine xattr (`xattr -d com.apple.quarantine MyApp.app`) or a right-click →
Open. Document this for testers; it is NOT a distribution path.

## Auto-updates

Sparkle (EdDSA-signed appcast) is the standard for menu-bar apps: embed the
Sparkle framework, host an `appcast.xml` + signed `.zip`/`.dmg`, and the app
updates itself. Sign the update with the Sparkle EdDSA key (separate from the
codesign identity).
