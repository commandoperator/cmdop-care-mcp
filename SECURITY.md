# Security Policy

## Reporting a vulnerability

Report security issues privately to **security@cmdop.com**.

Please do not open a public issue for a suspected vulnerability. A public
report is visible to everyone before there is a fix, which helps an attacker
more than it helps a user.

Include whatever you have: what you observed, how to reproduce it, the version
(`cmdop-care --version` or the tag you built from), and the host operating
system. A partial report is worth sending — we would rather triage something
incomplete than never hear about it.

### What to expect

- **Acknowledgement within 14 days.** If you have not heard back in that time,
  the message did not reach us; please resend or open a public issue saying
  only that you sent a private report and got no reply, with no detail.
- We will tell you whether we consider it a vulnerability and, if so, roughly
  when a fix will ship.
- Once a fix is released we will credit you in the release notes unless you ask
  us not to.

This project is maintained by a small team. We do not operate a bug bounty and
cannot pay for reports.

## Supported versions

Only the latest released version receives fixes. Published versions on the MCP
Registry are immutable and cannot be withdrawn, so a security fix ships as a
**new version** — see [`RELEASE.md`](RELEASE.md).

| Version | Supported |
|---|---|
| latest release | yes |
| everything earlier | no — upgrade |

## What this binary is, in security terms

Understanding the trust boundary makes reports much more useful.

`cmdop-care` is a **read-only diagnostics** MCP server exposing four tools. It
is deliberately narrow:

- **It holds no credential of its own.** It reads the relay token and any
  per-machine connection PIN that the main `cmdop` CLI has already stored in
  the operating system keyring (macOS Keychain, Linux Secret Service, Windows
  Credential Manager).
- **It does not implement enrollment.** The host must already be joined via
  `cmdop join <join-key>`. This binary never accepts a token through an
  environment variable or a mounted secret as its primary path.
- **It does not write.** The tools report machine health; they do not change
  configuration, install software, or execute arbitrary commands.
- **It speaks stdio, not a network port.** The container `EXPOSE`s nothing.

So the most valuable reports concern: credential exposure through the keyring
path, any way to make the binary act as a write surface, output that leaks
information a caller should not see, or supply-chain concerns about the
published image.

## Build and supply chain

- The published image is built from a pinned distroless base **by digest**, not
  by a floating tag, so a change to the upstream base cannot silently alter
  what ships.
- It runs as non-root (uid 65532).
- The binary is statically linked with `CGO_ENABLED=0`.
- Releases are deliberate and manual; nothing publishes automatically. See
  [`RELEASE.md`](RELEASE.md) for why.

If you believe a published artifact does not match this repository, that is a
security report and we want to hear it.
