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
  - `client/internal/{admin,config,routeros,invite,knock,token,servicecheck,deploy,web,version}`
    — core; `routeros/render.go` generates the `.rsc`, `deploy` applies it over SSH.
- `client-macos/` — the **native Swift menu-bar recipient app** (imports `.mkpk`
  invites, knocks). Has its own `AGENTS.md`. Built with SwiftPM, not in Go CI.
- `routeros/` — RouterOS-side reference / scripts. `docs/` — design, briefs, mockups.

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
  `client/cmd/*`. The provision web UI: `mkpk-provision serve --config mkpk.yaml`.
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
5. **Manual, macOS-only — the Wails provision-desktop app.** CI can't build it
   (needs macOS + `wails` CLI + Xcode CLT), so build and attach it to the same
   Release by hand:

   ```sh
   cd client && make desktop          # VERSION auto-resolves from the tag via git describe
   # → cmd/mkpk-provision-desktop/build/bin/mkpk-provision-desktop.app
   ../client-macos/script/make_dmg.sh cmd/mkpk-provision-desktop/build/bin/mkpk-provision-desktop.app \
     /tmp/mkpk-provision-desktop_vX.Y.Z_darwin_arm64.dmg mkpk-provision
   glab release upload vX.Y.Z /tmp/mkpk-provision-desktop_vX.Y.Z_darwin_arm64.dmg
   ```

   Needs `wails` (`go install github.com/wailsapp/wails/v2/cmd/wails@latest`) + Xcode CLT.
6. **Manual, macOS-only — the `client-macos/` recipient app.** Built, **Developer ID
   signed + notarized**, and attached to the same Release:

   ```sh
   cd client-macos
   ID="Developer ID Application: EDINY GOROD, OOO (R2M77TY8U9)"
   MKPK_VERSION=vX.Y.Z script/build_app.sh              # → .build/mkpk.app
   script/sign.sh                                       # Developer ID + hardened runtime + entitlements
   script/notarize.sh .build/mkpk.app                   # notarize + staple the app (offline first-launch)
   MKPK_SIGN_ID="$ID" script/make_dmg.sh .build/mkpk.app /tmp/mkpk-client_vX.Y.Z_darwin_arm64.dmg mkpk
   script/notarize.sh /tmp/mkpk-client_vX.Y.Z_darwin_arm64.dmg   # notarize + staple the DMG
   glab release upload vX.Y.Z /tmp/mkpk-client_vX.Y.Z_darwin_arm64.dmg
   ```

   Both macOS apps ship as **drag-to-Applications DMGs** (`client-macos/script/make_dmg.sh`,
   no deps — hdiutil + an /Applications symlink), arm64. The **client** is now signed +
   notarized + stapled (`spctl -a` → "Notarized Developer ID"; no quarantine prompt).
   Notarization needs the one-time keychain profile `mkpk-notary`
   (`xcrun notarytool store-credentials mkpk-notary --apple-id … --team-id R2M77TY8U9 --password <app-specific-pw>`).
   The **provision-desktop** (Wails) app is still ad-hoc — apply the same sign/notarize
   pass to it (tracked in the issues); until then clear its quarantine with `xattr -cr <app>.app`.

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
- Commit messages end with a `Co-Authored-By:` trailer when authored with an agent.
- Fail loudly with friendly errors; validate at boundaries (invites, network),
  trust internal code.
