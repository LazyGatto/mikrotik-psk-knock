# Changelog

Notable changes to this project. Format: [Keep a Changelog](https://keepachangelog.com).
Versions are the **Go CLI / provisioner** release tags (`mkpk`, `mkpk-provision`);
the native macOS recipient app in `client-macos/` ships separately.

## [Unreleased]

### Added
- **Single quality gate: `scripts/verify.sh`.** One command for `go vet` / `go build`
  / `go test` plus `swift build` and `mkpk-selfcheck`; the gate is its exit code.
  Stages that cannot run on the machine report SKIP (`--strict` turns a SKIP into a
  failure), `--quick` builds only, `--docs` adds markdownlint. The CI `test` job now
  runs this same script instead of duplicating the Go commands.
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
