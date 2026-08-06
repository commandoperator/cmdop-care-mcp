# Changelog

Notable changes per released version. Dates are the release date.

Published versions on the MCP Registry are immutable — a version cannot be
withdrawn or overwritten, so a fix always ships as a new version. See
[`RELEASE.md`](RELEASE.md).

No publicly known vulnerabilities have been reported or fixed in any released
version to date. Security reports go to **security@cmdop.com**; see
[`SECURITY.md`](SECURITY.md).

## v0.1.2 — 2026-07-18

**Fixed**

- The binary reported version `0.1.0` regardless of the tag it was built from,
  because the version was a hardcoded constant. It is now stamped at build time
  via `-ldflags` from the `VERSION` file, so `--version` reports the tag that
  was actually published. The `VERSION` file is the single source of truth.

## v0.1.1 — 2026-07-18

**Fixed**

- The published image claimed multi-architecture support but shipped an
  amd64 binary inside the arm64 manifest, so it would not run on Apple Silicon
  or arm64 Linux. The build now uses buildx `TARGETOS`/`TARGETARCH` per
  requested platform and produces a genuine `linux/amd64` + `linux/arm64`
  image index.

## v0.1.0 — 2026-07-18

First public release.

**Added**

- Four read-only diagnostics tools over MCP stdio transport.
- Credentials read from the OS keyring already populated by the main `cmdop`
  CLI. This binary implements no enrollment of its own and holds no credential.
- Distroless container image, pinned by digest, running as non-root (uid
  65532), statically linked with `CGO_ENABLED=0`.
