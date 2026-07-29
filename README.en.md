<p align="center">
  <img src="docs/logo.png" width="140" alt="mkpk">
</p>

<h1 align="center">mkpk — MikroTik PSK Knock</h1>

<p align="center">
  Open MikroTik services on demand with an authenticated port-knock.<br>
  Ports aren't exposed to the internet around the clock — they open briefly, and only
  for one specific address, after a successful secret-keyed "knock".
</p>

<p align="center">
  <a href="https://gitlab.eg23.ru/lazygatto/mikrotik-psk-knock/-/releases">Releases</a> ·
  <a href="docs/roadmap.md">Roadmap</a> ·
  <a href="docs/man/">Man pages</a> ·
  <a href="README.md">Русский</a>
</p>

---

## What it is

A `dst-nat` forward left open to the internet 24/7 is a permanent target. **mkpk** keeps
the service closed and opens it surgically: the client sends a staged UDP "knock" and a
short-lived PSK token, the router verifies them and adds **that exact** source address to an
allowed-list with a timeout. The rest of the time the port is closed, and the router carries
no standing holes and no external service — only native RouterOS features.

The project is not just the protocol, but a full toolset around it:

- **Runtime client** `mkpk` — knocks and checks reachability (CLI).
- **Admin tool** `mkpk-provision` — config, RouterOS script rendering, SSH deploy, client
  issuance. Available as a **CLI**, a **local web UI** (`serve`), and a **desktop app**
  (`mkpk-provision-desktop`, a native Wails window) — all over one core.
- **macOS client** (`client-macos/`) — a native **menu-bar app** for invite recipients:
  imports `.mkpk`, knocks/checks, shows the open-access countdown, and can "keep open"
  (auto re-knock before expiry). The same crypto runtime as the CLI (reimplemented in Swift
  and pinned to the Go golden vectors).

The CLI and the web UI (`serve`, opens in a browser) run on **any OS**; the native desktop is
just a convenient wrapper around the same UI and is built for macOS only. The cryptographic
runtime is entirely client-side; SSH is only the deployment channel.

## Features

- **Port-knock via a PSK-time-token** — staged UDP as a cheap filter + a `sha512` token bound
  to time (bucket); the `token-hit → poller → allowed` model narrows the replay window.
- **Multi-router, user × service** — one config for many routers; an access matrix; a separate
  PSK per (user, router) pair; a per-service token.
- **Client issuance** — a compact per-user invite blob (only their router address, PSK, services),
  without the shared admin config.
- **Three frontends, one core** — CLI (scriptable, for Ansible), a local web UI (loopback +
  a per-session token), and the desktop wrapper.
- **Notifications** — webhook / Telegram / email on every successful knock, with graceful degradation.
- **SSH provisioning** — install/update/uninstall the layer over SSH, idempotently (detected by
  a config hash), with dry-run.
- **Secure by default** — the config with all secrets is written 0600 atomically and never leaves
  the machine; the web UI is loopback-only; an invite carries only the router's public address.

## How it works

```text
client
  -> UDP knock stage 1
  -> UDP knock stage 2
  -> UDP token stage with a short-lived PSK token

MikroTik
  -> adds the src-address to a token-hit address-list
  -> the poller picks a valid hit and marks the bucket/token as used
  -> adds that src-address to the allowed address-list with a timeout
  -> notifies the owner
  -> dst-nat starts working only for that src-address
```

## Router requirements

- RouterOS 7.x (tested on 7.23.2).
- **Accurate clock (NTP enabled).** The token is bound to a 30-second time bucket, and the router
  accepts only the current + previous bucket. If the router clock drifts by more than ~half a
  bucket, tokens stop matching and **the knock silently fails** (visible in the firewall:
  stage1/stage2 match, but the token rule shows 0 packets). Enable with
  `/system ntp client set enabled=yes` + add a server. When polling the router, the provisioning
  app (web/desktop) checks the time and NTP status and warns if knocking won't work.

## Installation

