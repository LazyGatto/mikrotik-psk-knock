# Changelog

Notable changes to this project. Format: [Keep a Changelog](https://keepachangelog.com).
Versions are the **Go CLI / provisioner** release tags (`mkpk`, `mkpk-provision`);
the native macOS recipient app in `client-macos/` ships separately.

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
