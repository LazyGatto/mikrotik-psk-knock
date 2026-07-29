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

- Go: `cd client && go build ./... && go vet ./...`. Run the CLIs from
  `client/cmd/*`. The provision web UI: `mkpk-provision serve --config mkpk.yaml`.
- Swift client: see `client-macos/AGENTS.md` (`swift build`, `swift run mkpk-selfcheck`).

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
2. Commit. 3. `git tag vX.Y.Z`. 4. Push the tag → CI builds & releases.

`main` is protected; push over HTTPS via the `glab` credential helper. Pushing a
tag is separate from pushing to protected `main`.

## Conventions

- Task tracking is **GitLab issues** (member-only, though the repo is public).
- **Public repo:** never commit real infrastructure identifiers (router hosts,
  usernames, PSKs) to tracked files — use placeholders (`router.example.com`,
  `203.0.113.x`, synthetic PSKs). Real values are fine only in local/untracked
  config and at runtime.
- Commit messages end with a `Co-Authored-By:` trailer when authored with an agent.
- Fail loudly with friendly errors; validate at boundaries (invites, network),
  trust internal code.