Prebuilt binaries are on the [Releases](https://gitlab.eg23.ru/lazygatto/mikrotik-psk-knock/-/releases)
page (built by CI on a tag). Each CLI ships in a per-platform `.zip` — inside is the binary under
its plain name, with the executable bit preserved (no `chmod +x` needed).

For **macOS** there are also two native apps as **DMGs** (drag-to-Applications):
`mkpk-provision-desktop` (admin) and `mkpk-client` (the recipient menu-bar app).

> **macOS — Gatekeeper quarantine.** While the builds aren't notarized ([#12](https://gitlab.eg23.ru/lazygatto/mikrotik-psk-knock/-/issues/12)),
> downloaded binaries/apps are quarantined. Clear it manually:
> - CLI: `xattr -d com.apple.quarantine ./mkpk`
> - app: `xattr -cr /Applications/mkpk.app` (likewise for `mkpk-provision-desktop.app`)

Build from source (the `client/` directory):

```text
make build       # CLI: bin/mkpk and bin/mkpk-provision (version from the git tag)
make desktop     # desktop admin .app (macOS; needs the wails CLI + Xcode CLT)
make install     # binaries + man pages under PREFIX (default /usr/local)
make test        # go test ./...
```

The macOS client is a separate SwiftPM project: `cd client-macos && script/build_app.sh`
(and `script/make_dmg.sh` for a DMG). See [client-macos/AGENTS.md](client-macos/AGENTS.md).

## CLI & automation

All three frontends are thin wrappers over the `internal/admin` core, and **the CLI is
self-sufficient**: the web and desktop do nothing beyond it. A typical headless flow:

```text
mkpk-provision profile init --out mkpk.yaml --router-name r1 --router-address r1.example.com
mkpk-provision service add --config mkpk.yaml --name ssh \
  --stage1-port 41011 --stage2-port 41012 --token-port 41013 \
  --target-type forward --target-port 22 --target-to-address 192.0.2.10 --target-to-port 22
mkpk-provision user add --config mkpk.yaml --name laptop --services ssh
mkpk-provision deploy --config mkpk.yaml               # installs the layer over SSH
mkpk-provision export --config mkpk.yaml --user laptop --out laptop.mkpk
mkpk knock --invite @laptop.mkpk --service ssh --check # on the client side
```

`deploy` and `config validate` support `--json` for scripts/Ansible; `check --json` (and
`knock --json`) give a machine-readable result. Full reference — in the man pages (`mkpk(1)`,
`mkpk-provision(1)`) or `mkpk-provision help`.

## Status

A working ROS-only implementation with a CLI, a local web UI, a desktop admin app, and a
**native macOS client** for invite recipients, plus SSH provisioning and streamed deploy
progress; all verified end-to-end on live routers (RouterOS 7.x). Versioning is semver, pre-1.0;
the current version and binaries are on the [Releases](https://gitlab.eg23.ru/lazygatto/mikrotik-psk-knock/-/releases)
page. There's also an end-to-end **knock test** from the provisioning app (it knocks and verifies
the router-side counters/log/port over SSH), and "keep open" in the client. Next up: notarizing the
macOS builds (Developer ID) and an ICMP transport variant. Details — in [docs/roadmap.md](docs/roadmap.md).

## Docs

- [docs/context.md](docs/context.md) — consolidated context and technical notes.
- [docs/design.md](docs/design.md) — the initial ROS-only design.
- [docs/threat-model.md](docs/threat-model.md) — threat model and limitations.
- [docs/admin-app.md](docs/admin-app.md) — the admin-app model, multi-router, issuance (invite blob).
- [docs/multi-profile-render.md](docs/multi-profile-render.md) — the render scheme and data-driven poller.
- [docs/profile-format.md](docs/profile-format.md) — config field reference.
- [docs/open-questions.md](docs/open-questions.md) — open questions and decisions taken.
- [docs/roadmap.md](docs/roadmap.md) — the plan for further work.
- [docs/man/](docs/man/) — CLI man pages.
- [client/README.md](client/README.md) — CLI, provisioning and SSH-deploy details.
