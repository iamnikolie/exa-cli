# exa-cli (`exa`)

[![CI](https://github.com/iamnikolie/exa-cli/actions/workflows/ci.yml/badge.svg)](https://github.com/iamnikolie/exa-cli/actions/workflows/ci.yml)
[![Go Reference](https://pkg.go.dev/badge/github.com/iamnikolie/exa-cli.svg)](https://pkg.go.dev/github.com/iamnikolie/exa-cli)
[![License: MIT](https://img.shields.io/badge/license-MIT-blue.svg)](LICENSE)

An agent-facing CLI for the [Exa](https://exa.ai) search API. It replaces the Exa
MCP server inside Claude Code and other agent workflows: the agent runs
`exa <command>` from Bash and reads token-lean Markdown (`--json`/`--format`
for machines). One static binary, no runtime.

Covers the whole API surface: search (including deep search with structured
output), page contents, find-similar, answer (with streaming), Agent runs, and a
raw `exa api` passthrough for Websets, monitors, webhooks and anything newer.

Sibling of [`fibery-cli`](https://github.com/iamnikolie/fibery-cli),
[`gitlab-cli`](https://github.com/iamnikolie/gitlab-cli),
[`slack-cli`](https://github.com/iamnikolie/slack-cli) and
[`dziga`](https://github.com/iamnikolie/dziga): same doctrine — plain text on
stdout, nothing costs context until it is called.

> Unofficial, community-built tool. Not affiliated with, endorsed by, or supported by Exa Labs.

## Install

**Homebrew:**

```bash
brew trust iamnikolie/tap   # Homebrew 6 refuses untrusted third-party taps
brew tap iamnikolie/tap
brew install iamnikolie/tap/exa-cli
```

**Prebuilt binary** — download the archive for your platform from
[Releases](https://github.com/iamnikolie/exa-cli/releases), then:

```bash
tar xzf exa-cli_*_darwin_arm64.tar.gz
sudo mv exa /usr/local/bin/
```

**With Go** (1.23+) — note `go install` names the binary after the module, so
rename it:

```bash
go install github.com/iamnikolie/exa-cli@latest
mv "$(go env GOPATH)/bin/exa-cli" "$(go env GOPATH)/bin/exa"
```

**From source** — `make install` symlinks the binary, so a later `make build`
updates the installed CLI without reinstalling:

```bash
git clone https://github.com/iamnikolie/exa-cli.git
cd exa-cli
make install        # symlink → ~/.local/bin/exa
```

Check what you got with `exa version`.

> The binary is called `exa`. If you still have the archived `exa` file lister
> (the predecessor of `eza`) on your PATH, one of them will shadow the other —
> build with `make build BIN=exa-search` or rename the binary.

## Setup

Get an API key at [dashboard.exa.ai/api-keys](https://dashboard.exa.ai/api-keys), then either:

```bash
exa config init                 # paste the key when prompted (or pipe it on stdin)
export EXA_API_KEY=...          # or use the env var; it overrides the saved key
```

The key is stored in `~/.exa-cli/default/config.yaml` (mode 0600). Keep several
keys apart with profiles: `exa --config work config init`, then
`exa --config work search ...` or `EXA_CONFIG=work`.

## Usage

```bash
# Search: 10 results with relevance-sized highlights
exa search "rust async runtime comparison"

# Filters, dates, domains
exa search "vector database benchmarks" -n 5 --after 2026-01-01 --include-domain arxiv.org,github.com

# Full text instead of highlights, or links only
exa search "go 1.27 release notes" --text --max-chars 4000
exa search "exa ai" --no-contents --format table

# Deep search with a synthesized, structured answer
exa search "who leads Exa's research team" --type deep --schema '{"type":"text"}'

# Page contents (clean markdown), fresh crawl, subpages
exa fetch https://go.dev/doc/effective_go --max-chars 8000
exa fetch https://news.example.com --max-age 0
exa fetch https://docs.example.com --subpages 5 --subpage-target api,reference

# Similar pages, code examples
exa similar https://example.com/blog/post --exclude-source-domain
exa code "Go http.Client retry with exponential backoff"

# Grounded answer with sources (streaming optional)
exa answer "What is the latest stable Go version?" --stream

# Agent runs (beta): structured, grounded output; async unless --wait
exa agent run "Find 10 seed-stage devtools startups in Berlin" --schema @schema.json --wait
id=$(exa agent run - --effort high < brief.txt); exa agent get "$id" --wait

# Anything else, raw
exa api GET /websets/v0/websets -q limit=5
```

Output formats: Markdown by default; `--json` for the raw API response;
`--format table|csv|tsv` for result rows; `--fields url,title` to pick fields
(dotted paths allowed). Costs (`(cost $0.007)`), progress and per-URL fetch
errors go to stderr, so `x=$(exa ...)` stays clean.

`exa skill` prints the full agent reference — every command, flag and workflow.
Paste it into an agent's instructions, or point the agent at it.

## Use from Claude Code

Add to your `CLAUDE.md` (or a skill):

```markdown
Web search: use the `exa` CLI via Bash. Run `exa skill` once for the reference.
Default: `exa search "<query>" -n 5`; read a page with `exa fetch <url> --max-chars 8000`.
```

## Development

```bash
make build    # ./exa
make test     # go test ./... — never touches the network
make vet fmt
```

See [CONTRIBUTING.md](CONTRIBUTING.md). Security reports: [SECURITY.md](SECURITY.md).

## License

MIT — see [LICENSE](LICENSE).
