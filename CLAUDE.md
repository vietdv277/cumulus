# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Cumulus (`cml`) is a personal CLI tool for AWS and GCP resource management, written in Go.

## Build & Test

`make build/test/lint/all` wrap the usual Go commands (see `Makefile`). CI
(`.github/workflows/ci.yml`) runs lint (golangci-lint v2.14.0), `go test -race`, govulncheck and a
build. A golangci-lint binary built with an older Go than the toolchain crashes on load; use
`go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run` instead.

Tests are pure unit tests: no network or cloud calls, and config/kubeconfig tests point
`HOME`/`XDG_CONFIG_HOME`/`KUBECONFIG` at `t.TempDir()`.

## Architecture

### Two-Layer Command Design

The codebase has a legacy layer and a newer context-aware layer. Both coexist:

- **Legacy**: `cmd/ec2.go`, `cmd/profile.go`, `cmd/asg.go`, `cmd/lb.go`, `cmd/vpc.go`, `cmd/aws/` —
  direct AWS commands with `--profile`/`--region` flags
- **Context-aware**: `cmd/vm.go`, `cmd/secrets.go`, `cmd/db.go`, `cmd/storage.go`, `cmd/k8s.go`,
  `cmd/use.go`, `cmd/status.go`, `cmd/contexts.go` — use a saved context (`~/.config/cml/config.yaml`)
  to route to the right provider/profile/region

Context-aware commands resolve their context through `resolveContext` in
`cmd/context_resolve.go` (the `--context` flag with alias resolution, else the current
context). It rejects an explicitly passed global `-p/--profile` or `-r/--region`, which would
otherwise be silently ignored; check the flag's `Changed` state, never the `profile` variable,
which `initConfig` fills from saved config and env.

Destructive commands (`vm stop/reboot`, the interactive Stop, `secrets delete`,
`storage sync --delete`) confirm through `confirmAction` in `cmd/confirm.go`: `--yes/-y`
skips the prompt, and without a TTY they refuse instead of proceeding.

### Context System (`internal/config/context.go`)

Config lives at `~/.config/cml/config.yaml` (XDG-compliant; respects `$XDG_CONFIG_HOME`).
A context encodes provider + named environment:

```yaml
current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
    bastion: i-013xxxxx            # optional EC2 instance ID (used by k8s connect)
    bastion_port: 8888             # optional; remote port on bastion (default 8888)
  gcp:staging:
    provider: gcp
    project: mycompany-staging
    region: asia-southeast1
    bastion: bastion-host          # optional IAP/SSH bastion
    bastion_project: infra-project # defaults to project
    bastion_zone: asia-southeast1-b
    bastion_iap: true
aliases:                           # short names → full context names
  prod: aws:prod
tunnels:                           # saved tunnel configs (cml vm tunnel --save <name>, cml tunnel list/start/rm)
  db-prod:
    context: aws:prod
    target: i-0abc123
    remote_port: 5432
    local_port: 5432
defaults:
  output: table                    # table | json | yaml — read by the global --output flag (flag wins)
```

Use `ParseContextName("aws:prod")` → `{provider: "aws", name: "prod"}`.

Context-aware commands (`vm`, `secrets`, etc.) accept `--context <name>` to temporarily
override the active context without switching it.
Aliases resolve everywhere a context name is accepted: `--context`, `cml use`, `use update`, `use delete`.
`cml use <unknown>` exits 1 (`config.ErrContextNotFound`). `use add` stores the provider part lower-case.
`cml profile set X` upserts an `aws:X` context and makes it current; legacy commands read the
profile from the current AWS context.

Three migration helpers run at startup for legacy config locations:

- `MigrateFromOldConfig()` — from `~/.cml/config.yaml` (handled by `internal/config/config.go`,
  the legacy-only package)
- `MigrateFromDotFileConfig()` — from `~/.cml.yaml`
- `MigrateFromMacOSConfig()` — from `~/Library/Application Support/cml/config.yaml`

A migration aborts (warning on stderr) when the new config exists but cannot be parsed, and
never overwrites an existing context.

`internal/config/config.go` is legacy-only; all active config logic lives in `internal/config/context.go`.
The config file is written atomically (temp file + rename) with mode 0600, in a 0700 directory.

### Provider Interface (`pkg/provider/interfaces.go`)

