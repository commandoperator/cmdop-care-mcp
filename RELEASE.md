# Releasing cmdop-care-mcp

This document explains the ONE thing you need to remember: how a new version
of `cmdop-care-mcp` actually gets published, and why it is a deliberate,
manual action rather than something that happens automatically when the main
`cmdop` CLI ships a release.

## Why this is manual, not automatic

`cmdop-care-mcp` is a **public, immutable artifact** on the official MCP
Registry. Once a version is published there, it cannot be unpublished or
overwritten (per the Registry's own FAQ) — a bad publish can only be
superseded by a new version, never deleted. Combined with the fact that this
artifact went through an explicit security review before its first release,
every publish deserves a human looking at what's about to ship, not a script
deciding on its own.

For that reason: **this repository is never wired to any CI/CD, and
`cmdop_go`'s own `make release` never triggers a publish here.** The only
thing `cmdop_go`'s release pipeline does is print one non-fatal reminder line
if the internal tool source this artifact models has drifted (see
"How cmdop_go stays in sync" below) — it never builds, tags, or pushes
anything on your behalf.

## The one command

Once you've decided a new version should ship:

```bash
make publish
```

That's it. This single command runs, in order:

1. `go vet` + `go test ./...` — the full test suite must pass.
2. `docker build` — builds the image from the pinned, digest-locked base.
3. A hard local check that the built image runs as **non-root** and carries
   the correct `io.modelcontextprotocol.server.name` OCI label — the script
   **refuses to push** if either check fails, no exceptions.
4. `docker push ghcr.io/commandoperator/cmdop-care:<version>` — reads the
   version from the `VERSION` file.
5. `git tag <version>` and `git push origin <version>`.

If you just want to see everything up through step 3 — build, test, verify —
without touching the network or pushing anything, run:

```bash
make publish-dry-run
```

This is safe to run any time, with no credentials at all, and is the right
way to sanity-check a change before deciding to actually publish.

## One-time setup: the GHCR credential

`make publish` needs a GitHub token with permission to push container images
to GHCR (`ghcr.io/commandoperator/cmdop-care`). This repository never stores
that token — you provide it once, locally, in a gitignored file.

1. Create a GitHub token with **`write:packages`** scope:
   - Classic token: <https://github.com/settings/tokens/new>, tick
     `write:packages`.
   - Fine-grained token: grant it **Packages: Read and write** for this
     repository/account.
2. Copy the template and fill it in:

   ```bash
   cp release/.env.example release/.env
   ```

   ```
   GHCR_USER=commandoperator
   GHCR_TOKEN=<your token>
   ```

3. `release/.env` is gitignored — it never gets committed, never shows up in
   `git status`, and is not part of any commit history. Treat it like any
   other local secret file: don't paste its contents anywhere, don't share
   it, and rotate/revoke the token if you ever suspect it leaked (e.g. if it
   was ever typed into a chat, a ticket, or a screen-shared terminal).

If `release/.env` is missing or `GHCR_TOKEN` is empty, `make publish` stops
with a clear error before attempting any push — it never silently tries an
unauthenticated push or falls back to a different credential source.

## Bumping the version

Edit the `VERSION` file (currently `v0.1.0`) to the next version **before**
running `make publish`. Registry publishes are immutable, so pick the version
deliberately — this is not auto-incremented anywhere.

## How `cmdop_go` stays in sync (without auto-publishing)

`cmdop-care-mcp` is an independent reimplementation, not a vendored copy — it
reads the same host keyring format as the real `cmdop` CLI, but its tool
descriptions and DTOs are modeled on (not imported from) internal cmdop_go
source (`internal/mcp/tools/fleet_care.go`, `fleet.go`,
`core/agent/ports/machinecare.go`). Nothing forces those two to stay
consistent automatically.

To catch drift without auto-publishing anything, `cmdop_go`'s own
`make release` runs a **read-only, non-fatal check**
(`go/scripts/build/check-cmdop-care-drift.sh`) as its very last step: it
compares the pinned checksums in this repository's
`internal/relay/UPSTREAM_SNAPSHOT` against the current upstream files. If
they still match, it prints nothing. If they don't, it prints one warning
telling you to come back to this repository and review whether a matching
update (and a new `make publish`) is needed — then, separately, to update
`UPSTREAM_SNAPSHOT` so the reminder doesn't repeat for the same drift.

That check never touches this repository automatically, never blocks a
`cmdop_go` release, and never decides on your behalf that a new
`cmdop-care-mcp` version should ship. It only makes sure you notice.

## After a publish

Registry registration itself (`mcp-publisher publish` or the registry's own
submission flow) is a separate, still-manual step from pushing the image —
see this artifact's security review
(`cmdop-docs/@go-to-market/startup-programms/applications/active/official-mcp-registry/security-architecture-2026-07-18.md`)
for the full publication checklist, including the Registry Terms of Service
and CC0 metadata dedication review that must happen before that step.
