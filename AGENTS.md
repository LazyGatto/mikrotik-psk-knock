# AGENTS.md — repo guide

MikroTik **PSK-time port-knock** layer (a PSK-derived time-token knock in front of
RouterOS firewall rules) plus provisioning tooling and clients. This file is the
repo-wide orientation for AI agents / contributors. Subtrees have their own
`AGENTS.md` where relevant.

## Layout

- `client/` — the **Go module**: the provisioner + CLIs (confusingly named; it is
  *not* the recipient app).
  - `client/cmd/mkpk` — end-user knock/check CLI.
  - `client/cmd/mkpk-provision` — admin CLI + local web UI (`serve`).
  - `client/cmd/mkpk-provision-desktop` — Wails desktop wrapper of the web UI.
  - `client/cmd/mkpk-client` — Wails GUI for invite recipients, Windows-first
    (cgo-free cross-compile); the macOS recipient app is native (`client-macos/`).
  - `client/internal/{admin,config,routeros,invite,knock,token,servicecheck,deploy,web,version}`
    — core; `routeros/render.go` generates the `.rsc`, `deploy` applies it over SSH.
- `client-macos/` — the **native Swift menu-bar recipient app** (imports `.mkpk`
  invites, knocks). Has its own `AGENTS.md`. Built with SwiftPM, not in Go CI.
- `routeros/` — RouterOS-side reference / scripts. `docs/` — design, briefs, mockups.
- `deploy/docker/` — compose recipes for the shared provision instance (with and
  without Caddy); the image is built from the root `Dockerfile` by CI on a tag.

The knock token is `sha512(psk|v1|service|client_id|bucket|psk)` (lowercase hex);
`bucket = floor(unixSeconds / bucket_seconds)`. It MUST stay byte-identical across
the Go reference, the Swift client, and the rendered RouterOS rules.

## Build & test

**The gate is one command** — and it is its exit code, never a grep over the output:

```sh
bash scripts/verify.sh          # go vet/build/test + swift build + mkpk-selfcheck
bash scripts/verify.sh --strict # also fail if a stage had to be skipped
bash scripts/verify.sh --docs   # plus markdownlint over the agent docs
echo $?                         # GATE=0 or it is not done
```

Swift stages run on macOS only and report SKIP elsewhere; **a SKIP is not a PASS**.
Live runs against a real router (`mkpk knock`, `mkpk-provision deploy`,
`mkpk-selfcheck live`) are outside the gate — they are the owner's manual check.
Individual commands:

- Go: `cd client && go build ./... && go vet ./...`. Run the CLIs from
  `client/cmd/*`. The provision web UI: `mkpk-provision serve --config mkpk.yaml`
  (loopback, no password). A shared team instance runs in Docker with an admin
  password — `Dockerfile`, `deploy/docker/`, `docs/deploy-docker.md`.
- Swift client: see `client-macos/AGENTS.md` (`swift build`, `swift run mkpk-selfcheck`).

## Agent workflow

Roles, delegation ladder, wave mechanics and the swarm memory live in
[`docs/agents/`](docs/agents/README_FOR_AGENTS.md) — **start there**:

- [`README_FOR_AGENTS.md`](docs/agents/README_FOR_AGENTS.md) — who you are, what to
  read, the gate, forbidden assumptions.
- [`02_executor_pool.md`](docs/agents/02_executor_pool.md) — *who* does the work.
- [`03_orchestration.md`](docs/agents/03_orchestration.md) — *how* a wave runs
  (Herdr worktrees, verify pane, DA gate, landing through an MR).
- [`04_lessons.md`](docs/agents/04_lessons.md) — symptom → cause → cure, one line
  per lesson. Add one when you hit and fix an environment/build/tooling problem.

Supporting material: process skills in [`.agents/skills/`](.agents/skills/)
(`devils-advocate`, `verification`, `writing-plans`), the HALT protocol in
[`.claude/rules/halt.md`](.claude/rules/halt.md), plans in `docs/plans/`, and the
macOS/Swift technical skills in [`client-macos/.agents/skills/`](client-macos/.agents/skills/).
RouterOS-side domain principles: [`agent/instructions.md`](agent/instructions.md).

## Versioning & CI/CD

- **No VERSION file.** `client/internal/version` has `var Version = "dev"`, overridden
  at build time via ldflags: `-X mikrotik-psk-knock/client/internal/version.Version=${CI_COMMIT_TAG}`.
  So **a release is a git tag `vX.Y.Z`** — nothing to bump in-file.
