# Changelog

Notable changes to this project. Format: [Keep a Changelog](https://keepachangelog.com).
Versions are the **Go CLI / provisioner** release tags (`mkpk`, `mkpk-provision`);
the native macOS recipient app in `client-macos/` ships separately.

## [Unreleased]

### Changed
- **macOS client fails loudly on a failed knock** (parity with the Windows
  client): a manual knock that ends closed / unreachable now shows a dismissible
  in-app banner with the attempt count and the underlying error, and the
  service log records the attempts; a successful open clears the banner.
  Background keep-open renewals stay quiet as before.

## [0.6.0] — 2026-08-26

### Added
- **Provision update check**: the admin UI (web and desktop) now checks the
  public GitHub releases feed (cached, 6h) and shows a footer badge + toast
  when a newer version exists; in the desktop app the link opens in the system
  browser. Dev builds never nag.
- **Native save dialog in mkpk-provision-desktop** (#28): exporting an invite
  now opens a proper "where to save" dialog instead of silently writing to
  `~/Downloads`; cancelling saves nothing. The browser (`serve`) flow is
  unchanged.
- **Human-readable Telegram notification** from the router on a knock: the
  rendered `mkpk-tt-notify` now sends `🔓 <router>: <service> open for
  <client_id>` + `from <ip> · <ttl>` instead of the `key=value` line (the
  webhook payload and email body stay machine-parseable). Re-deploy routers to
  pick it up. Confirmed live on a real router (issue #27): allow-before-notify
  order and the on-error guard behave as designed.
- **Windows GUI client `mkpk-client`** (#26): a small Wails app for invite
  recipients — import `.mkpk` invites (stored under `%APPDATA%\mkpk`), see
  routers/services, knock and check with an honest open/closed/error status,
  EN/RU interface. It links the reference Go runtime directly (`invite`,
  `token`, `knock`, `servicecheck`) — no protocol reimplementation. The Wails
  Windows backend is cgo-free, so CI cross-compiles `windows/amd64` and
  `windows/arm64` zips alongside the CLI binaries; unsigned (SmartScreen
  warning accepted for now).

## [0.5.0] — 2026-08-26

Infrastructure release: the Go CLI / provisioner is functionally unchanged from
0.4.2; the macOS client gains in-app auto-update.

### Added
- **Single quality gate: `scripts/verify.sh`.** One command for `go vet` / `go build`
  / `go test` plus `swift build` and `mkpk-selfcheck`; the gate is its exit code.
  Stages that cannot run on the machine report SKIP (`--strict` turns a SKIP into a
  failure), `--quick` builds only, `--docs` adds markdownlint. The CI `test` job now
  runs this same script instead of duplicating the Go commands.
- **Automated macOS releases + in-app auto-update.** A `v*` tag now also runs
  `release:macos` on the macOS runner (`scripts/package_release.sh`): builds,
  Developer ID signs, notarizes and DMG-packs **both** macOS apps (the native
  client and the Wails provision-desktop), attaches them to the GitLab release
  and mirrors everything (DMGs, Go zips, `appcast.xml`) to the public GitHub
  releases. The macOS client gains **Sparkle** in-app updates (app target only;
  feed on GitHub releases, EdDSA-signed) with a "Check for updates" button in
  Settings; dev builds and builds without the feed key stay updater-free.
- **Agent workflow documentation** (`docs/agents/`): roles, the delegation ladder,
  Herdr wave orchestration, and a lessons diary. Supporting process skills in
  `.agents/skills/` (`devils-advocate`, `verification`, `writing-plans`), the HALT
  protocol in `.claude/rules/halt.md`, and `docs/plans/` for wave plans. Practices
  adapted from the maintainer's gitlab-companion project.

## [0.4.2] — 2026-07-31

This release is **macOS-client focused**; the Go CLI / provisioner is functionally
unchanged from 0.4.1 (the binaries are rebuilt under the new tag).

### Added
- macOS client **re-checks the external (egress) IP** — on network changes
  (Wi-Fi ↔ cellular / reconnect), periodically while a port is open, and via a
  manual refresh button in the footer. When a service is open for an address that
  no longer matches, the client flags it (menu-bar attention, an in-app warning and
  a notification) so you know to re-knock. Closes the silent "port is open, but for
  my *old* IP" trap after a reconnect.

### Changed
- macOS client is now **Developer ID signed + notarized** — a downloaded DMG/app no
  longer trips Gatekeeper quarantine (no `xattr -cr` needed). The provision-desktop
  (Wails) app is still ad-hoc for now.

### Fixed
- macOS client: brand icons are flattened into `Contents/Resources` and loaded via
  `Bundle.main`, so the `.app` is codesignable (a resource bundle at the bundle root
  blocked signing).

### Notes
- iCloud Keychain sync of invites stays off pending a Developer ID provisioning
  profile (#24); the Settings toggle falls back to the local file store.

## [0.4.1] — 2026-07-29

### Fixed
- RouterOS poller no longer rewrites the token rule `content=` every second — it
  updates only when the bucket rolls. The unconditional `set` made RouterOS log a
  "filter rule changed by scheduler" line each second per token rule, flooding the
  device log (worsened by keep-open re-knocks). Existing routers need a redeploy
  (uninstall → install) for the fix to take effect.

## [0.4.0] — 2026-07-29

### Added
- Per-service **allowed-timeout** (allowed-list TTL after a knock): configurable per
  service and as a router-wide default (web form + CLI `--allowed-timeout` /
  `router set`), and **exported into the invite** so the client can show the
  open-port countdown.
- **Clock-health** surfacing everywhere: warn on router clock skew / NTP off — a
  silent knock breaker.
- **`mkpk-provision test`** — end-to-end knock test over SSH: knocks from this host,
  then reads the router-side stage1/stage2/token counters, the allow-log and the
  target port to pinpoint exactly how far the sequence got.
- Web UI: **"Test knock"** button per service + live streaming modal; **deploy
  progress streaming** over SSH (NDJSON); **backend-liveness** heartbeat + banner
  (detects a stopped `serve`).
- `mkpk knock --json` and **non-silent result lines** for `knock` / `check`
  (automation-friendly, meaningful exit codes); CLI errors now point at `-h`.

### Changed
- Router-wide default `allowed_timeout` is editable in the UI/CLI (was YAML-only);
  defaults to 3m.

### Fixed
- Dark-theme "saved" toast was unreadable (light text on a light background).
- Desktop invite / `.rsc` download (server-side save); accept an invite path without `@`.

### Notes
- The native **macOS menu-bar client** (`client-macos/`) landed in the repo this
  cycle — Swift runtime pinned to the Go golden vectors, invite import + persistence,
  menu-bar UI, settings, and a dev `.app` bundle. It is built with SwiftPM and
  released separately (signing / notarization pending).

## [0.3.0]

- Multi-router config, channels, poller, SSH deploy, and CLI/web provisioning
  (pre-changelog baseline).

## [0.1.0]

- Initial versioned release.