All cloud resources are abstracted behind interfaces: `VMProvider`, `SecretsProvider`,
`DBProvider`, `StorageProvider`, `LogsProvider`, `K8sProvider`. The `CloudProvider`
interface aggregates them all. AWS implementations (vm, secrets, db, storage, logs, k8s) live
in `internal/aws/`; GCP implementations (vm, secrets, db, storage, logs, k8s) live in `internal/gcp/`.
GCP secrets, db and storage use the REST clients in `google.golang.org/api` (Secret Manager,
Cloud SQL Admin, Cloud Storage); `db connect` on GCP runs `cloud-sql-proxy`, and GCS presign
signs V4 URLs locally from service-account key credentials.
`LogsProvider` is implemented for AWS (CloudWatch Logs) and GCP (Cloud Logging, `logging/v2`).

### `k8s connect`

`cml k8s connect <cluster>` (AWS only) uses `AWSVMProvider.StartPortForward`
(in `internal/aws/vm.go`) to open an SSM port-forward to the context's
`bastion` instance on `bastion_port`, then spawns `$SHELL -i` with
`HTTPS_PROXY`/`HTTP_PROXY` set. On subshell exit (or SIGINT), the tunnel
is torn down in the deferred handler. `StartPortForward` is shared with
`vm tunnel`.

### Logs (`cml logs`)

`cml logs <target>` queries, `cml logs <target> -f` tails, `cml logs list
[prefix]` lists targets. AWS (`internal/aws/logs.go`): a target is a log
group; CloudWatch `FilterLogEvents`/`DescribeLogGroups` via paginators, deduped
by event ID. GCP (`internal/gcp/logs.go`): a target is a log name;
`entries.list` with a `logName`/`timestamp` filter plus `--filter` in
parentheses, deduped by insert ID. Both tail by polling (default 2s).
`--follow` rejects `-o yaml` and `--limit`; Ctrl-C during a tail exits 0.

### Tunnels

Tunnel lifecycle rules: the local port must be free before the tunnel starts (a busy port
would silently route through another tunnel); readiness fails fast if the tunnel process
exits; the SSM session runs in its own process group (`internal/aws/tunnel_unix.go`,
`tunnel_windows.go`) and teardown signals the whole group so `session-manager-plugin` dies
too; `vm tunnel` traps SIGINT/SIGTERM itself; SIGTERM to `k8s connect` reaches the subshell
as SIGHUP. Tunnel sessions get no stdin.

Package-specific notes live next to the code: `internal/aws/CLAUDE.md` (errors, secrets routing),
`internal/gcp/CLAUDE.md` (zone/region handling), `internal/kubeconfig/CLAUDE.md`.

## Conventions

- Safe pointer dereference: use `deref(s *string) string` pattern in `internal/aws/`
- Error wrapping: `fmt.Errorf("context: %w", err)` consistently
- All operations accept `context.Context` for cancellation
- Use `aws.String()` helper for AWS SDK pointer fields
- Cloud provider logic stays in `internal/<provider>/`
- Shared domain types live in `pkg/types/`; provider interfaces in `pkg/provider/`
- UI components (selectors, tables) live in `internal/ui/`
- Context-aware commands get their context from `resolveContext`; never re-implement the lookup
- Destructive operations use `confirmAction` with a `--yes` flag
- OS-specific code (signals, process groups) needs build-tagged files; releases target
  linux/darwin/windows, so `GOOS=windows go build ./...` must pass
- Interactive selectors return `selection cancelled` on Esc/q/Ctrl-C (exit 0); any other
  selector error is returned
- `rootCmd` silences cobra's error printing (`Execute` prints `Error: ...` once) and sets
  `SilenceUsage` in its `PersistentPreRun`, so usage prints only for flag and argument errors;
  a subcommand defining its own `PersistentPreRun` must keep that behaviour
- Error hints: `Execute` passes the error to `errhint.Hints` (`internal/errhint`), which
  classifies by error type/API code first and message text only for the matching provider;
  return nil rather than a wrong hint. Keep SDK errors wrapped with `%w` so the types survive
- Not-found errors for user-typed names append `suggest.DidYouMean(suggest.Similar(name,
  candidates, 3))` (`internal/suggest`; contexts via `config.ContextSuggestion`) and keep any
  sentinel (`ErrContextNotFound`, `ErrAliasNotFound`) wrapped
- Retries: the AWS SDK standard retryer (3 attempts) and the Compute Engine client (reads on
  503/504) retry on their own; the `google.golang.org/api` REST clients do not. Do not add a
  second retry layer on AWS
- Data-printing commands route through `render` (`cmd/output.go` → `internal/output`); with
  json/yaml, stdout carries only the document and empty results print `[]`
