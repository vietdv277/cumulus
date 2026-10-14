# `internal/aws`

## Errors and lookups

- Report "not found" only for the real not-found API codes (`isNotFoundErr` in `errors.go`); wrap
  and return everything else (AccessDenied, expired SSO).
- List calls use the SDK v2 paginators.
- A VM name matching several non-terminated instances is an error listing each `id (state)`.

## Secrets routing (`secrets.go`)

- `ssm:<name>` / `sm:<name>` force the backend (prefix stripped).
- Names starting with `/` route to SSM Parameter Store; other names route to Secrets Manager.
  `get` falls back to SSM on a Secrets Manager not-found; `set` updates an existing SSM
  parameter of that name instead of creating a duplicate secret.
- `delete` never falls back to SSM: it errors and points to `ssm:<name>`. Secrets Manager
  deletes are recoverable for 7 days unless `--force`.
- `List()` aggregates both, or one backend with `--ssm-only` / `--sm-only`.
