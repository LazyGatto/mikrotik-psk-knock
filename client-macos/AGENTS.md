# AGENTS.md — mkpk client (macOS)

The native **Swift 6 / SwiftUI** menu-bar client of mkpk, for invite recipients.
It imports `.mkpk` invites and knocks/checks services. This file is the source of
truth for agent conventions in `client-macos/`; `CLAUDE.md` only points here.

Conventions here are adapted from the excellent, AI-agent-built
[OpenUsage](https://github.com/robinebers/openusage) (MIT) — patterns and ideas,
written in our own words. See the design brief in [../docs/client-brief.md](../docs/client-brief.md).

## Architecture

- SwiftPM package (`Package.swift`), no Xcode project. Targets:
  - `MkpkKit` — the runtime core: invite decode, PSK time-token, staged UDP knock,
    TCP check. Reimplemented from the Go reference (`../client/internal/{token,invite,knock,servicecheck}`).
  - `mkpk-selfcheck` — a framework-free verification runner (see Testing).
  - (later) the menu-bar app target: SwiftUI hosted in an AppKit-owned
    `NSStatusItem` + a custom key-capable `NSPanel` popover (not `MenuBarExtra`).
- Swift 6 with strict concurrency (`.swiftLanguageMode(.v6)`).
- The app is a background, dockless menu-bar process; the bar icon reflects state.

## Protocol fidelity (critical)

The knock/token protocol MUST stay byte-identical to the Go client and RouterOS.
The token is `sha512(psk|v1|service|client_id|bucket|psk)` (lowercase hex);
`bucket = floor(unixSeconds / bucket_seconds)`.

- Any change touching the token, bucketing, or knock wire format requires
  regenerating and re-checking the **golden vectors** against the Go reference
  (`client/internal/token`). Never hand-edit an expected token.
- The Go `mkpk` remains the source of truth for the formula.

## Building / running / testing

- `swift build` — build.
- `swift run mkpk-selfcheck` — run the runtime self-check (golden token vectors,
  invite decode). This is the verification that works with **Command Line Tools
  only**.
- `swift test` (XCTest / Swift Testing) needs **full Xcode** — the CLT ships no
  usable test runtime (`lib_TestingInterop.dylib` is absent). Until Xcode is a
  given (e.g. a macOS CI runner), correctness lives in `mkpk-selfcheck`; add
  proper `swift test` suites when the toolchain allows.
- No hot reload: the app is a long-lived menu-bar process — every change needs a
  full rebuild and relaunch of the running app before testing.

## Code conventions

- Keep files under ~500 LOC; split/refactor as needed.
- No new dependencies without justification.
- **Fail loudly** (log + friendly user error); no silent fallbacks that hide real
  problems. Validate only at boundaries (imported invites, network), trust
  internal code.
- Add a regression check to `mkpk-selfcheck` when fixing a runtime bug.
- Do not reintroduce real infrastructure identifiers (hosts/PSKs) into tracked
  files — use placeholders (`router.example.com`, synthetic PSKs).

## Releases

- Never bump the version on your own initiative — propose it and wait for the
  owner's explicit sign-off before tagging.
- **Distribution** (`script/`): `build_app.sh` → `sign.sh` → `make_dmg.sh` →
  `notarize.sh`. `sign.sh` uses the **Developer ID Application** identity
  (`EDINY GOROD, OOO (R2M77TY8U9)`) + hardened runtime + `release.entitlements`
  (the `__TEAM_PREFIX__` keychain-group placeholder is filled with the team id at
  sign time). `notarize.sh` submits via the `mkpk-notary` keychain profile and
  staples — accepts a `.app` (zips it) or a `.dmg`. Bundle id `ru.eg23.mkpk.client`
  (its default keychain-access-group equals the entitlement, so no App-ID/portal
  registration is needed). Packaging note: the brand PNGs are **flattened into
  Contents/Resources** and loaded via `Bundle.main` — a resource bundle beside
  `Contents/` makes the app unsignable ("unsealed contents in the bundle root"),
  so `Bundle.module` is a `#if DEBUG` (`swift run`) fallback only.
- **iCloud Keychain sync** (the Settings toggle) is **off until provisioned**: the
  `keychain-access-groups` entitlement only launches on macOS when the `.app`
  embeds a matching **Developer ID provisioning profile** (without it amfi kills
  the app — error 163 "Launchd job spawn failed"). So `sign.sh` applies the
  entitlement only when `MKPK_PROVISION_PROFILE` (or `script/mkpk.provisionprofile`)
  is present; otherwise it signs entitlement-free and the toggle falls back to the
  file store. To enable: register App ID `ru.eg23.mkpk.client` + a Keychain-Sharing
  group, make a Developer ID provisioning profile, drop it at `script/mkpk.provisionprofile`,
  and set `kSecUseDataProtectionKeychain` on the synchronizable items. Tracked as a
  GitLab issue.
- Sparkle appcast: still later.

## Skills

Reusable macOS/Swift know-how (SwiftPM, AppKit↔SwiftUI interop, signing,
notarization, iCloud entitlements) is vendored under `.agents/skills/` as our own
adaptations (the upstream skill sources are unlicensed — reference only). Pull the
relevant one into context when working on that area.
