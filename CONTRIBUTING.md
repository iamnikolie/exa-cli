# Contributing

Thanks for taking a look. This is a small, focused CLI — bug reports and pull
requests are welcome, and so is a plain question in an issue.

## Reporting a bug

Include the output of `exa version`, the exact command you ran, and what you
expected instead. `--verbose` dumps the API request and response to stderr,
which is usually enough to see what Exa answered — **redact your queries and
anything private before pasting it.** The API key itself is never printed.
The error body (`{"error":"...","tag":"...","requestId":"..."}`) is the part
that matters; the `requestId` lets Exa support find the call.

## Pull requests

Before opening one:

```bash
make fmt           # gofmt -w .
make vet           # go vet ./...
make test          # go test ./...
```

CI runs the same three (plus `-race`) on Linux and macOS, so a green local run
usually means a green PR.

House rules:

- **One concern per PR.** A bug fix and a refactor in the same diff take three
  times as long to review.
- **Tests for pure logic.** The suite never touches the network. When a feature
  needs the API, factor the reference parsing, query building or rendering into
  a function that can be tested without a client — that is how every existing
  command is structured.
- **Keep the output token-lean.** The default rendering exists so an agent can
  read it without burning context. New columns belong behind `--fields`, not in
  the default set.
- **Version and other data go to stdout.** Only errors, hints and `--verbose`
  dumps go to stderr, so `x=$(exa ...)` always works.
- **Update the docs in the same commit.** Any change to the CLI surface must
  also update `cmd/skill.md` (embedded in the binary, printed by `exa skill`,
  and the single source of truth for command UX) and `README.md`.
- **Conventional commit subjects** — `feat:`, `fix:`, `docs:`, `refactor:`,
  `test:`, `chore:`. Release notes are generated from them.

## API coverage

Dedicated commands cover the endpoints people call daily. Everything else
(Websets, monitors, webhooks, imports) goes through `exa api` rather than a
thin wrapper per endpoint — a new command should earn its place with real
rendering or workflow value (polling, streaming, readable output), not just
flag-to-JSON plumbing.

## Releases

Maintainer-only. Tag and push:

```bash
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

GoReleaser builds archives for linux/darwin/windows on amd64 and arm64 and
publishes the GitHub release with a generated changelog.
