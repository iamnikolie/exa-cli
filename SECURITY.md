# Security Policy

## Supported versions

The latest release is the supported one. Fixes land on `main` and go out in the
next tag.

## Reporting a vulnerability

Please do **not** open a public issue for a security problem.

Use GitHub's private vulnerability reporting instead:
[Security → Report a vulnerability](https://github.com/iamnikolie/exa-cli/security/advisories/new).
That opens a private advisory visible only to the maintainers.

Include what you did, what happened, and the impact you think it has. Expect a
first response within a week — this is a spare-time project, not a product with
an on-call rotation.

## Scope notes

Some things are known and by design rather than vulnerabilities:

- **The API key is stored in plain text** in `~/.exa-cli/<profile>/config.yaml`
  at mode 0600 — the same posture as `~/.aws/credentials` or `.netrc`. Anyone
  who can read your home directory can spend your Exa credits. Use
  `EXA_API_KEY` from a secret manager if you need better than that.
- **`--verbose` prints request and response bodies to stderr.** The key is not
  logged, but your queries and the returned page content are. Redact before
  pasting output into an issue.
- **Web content is rendered as it arrives.** Search results and fetched pages are
  written by third parties. The CLI does not sanitize them for the terminal, and
  an agent reading them should treat them as untrusted input (prompt injection).

Revoke a leaked key at [dashboard.exa.ai/api-keys](https://dashboard.exa.ai/api-keys);
it stops working immediately.
