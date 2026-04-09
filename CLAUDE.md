# Claude Code — Project Instructions

## Config & example files

- Never include real org names, URLs, hostnames, emails, or any identifying
  information in example config files, `.env.example`, README code blocks, or
  any other file that could be committed to a public repo.
- Always use obviously generic placeholders: `your-org`, `your-instance`,
  `user@example.com`, etc.
- `settings.yaml` and `.env` are gitignored — never commit them.

## README

- Update `README.md` before every push to GitHub or PR creation.
- The README should always reflect the current state of the project.

## Go conventions

- No external packages without a clear reason — prefer stdlib where practical.
- Return errors; don't log-and-continue in library code (`internal/`). Logging
  belongs in the command layer (`cmd/`).
- Use `0` as the sentinel "not set" value for optional integer IDs passed to
  Snipe-IT — omit them from the request body rather than sending `0`.
- Validate required config values early in the command, before any API calls.

## Dry-run safety

- Dry-run must never create, modify, or delete anything in external systems.
- Read-only API calls (GET/search) are fine in dry-run.
- Log `[dry-run]` prefix on every action that would have been taken.
- Report the same result counters as a real run so the output is meaningful.

## Snipe-IT API

- POST/PATCH responses use a `{ status, messages, payload }` envelope.
  Always check `env.Status == "success"` — a 200 can still carry an error.
- GET responses return the object directly with no envelope.
- Rate-limit all Snipe-IT calls to 2 req/s (`rate.Every(500ms)`).

## Okta API

- Use percent-encoding in filter query strings (`%20` for space, `%22` for
  quotes). Literal quotes cause a 400.
- HTTP 403 from `/api/v1/users/{id}/roles` means the API token lacks the
  roles-read permission for that user — treat as empty role list, not an error.
