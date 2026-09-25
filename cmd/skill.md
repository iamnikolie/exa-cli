# exa — Exa CLI (agent reference)

Agent-facing Exa from Bash: web search, page contents, grounded answers and
agent runs. Markdown on stdout, errors/progress/cost on stderr.

## Setup
Key from https://dashboard.exa.ai/api-keys. `exa config init` (prompts or reads
stdin), or `exa config init --api-key ...`, or just export `EXA_API_KEY`
(overrides the file). Profiles: `--config <name>` (env `EXA_CONFIG`, default
`default`) → `~/.exa-cli/<name>/config.yaml`. `exa config show` masks the key.

## Global
- `--json` (= `--format json`) raw API JSON; `--format md|table|csv|tsv`.
- `--fields url,title,publishedDate` projects result rows (dotted paths OK);
  with `--json` it emits a lean array instead of the full response.
- `--timeout 5m` per request; `--verbose` dumps requests/responses to stderr.
- Cost of each call prints to stderr as `(cost $0.007)` in non-JSON modes.

## Commands
- `exa search <query>` — web search. Default content: highlights (relevance-
  sized snippets; cheapest useful context). Change with `--text [--max-chars N]`,
  `--summary [--summary-query q] [--summary-schema json]`, `--highlights-query q`,
  or `--no-contents` (links only).
  Filters: `-n 10`, `--type auto|fast|instant|neural|deep-lite|deep|deep-reasoning`,
  `--category company|people|news|publication|personal site|financial report`,
  `--include-domain a.com,b.com` XOR `--exclude-domain ...`,
  `--after 2026-01-01 --before 2026-06-30` (published date),
  `--include-text "phrase"` / `--exclude-text "phrase"` (≤5 words),
  `--country US`, `--moderation`.
  Synthesis (deep types): `--schema '{"type":"text"}'` or a JSON schema object →
  `## Output` section before `## Sources`; `--system-prompt`, `--also "alt query"` (repeat).
  Freshness: `--max-age 0` forces a live crawl; `--max-age 24` accepts a cache ≤24h.
  Crawl more: `--subpages 3 --subpage-target docs,api`, `--links 10`, `--image-links 5`.
- `exa fetch <url>...` — clean page contents; default full markdown text.
  Same content/freshness flags as search. Per-URL failures go to stderr as
  `! <url>: error <TAG>`.
- `exa similar <url>` — pages like this one; search's filters + content flags,
  `--exclude-source-domain`.
- `exa code <query> [-n 10]` — code/API-docs preset (fast search, query-steered
  highlights, 300-char text preview).
- `exa answer <question>` — LLM answer + `Sources:` list. `--stream`, `--text`
  (full citation text), `--model exa|exa-pro`, `--schema json`, `--system-prompt`.
- `exa agent run <query|->` — Exa Agent run (beta; header sent automatically).
  `--effort minimal|low|medium|high|xhigh|auto|ultra|max`, `--schema json`,
  `--max-cost 5` (auto/ultra), `--max-duration 900` (ultra), `--previous <run-id>`
  (follow-up), `--input '{"data":[...]}'`, `--data-source <provider>` (repeat),
  `--wait`. Output: text, structured JSON, per-field grounding citations.
  `exa agent get <id> [--wait]`, `agent list`, `agent cancel <id>`, `agent events <id>`.
- `exa api <METHOD> <path> [-d json|@file|-] [-q k=v]... [-H Name:value]...` —
  any endpoint, e.g. Websets under `/websets/v0/...` (needs a plan with Websets access).
- `exa config init|show`, `exa skill`, `exa version`.

Waiting (`--wait`): polls every `--poll 5s`, status changes on stderr, gives up
after `--wait-timeout 30m` (the task keeps running — resume with `get <id> --wait`).
Never re-create a task just because a wait timed out.

JSON args (`--schema`, `--summary-schema`, `--input`, `--metadata`, `-d`) accept
inline JSON, `@file.json`, or `-` for stdin. A query of `-` reads stdin.

## Workflows
- Quick lookup: `exa search "..." -n 5` → read highlights → `exa fetch <url> --max-chars 8000` for the one that matters.
- Links only, cheapest: `exa search "..." --no-contents --format table`.
- Lean JSON for scripts: `exa search "..." --json --fields url,title,publishedDate`.
- Fresh news: `exa search "..." --category news --after 2026-09-01 --max-age 0`.
- One-shot fact with sources: `exa answer "..."`.
- Structured facts: `exa search "..." --type deep --schema @schema.json` or
  `exa agent run "..." --schema @schema.json --wait`.
- Long job: `id=$(exa agent run "..." --effort high)`; later `exa agent get "$id" --wait`.

The old Research API (`/research/v1`) is retired by Exa (HTTP 410); use `exa agent run`.

## Errors
`exa error: HTTP <code>: <message> [TAG] (request <id>)` + a hint for 401/402/403/429.
429 and 502/503/504 are retried with backoff (Retry-After honoured).