- **CI** (`.gitlab-ci.yml`, runner tag `gitlab`, `golang:1.26`): stages test → build →
  release. Every push runs `test` (vet + build). A tag matching `^v[0-9]+\.[0-9]+\.[0-9]+`
  triggers `build:binaries` (cross-compiles **mkpk + mkpk-provision** for
  linux/darwin/windows × amd64/arm64 → zips → package registry) then `release`
  (GitLab Release). **CI builds only the Go module** — the Swift `client-macos/` app
  ships separately (signed/notarized; tracked in the issues).
- Follow semver: new backward-compatible features → **minor**; bug-fixes only → patch.
- **Never bump/tag without the owner's explicit approval.**

## Release flow

1. Update `CHANGELOG.md` (Keep-a-Changelog).
2. Commit.
3. `git tag vX.Y.Z`.
4. Push the tag → CI builds & releases the Go binaries (`mkpk`, `mkpk-provision`)
   and creates the GitLab Release.
5. **Automated, mac-ci-01 — both macOS apps.** The same tag also runs
   `release:macos` (`scripts/package_release.sh`) on the macOS runner: builds,
   Developer ID signs and notarizes the native `client-macos/` client **and** the
   Wails `mkpk-provision-desktop`, packs both into drag-to-Applications DMGs
   (arm64), attaches them to the GitLab release, mirrors the release to GitHub
   (DMGs + Go zips + `appcast.xml`) and Sparkle-signs the client DMG for
   **in-app auto-update** (feed: GitHub `releases/latest/download/appcast.xml`).
   Runner provisioning (CI keychain, `mkpk-notary` profile, EdDSA key, `wails`):
   [`docs/plans/2026-08-26-macos-release-ci-plan.md`](docs/plans/2026-08-26-macos-release-ci-plan.md).
6. **Artifact retention.** The self-hosted GitLab keeps only the **last two
   versions** — releases, generic packages and image tags alike; the public
   GitHub mirror keeps the full history. `cleanup:gitlab` does this on every tag
   (`scripts/prune_gitlab_storage.sh`, `MKPK_KEEP_RELEASES`, dry-run via
   `MKPK_PRUNE_DRY_RUN=1`). Git tags and the `latest` image tag are never
   touched — a tag is the version history and what already-deployed instances
   pull. **Job artifacts are a separate bucket** and the biggest one if
   forgotten: `build:binaries` produces ~50 MiB per tag, so its `expire_in` is
   one day — the same zips live in the package registry and on GitHub within
   minutes. A backlog of old artifacts is cleared with
   `MKPK_PRUNE_ARTIFACTS=1 bash scripts/prune_gitlab_storage.sh`.
7. **Manual fallback** (maintainer's machine, e.g. when the runner is down):
   `bash scripts/package_release.sh vX.Y.Z` from the tagged checkout runs the
   same build/sign/notarize pipeline locally and prints the `glab`/`gh` upload
   commands. The underlying scripts (`client-macos/script/{build_app,sign,make_dmg,notarize}.sh`,
   `cd client && make desktop`) remain usable individually. Notarization needs
   the one-time keychain profile `mkpk-notary`
   (`xcrun notarytool store-credentials mkpk-notary --apple-id … --team-id R2M77TY8U9 --password <app-specific-pw>`).

`main` is protected; push over HTTPS via the `glab` credential helper. Pushing a
tag is separate from pushing to protected `main`.

## Conventions

- **Primary repo = self-hosted GitLab** (`origin`; CI, issues, releases). A **public
  GitHub mirror** exists at `github.com/LazyGatto/mikrotik-psk-knock` (remote `github`).
  Keep the **GitLab host out of public-facing files** — READMEs and man pages link to
  the GitHub URL, never `gitlab.eg23.ru`. Push code to both remotes; release assets are
  synced to GitHub **by hand** (`glab release download` → `gh release …`) until the sync
  is automated. Issues live on GitLab, so `Closes #N` in commits refers to GitLab issues.
- Task tracking is **GitLab issues** (member-only, though the repo is public).
- **Public repo:** never commit real infrastructure identifiers (router hosts,
  usernames, PSKs) to tracked files — use placeholders (`router.example.com`,
  `203.0.113.x`, synthetic PSKs). Real values are fine only in local/untracked
  config and at runtime.
- **Unified product naming.** Four products, and every artifact carries its
  product name: `mkpk` (recipient CLI), `mkpk-client` (recipient GUI — the
  native macOS app *and* the Wails Windows app; the macOS bundle itself is
  `mkpk.app`), `mkpk-provision` (admin CLI + web UI), `mkpk-provision-desktop`
  (admin GUI). Release files are `<product>_vX.Y.Z_<os>_<arch>.<zip|dmg>`.
  Don't invent new prefixes.
- Commit messages end with a `Co-Authored-By:` trailer when authored with an agent.
- Fail loudly with friendly errors; validate at boundaries (invites, network),
  trust internal code.
