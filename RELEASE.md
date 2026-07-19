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

## After a publish: registering the version on the official MCP Registry

`make publish` gets the image and git tag out. Registering that version on
`registry.modelcontextprotocol.io` — so MCP clients can actually discover
it — is a **separate, still-manual step**, done with the official
`mcp-publisher` CLI. This section is the exact, tested sequence (every
mistake below was hit and fixed for real during the `v0.1.2` publish —
follow it in order to skip repeating them).

### 0. One-time: make the GHCR package public

GitHub creates a freshly pushed GHCR package as **Private** by default, even
though the repository and the image push both succeeded. A private package
cannot be pulled by an MCP client or by the Registry's own validation.

1. Go to <https://github.com/commandoperator?tab=packages> → open
   `cmdop-care`.
2. **Package settings** (right sidebar) → **Danger Zone** → **Change
   visibility** → **Public**. Confirm by typing the package name.
3. Verify anonymously, with no login, from any machine:
   ```bash
   docker logout ghcr.io   # make sure you're not using a cached credential
   docker pull ghcr.io/commandoperator/cmdop-care:<version>
   ```
   If this works without `docker login`, the package is really public.

You only need to do this once per package (not per version) — subsequent
`make publish` runs push new tags into the same already-public package.

### 1. Confirm the image is genuinely multi-arch

`docker build` (no `buildx`) only ever produces ONE platform: whatever the
build host's native architecture is. `make publish` already uses `docker
buildx build --platform linux/amd64,linux/arm64 --push`, so this should be
automatic — but double-check after any Dockerfile change:

```bash
docker manifest inspect ghcr.io/commandoperator/cmdop-care:<version>
```

You should see two `platform` entries — `{"architecture":"amd64","os":"linux"}`
and `{"architecture":"arm64","os":"linux"}` — alongside two `unknown/unknown`
attestation manifests (those are normal buildx metadata, not a bug).

### 2. Verify the shipped binary reports the version you think it does

`main.go`'s version is stamped at build time via ldflags
(`-X main.version=...`), sourced from the `VERSION` file — not hardcoded. If
you ever add a new binary entrypoint or change how the Dockerfile builds,
confirm the running binary agrees with the tag:

```bash
docker run --rm --platform linux/amd64 --entrypoint="" \
  ghcr.io/commandoperator/cmdop-care:<version> /cmdop-care --version
```

### 3. Install `mcp-publisher`

```bash
curl -fsSL -o /tmp/mcp-publisher.tar.gz \
  https://github.com/modelcontextprotocol/registry/releases/latest/download/mcp-publisher_darwin_amd64.tar.gz
tar -xzf /tmp/mcp-publisher.tar.gz -C /tmp
/tmp/mcp-publisher --help
```

(Swap `darwin_amd64` for `darwin_arm64`/`linux_amd64`/etc. to match your
machine — check the release assets at
<https://github.com/modelcontextprotocol/registry/releases/latest> if unsure.)

### 4. Log in as the account that OWNS the `io.github.<owner>` namespace

```bash
cd projects/github/cmdop-care-mcp
/tmp/mcp-publisher login github
```

This opens a GitHub device-code flow in your browser (`github.com/login/device`
+ a one-time code). **This must be the same GitHub account as the `name` field
in `server.json`** — `io.github.commandoperator/cmdop-care` requires being
signed in as `commandoperator`, not your personal account, even if your
personal account has push access to the repository. Signing in as the wrong
account produces a confusing-looking 403 much later, at `publish` time:

```
Error: publish failed: server returned status 403: {"title":"Forbidden",
"detail":"You do not have permission to publish this server. You have
permission to publish: io.github.<your-personal-account>/*. Attempting to
publish: io.github.commandoperator/cmdop-care. ..."}
```

If that happens: `/tmp/mcp-publisher logout`, then `login github` again,
making sure the browser/device-flow prompt is completed while signed into
the correct GitHub account (switch accounts in the browser first if needed).

Confirm before continuing — the CLI doesn't print who's signed in, but you
can inspect the token's own claim:

```bash
python3 -c "import json,base64,sys; \
  t=json.load(open('$HOME/.config/mcp-publisher/token.json'))['token']; \
  print(json.loads(base64.urlsafe_b64decode(t.split('.')[1]+'==')))"
```

Look for `"auth_method_sub"` — it must match the owner in `server.json`'s
`name` field.

### 5. Validate, then publish

```bash
/tmp/mcp-publisher validate   # safe, hits the registry read-only
/tmp/mcp-publisher publish    # NOT reversible — this is the real submission
```

**Known gotcha:** the local schema validator (`ajv-cli`) and even
`mcp-publisher validate` do not catch every server-side rule. One that bit
`v0.1.2`: an OCI `packages[]` entry must **not** carry its own `"version"`
field — the version belongs only inside the image tag in `"identifier"`
(e.g. `ghcr.io/commandoperator/cmdop-care:0.1.2`). Having both fields passes
local validation but fails `publish` with a 400:

```
"registry validation failed for package 0 (...): OCI packages must not have
'version' field - include version in 'identifier' instead"
```

Fix: delete the `"version"` key from that package object (the top-level
`server.json` `"version"` field stays — only the nested one inside
`packages[]` must go), then re-run `validate` and `publish`.

### 6. Confirm it's really live

```bash
curl -fsSL "https://registry.modelcontextprotocol.io/v0/servers?search=cmdop-care"
```

Look for `"status":"active"` and `"isLatest":true` in the response — that's
independent proof the registry actually has it, not just that the CLI printed
success.

### After that

Update this artifact's card in
`cmdop-docs/@go-to-market/startup-programms/applications/active/official-mcp-registry/`
(move it toward `applications/submitted/` once this is truly the final state)
with the published version, the registry confirmation above, and the date.
