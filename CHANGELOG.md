# Changelog

Notable changes to this project. Format: [Keep a Changelog](https://keepachangelog.com).
Versions are the **Go CLI / provisioner** release tags (`mkpk`, `mkpk-provision`);
the native macOS recipient app in `client-macos/` ships separately.

## [Unreleased]

### Added
- **Отпечаток инвайта** (#38). Инвайт работает, только пока совпадает с тем, что
  залито на роутер: сменились PSK, порт стука, адрес или bucket — и выданный
  инвайт мёртв, причём молча (стук по UDP доставку не подтверждает). Теперь есть
  короткий отпечаток ровно от этих полей:
  - `mkpk invite show @laptop.mkpk [--json]` — печатает роутер, сервисы, порты и
    отпечаток; PSK не печатается никогда;
  - в админке отпечаток стоит рядом с доступом юзера и в модалке выдачи инвайта —
    сверить «инвайт на руках ↔ конфиг» стало делом одного взгляда;
  - при сохранении, которое сломало уже выданные инвайты, админка показывает,
    у кого именно, и предлагает перейти к этим юзерам. Косметические правки
    (примечание, allowed-timeout, launch-пресет) молчат.

## [0.15.0] — 2026-08-27

### Added
- **«О программе» в веб-админке** — пункт в меню внизу сайдбара: версия
  инсталляции, ссылка на проект, на документацию и, главное, на страницу
  последнего релиза, откуда пользователи забирают клиентов (mkpk-client для
  macOS и Windows, CLI `mkpk`). Показывает и результат проверки обновлений.

### Fixed
- **Деплой роутера, у которого есть сервисы, но нет ни одного пользователя**
  (#37). Пустая таблица клиентов рендерилась как `:set mkpkTtClients { }`, а
  для RouterOS `{ }` — это пустой блок кода, а не пустой массив: `/import`
  падал с `syntax error`, при том что `dry-run` проходил. Теперь используется
  идиома `[:toarray ""]`. Заодно `mkpk-tt-apply-service` всегда заканчивается
  `:return 0`, чтобы у роутера вообще без сервисов не получился пустой
  `source={ }`.
- **Зеркалирование релиза на GitHub** падало на `mktemp -t relnotes`: это
  BSD-идиома, а джоба переехала с macOS-раннера на Debian, где нужен шаблон с
  `XXXXXX`. Пустой путь скармливал в API пустое тело релиза и всплывал как
  невнятный HTTP 422.

## [0.14.0] — 2026-08-27

### Added
- **Windows client: re-run the command without knocking.** A ▶ button next to
  each service (and a click on the green `open` badge) starts the connection
  again while the router's window is still open — reconnecting after the RDP
  session drops no longer costs a knock. The port is probed first, so an expired
  window is reported as closed instead of launching into nothing.

### Fixed
- **The Windows client actually runs the post-knock command** — it reported
  "launched" while nothing happened, for presets and typed commands alike. Go
  escapes process arguments the way MSVC expects and `cmd.exe` does not read
  that, so any command containing quotes (`start "" mstsc /v:host:port`, which
  is every RDP preset) reached the shell mangled. The line is now passed to
  `cmd.exe` verbatim.
- **A failed launch says so** — only the shell starting was checked, so a
  command that could not run was indistinguishable from one that worked. The
  client now waits briefly, reports a non-zero exit in the UI, and records every
  attempt (command, exit status, output) in `launch.log` next to the config; the
  path is shown under the command field.

## [0.13.0] — 2026-08-27

### Added
- **Onboarding tidies up after itself**: if you had to allow password login on
  the router (`/ip ssh set password-authentication=yes`) to let provision in,
  it sets the value back to RouterOS's default `yes-if-no-key` afterwards — the
  service account uses its key from then on. Shown as a checkbox in the dialog
  (on by default), reported in the result, and available as
  `--restore-password-auth` on `mkpk-provision serviceuser add`. Only fires when
  the connection actually used a password and the router really was set to
  `yes`.
- **Router commands are copyable** — the key-import command in the SSH key
  dialog, and the `/ip ssh set password-authentication=yes` hint that now
  appears right under a failed password login, come with a copy button instead
  of asking you to retype them.

## [0.12.1] — 2026-08-27

### Fixed
- **Password login to a router works again where RouterOS wants
  keyboard-interactive** — some builds offer only that method, and the client
  knew only plain `password`, so onboarding failed with "no supported methods
  remain". A failed login now also explains the usual cause: RouterOS defaults
  to `password-authentication=yes-if-no-key`, so importing an SSH key for a user
  silently disables password login *for that user*.
- **CHANGELOG history restored** — cutting a release replaced the previous
  version's heading instead of inserting above it, so the 0.11.0, 0.11.1,
  0.11.2 and 0.12.0 sections silently merged into `[Unreleased]` and vanished
  from the published changelog. Sections rebuilt from the release commits.
- **A sleeping macOS runner no longer stalls the public mirror** — pushing the
  tag, creating the GitHub release and uploading the Go archives moved out of
  the macOS job into `mirror:github` on the Linux runner. The macOS job now only
  adds what it alone can build: the signed DMGs and the Sparkle appcast. Before
  this, an unavailable Mac meant no GitHub release at all, so clients stopped
  seeing updates even though every Linux stage had succeeded.

## [0.12.0] — 2026-08-27

Routers get their own service account instead of borrowing an administrator's
login: adding a router installs it, deleting a router takes it away.

### Added
- **Adding a router now onboards it** (#33): the add-router dialog asks for
  administrator credentials once, installs the `mkpk` service account with the
  installation key, and saves the router only after that succeeds — so a router
  in the list is one provision can actually reach. Deleting a router removes the
  account from it (the account deletes itself); if the router is unreachable it
  is still removed from the config, with a warning and the command to finish by
  hand.
- **Service account on the router** (#33, backend + CLI): provision installs its
  own `mkpk` user with a minimal group (`ssh,ftp,read,write,policy,test` —
  confirmed on a live RouterOS 7.23.2; `sensitive` is not needed) and the
  installation key, so deploys stop depending on an administrator's personal
  login. `mkpk-provision serviceuser status|add|remove`: `add` uses one-off
  admin credentials, `remove` lets the account delete itself — keys, user and
  group in one command — so handing a router back needs no admin login. RouterOS
  refuses a passwordless user, so onboarding generates a random password it
  never stores; authentication is by key.

### Changed
- **Telegram notification form regrouped**: the bot token gets its own
  full-width field first, with chat id and topic side by side underneath.

### Fixed
- **The last router can be deleted** — an empty config was rejected as invalid,
  so removing the only router failed after its service account had already been
  removed. A config with no routers is now a valid state (it is also what a
  fresh install looks like).
- **Provision footer no longer overlaps its own version string** — the icon row
  (key, password, sign-out, language, theme) is now one menu button opening a
  compact popup, which also gives the actions readable labels instead of
  icon-guessing.
- **Every modal has a close button** (× in the header) — the SSH key dialog had
  no way out but Esc.
- **The public key wraps instead of scrolling** in the SSH key dialog, so the
  whole key is visible at a glance.

## [0.11.2] — 2026-08-27

### Fixed
- **The SSH key button was missing from the provision UI** — the deploy key
  landed in the API, the CLI and the docs, but the patch that added the sidebar
  button, the modal and its strings never made it into `app.js`, so there was
  no way to see or download the public half from the interface. The router
  form's "instance key" button was equally broken (no icon, untranslated
  label).

## [0.11.1] — 2026-08-26

### Fixed
- **The `--behind-ingress` port can be published on a LAN address** — it was
  pinned to `127.0.0.1`, which only works when the reverse proxy runs on the
  same host. `--bind` (and `MKPK_BIND` in `.env`) now takes the address the
  proxy can reach, with `--port` alongside it. `--bind 0.0.0.0` is the practical
  choice for an internal VM on DHCP, where a fixed address would break after a
  reboot; the installer warns when the port leaves loopback, since that traffic
  is plain HTTP.

## [0.11.0] — 2026-08-26

A remote host goes from nothing to a running instance in one command:

```sh
curl -fsSL https://raw.githubusercontent.com/LazyGatto/mikrotik-psk-knock/main/deploy/docker/install.sh \
  | sudo sh -s -- --domain mkpk.example.com
```

### Added
- **OCI labels on the image** (`source`, `title`, `description`, `licenses`,
  `version`), so the published package links back to the repository.
- **Public image in GitHub Packages** — every tag now also publishes
  `ghcr.io/lazygatto/mikrotik-psk-knock/provision:vX.Y.Z` and `:latest`
  alongside the GitLab registry copy, under the same path. The installer needs
  no credentials or image argument by default; `--registry <host>` switches to a
  private mirror of the same project and `--tag` pins a version.
- **One-command installer** `deploy/docker/install.sh`: checks Docker, fetches
  the compose files, generates the admin password, starts the stack, waits for
  the container to report healthy and prints the URL, the password and the next
  steps. `--domain` runs Caddy with automatic TLS, `--behind-ingress` publishes
  a local port for an existing proxy, `--install-docker` bootstraps Docker.
  Re-running keeps the existing password, so it doubles as the upgrade path.

### Fixed
- **DMG packaging retries `hdiutil`** — it intermittently fails with
  `Resource busy` on the CI machine (Spotlight still holding the staged copy),
  which failed a whole release job for no real reason.

## [0.10.0] — 2026-08-26

The deploy identity now belongs to the installation rather than to one admin,
and `docs/deploy-docker.md` carries a from-scratch runbook for a remote host.

### Fixed
- **Compose no longer carries a wrong (and private) registry hostname** — the
  image path was invented rather than checked, so `docker pull` would have
  failed, and it also put the internal GitLab host into public files against
  the project's own rule. `MKPK_IMAGE` is now required and documented; the real
  path comes from GitLab's Container Registry page and lives only in the
  operator's untracked `.env`.

### Added
- **Step-by-step remote-server deployment guide** in
  `docs/deploy-docker.md`: prerequisites, Docker, files, `.env`, restricting
  access before the first start, first login and password change, the deploy
  key, backups, upgrades and a troubleshooting table.
- **Deploy key that belongs to the installation** — provision can generate its
  own ed25519 SSH key (`ssh/id_ed25519` beside the config, 0600) instead of
  borrowing an admin's personal one: the key icon in the sidebar shows the
  public half with copy and `.pub` download, the router form fills the key path
  in one click, and `mkpk-provision sshkey show|create` does the same from a
  shell. The private half is never served by the API. Regenerating is guarded —
  it invalidates the key on every router that already trusts it.

## [0.9.0] — 2026-08-26

Provision can now run as one shared instance for a team instead of a local copy
per admin — see [docs/deploy-docker.md](docs/deploy-docker.md) and the widened
trust zone in [docs/threat-model.md](docs/threat-model.md).

### Added
- **Provision: Docker image and two compose recipes** (#32) — `Dockerfile`
  (static CGO-free binary, alpine, non-root, healthcheck, `/data` volume) plus
  `deploy/docker/`: `compose.yaml` for an ingress you already run, and
  `compose.caddy.yaml` with Caddy and automatic Let's Encrypt certificates for
  a from-scratch host. CI publishes the image on every tag to the GitLab
  registry (`…/provision:vX.Y.Z` and `:latest`). New guide
  `docs/deploy-docker.md` covers all three ways to run provision.
- **Provision: shared-instance mode with a password** (#31) — the same web UI
  can now serve a whole team instead of living in each admin's local copy.
  A password (argon2id, stored in `mkpk-admin.json` beside the config, seeded
  from `MKPK_ADMIN_PASSWORD` or `mkpk-provision passwd`) turns on server-side
  sessions with `HttpOnly; SameSite=Strict; Secure` cookies, a per-session CSRF
  token, login throttling, sign-out and password change in the UI. Local use is
  unchanged: loopback `serve` and the desktop app need no password.
  `serve` refuses a non-loopback address without a password, and refuses plain
  HTTP without `--behind-proxy` / `MKPK_BEHIND_PROXY=1`.
- **Provision: lost-update protection** — the config carries a version and a
  save against a config another admin already changed is rejected with 409 and
  a reload, instead of silently overwriting their work.
- **Provision: launch-preset badge on the service row** — a service that opens
  an app on the client side now shows an `↗ RDP` badge next to its type, so the
  preset is visible without opening the service form.

## [0.8.0] — 2026-08-26

### Added
- **Admin-defined launch presets** (#30): a service can carry a launch *kind*
  (`rdp` / `ssh` / `http` / `https` / `vnc`, picked in the provision service
  form) which travels in the invite; the GUI client opens the matching app
  itself after a confirmed open — the user configures nothing. The invite
  never carries a command line (unsigned file → would be arbitrary code
  execution); the client builds the invocation from the router address and
  port, and refuses to launch when the host is not a plain hostname/IP.
  A user's own local command still overrides the preset.
- **mkpk-client: run a command after a service opens.** Each service gets an
  optional local command (the ⚙ next to it), executed only after a confirmed
  `open` — e.g. `start "" mstsc /v:{host}:{port}` to jump straight into RDP.
  `{host}`, `{port}` and `{service}` are substituted; knocking from the tray
  launches it too. The command is typed by the user and stored only on that
  machine (`settings.json`): invites are unsigned, so executable strings are
  deliberately never carried in them (see issue #30).
- **`docs/notifications.md`** — setup guide for all three notification channels:
  how to get a Telegram `bot_token` / `chat_id` / forum-topic `thread_id`
  (with a curl self-test), the webhook JSON payload, SMTP fields, and how to
  read the router-side diagnostics.
- **Telegram notifications into a forum-supergroup topic**: `notify.telegram`
  gains an optional `thread_id` (Bot API `message_thread_id`) — set it in the
  router's Notifications tab; empty keeps the General topic / plain chats.
  Re-deploy the router to apply.
- **mkpk-client: About** — a `?` button in the header opens an About popup
  (brand icon, version, short description, GitHub link), and the tray menu
  gains an "mkpk vX.Y.Z — About" item opening the project page.

### Fixed
- **Provision UI: long views scroll again** — the dashboard and the user access
  matrix (any view taller than the window) were unscrollable because the view
  wrapper never constrained the content height; resizing the window was the
  only workaround.

## [0.7.0] — 2026-08-26

### Added
- **mkpk-client (Windows): system tray** (#29) — the brand icon lives in the
  tray (green-dot variant while any port is open, tooltip with the open count);
  the menu offers "Open mkpk", one knock item per service with a live
  "open · Nm left" countdown, and Quit. Closing the window hides it to the
  tray, not the taskbar (the minimize button stays native — Wails v2 cannot
  intercept it). New dependency: `fyne.io/systray` (BSD-3, app target only).
- **mkpk-client: light theme** — a sun/moon toggle next to the language
  switch; the choice persists in settings.
- **About → GitHub** in both GUI clients: a footer link in mkpk-client
  (opens via the system browser) and a "GitHub" button in the macOS client's
  Settings.

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
