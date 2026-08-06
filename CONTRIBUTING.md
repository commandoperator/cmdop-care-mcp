# Contributing

Thanks for looking. This is a small, deliberately narrow project, so the most
useful contribution is usually a precise bug report rather than a large change.

## Before you open a pull request

**Open an issue first for anything beyond a fix.** This binary has a stated
scope — read-only diagnostics, four tools, no credentials of its own — and a
threat model that was reviewed before implementation. A change that widens that
scope will be declined however well it is written, and it is unfair to let you
find that out after the work.

Changes that are welcome without discussion:

- correcting something that is factually wrong (documentation, an error
  message, a command that no longer exists);
- fixing a bug with a test that fails before the fix and passes after;
- improving an error message so a stuck user knows what to do next.

Changes that need an issue first:

- a new tool, or a new argument to an existing tool;
- anything that writes, executes, installs, or changes host state;
- a new dependency;
- a new way to supply credentials.

## Security issues are not pull requests

If you have found a vulnerability, **do not open an issue or a PR**. Report it
privately — see [`SECURITY.md`](SECURITY.md). A public patch for a security bug
tells everyone about the bug before anyone can upgrade.

## Building and testing

Requires Go 1.26 or later. No CGO, no code generation step, no external
services.

```sh
make help     # list every target
make build    # compile the binary locally
make test     # run the full test suite
```

`make test` is the gate. Run it before opening a pull request; a change that
breaks it will not be merged.

**New functionality needs a test.** The existing tests live beside the code
they cover (`internal/**/*_test.go`) — follow that layout. If you are fixing a
bug, the ideal patch adds a test that fails on `main` and passes with your
change, so the bug cannot return unnoticed.

## Style

- Keep the diff to the change you are making. Unrelated reformatting makes
  review slower and hides the real edit.
- Write comments that explain **why**, not what. The surrounding code does this
  consistently; match it.
- Do not add a dependency to save a few lines. This binary ships in a
  distroless image and its dependency count is part of its security posture.

## Commits and pull requests

- Write a commit subject that says what changed and why it matters. "chore:
  commit in-progress work" is the example to avoid — it describes nothing to
  the next reader.
- One logical change per pull request.
- Describe how you tested it. "Ran `make test`" is a complete answer for most
  changes.

## Releases

Contributors do not publish releases. Publication to the MCP Registry is
manual, deliberate and irreversible — a published version cannot be withdrawn
or overwritten. See [`RELEASE.md`](RELEASE.md) for how and why.

## Licence

By contributing you agree that your contribution is licensed under
[Apache-2.0](LICENSE), the same licence as the project.
