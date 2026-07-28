# .agents/skills — vendored macOS/Swift know-how

Local, self-contained reference notes for building the native Swift menu-bar
client. They are **our own adaptations** written in our words: the upstream
inspiration ([robinebers/skills](https://github.com/robinebers/skills)) is
unlicensed, so nothing is copied — only the domain knowledge is reused. The
OpenUsage app itself (MIT) is a working reference for the same patterns.

Everything is vendored here on purpose: no runtime dependency on external repos
that could disappear.

When working on an area (SPM layout, the menu-bar shell, signing/notarization,
iCloud Keychain), read the matching `SKILL.md` first.

- `macos-swiftpm/` — package layout, targets, build/run without Xcode.
- `macos-menubar-app/` — NSStatusItem + NSPanel popover shell (not MenuBarExtra).
- `macos-signing-notarization/` — sign, notarize, staple, DMG.
- `macos-icloud-keychain/` — entitlements + syncable Keychain items.
