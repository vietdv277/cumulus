# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Cumulus (`cml`) is a personal CLI tool for AWS and GCP resource management, written in Go.

## Module Path

`github.com/vietdv277/cumulus`

All imports must use this full path.

## Tech Stack

- Language: Go 1.26+ (`go.mod`; CI builds on the latest 1.26.x patch)
- CLI Framework: Cobra + Viper
- AWS SDK: `aws-sdk-go-v2`
- GCP SDK: `cloud.google.com/go/compute` + `google.golang.org/api`
- TUI: charmbracelet/bubbletea (interactive selectors) + charmbracelet/lipgloss (styling)

## Build & Test

```bash
# Build (injects version/commit/date via ldflags)
make build

# Run without building
make run ARGS="ec2 ls"

# Test all packages
make test

# Run a single test
go test ./internal/aws/... -run TestName -v

# Test with coverage report
make test-cover

# Format, vet, test, and build
make all

# Lint (requires golangci-lint)
make lint

# Vulnerability scan (also runs in CI)
go run golang.org/x/vuln/cmd/govulncheck@latest ./...

# Install to $GOPATH/bin
make install
```

CI (`.github/workflows/ci.yml`) runs lint (golangci-lint v2.14.0), `go test -race`, govulncheck
and a build. A golangci-lint binary built with an older Go than the toolchain crashes on load;
use `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.14.0 run` instead.

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
tunnels:                           # saved tunnel configs — reserved, not yet used by any command
  db-prod:
    context: aws:prod
    target: i-0abc123
    remote_port: 5432
    local_port: 5432
defaults:                          # reserved, not yet read by commands
  output: table                    # table | json | yaml
  interactive: false
  region_fallback: us-east-1
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
interface aggregates them all. AWS implementations (vm, secrets, db, storage, k8s) live
in `internal/aws/`; GCP implementations (vm, k8s) live in `internal/gcp/`. GCP secrets, db and
storage are not implemented: the provider getters in `cmd/` return a "not yet implemented for
GCP" error.
`LogsProvider` has no implementation yet.

### Kubeconfig (`internal/kubeconfig/`)

Read-only reader for `~/.kube/config` (or `KUBECONFIG`). Every file in `KUBECONFIG` is
merged like kubectl does (first file wins for duplicates and `current-context`). Extracts
contexts and current-context for `cml k8s contexts`. Writing is delegated to cloud CLIs
(`aws eks update-kubeconfig`, `gcloud container clusters get-credentials`) — do not
add kubeconfig mutation here.

`cml k8s connect <cluster>` (AWS only) uses `AWSVMProvider.StartPortForward`
(in `internal/aws/vm.go`) to open an SSM port-forward to the context's
`bastion` instance on `bastion_port`, then spawns `$SHELL -i` with
`HTTPS_PROXY`/`HTTP_PROXY` set. On subshell exit (or SIGINT), the tunnel
is torn down in the deferred handler. `StartPortForward` is shared with
`vm tunnel`.

Tunnel lifecycle rules: the local port must be free before the tunnel starts (a busy port
would silently route through another tunnel); readiness fails fast if the tunnel process
exits; the SSM session runs in its own process group (`internal/aws/tunnel_unix.go`,
`tunnel_windows.go`) and teardown signals the whole group so `session-manager-plugin` dies
too; `vm tunnel` traps SIGINT/SIGTERM itself; SIGTERM to `k8s connect` reaches the subshell
as SIGHUP. Tunnel sessions get no stdin.

### AWS Client (`internal/aws/client.go`)

Uses the functional options pattern:

```go
client, err := aws.NewClient(ctx, aws.WithProfile("prod"), aws.WithRegion("us-east-1"))
```

The `Client` struct holds sub-clients for EC2, Auto Scaling, ELBv2, RDS, S3, and EKS.
They are constructed lazily on first use; SSM and STS calls use the shared
AWS config loaded into the client.

### GCP Client (`internal/gcp/client.go`)

Uses Application Default Credentials (ADC) via `google.FindDefaultCredentials`. Auth resolves in order:
`GOOGLE_APPLICATION_CREDENTIALS` env var → gcloud user credentials → GCE metadata server.

```go
client, err := gcp.NewClient(ctx, gcp.WithProject("my-project"), gcp.WithRegion("us-central1"))
```

GCP VM connectivity uses `gcloud compute ssh`, optionally routing through an IAP bastion configured on the context.

GCP `region` field behaviour (`zoneRE`/`regionRE` in `internal/gcp/vm.go`):

- a zone (e.g. `us-central1-a`) uses a zone-scoped `instances.List`;
- a region uses `instances.AggregatedList` filtered to that region's zones;
- anything else is an error.

`bastion_zone` falls back to `region` only when `region` is a zone. A VM name found in
several zones is an error.

### AWS errors and lookups

- Report "not found" only for the real not-found API codes (`isNotFoundErr` in
  `internal/aws/errors.go`); wrap and return everything else (AccessDenied, expired SSO).
- List calls use the SDK v2 paginators.
- A VM name matching several non-terminated instances is an error listing each `id (state)`.

### Secrets Routing (`internal/aws/secrets.go`)

- `ssm:<name>` / `sm:<name>` force the backend (prefix stripped).
- Names starting with `/` route to SSM Parameter Store; other names route to Secrets Manager.
  `get` falls back to SSM on a Secrets Manager not-found; `set` updates an existing SSM
  parameter of that name instead of creating a duplicate secret.
- `delete` never falls back to SSM: it errors and points to `ssm:<name>`. Secrets Manager
  deletes are recoverable for 7 days unless `--force`.
- `List()` aggregates both, or one backend with `--ssm-only` / `--sm-only`.

## Full Command Tree

```text
cml
├── vm                     # Context-aware unified VM management (AWS + GCP)
│   ├── list               # List VMs (--state, --name, --tag, --interactive)
│   ├── get <id|name>      # Get VM details
│   ├── connect <id|name>  # SSH/SSM (AWS) or gcloud ssh (GCP)
│   ├── tunnel <id> ...    # Port forwarding via SSM or gcloud
│   ├── start/stop/reboot  # Lifecycle (stop/reboot confirm; --yes)
├── secrets                # Unified secrets (SSM + Secrets Manager)
│   ├── list               # List secrets (--ssm-only / --sm-only)
│   ├── get <name>         # Get secret value (ssm:/sm: prefixes force the backend)
│   ├── set <name> <val>   # Create/update secret
│   └── delete <name>      # Delete secret (confirms; --yes, --force = no recovery window)
├── db                     # Managed databases (AWS RDS; GCP Cloud SQL not implemented)
│   ├── list               # List databases (--engine filter)
│   ├── get <name>         # Details
│   └── connect <name>     # Port-forward tunnel (--via <instance-id>, else context bastion)
├── storage (s3)           # Object storage (S3; GCS not implemented); paths use s3://bucket/key
│   ├── ls [s3://b [pfx]]  # List buckets, or objects under prefix
│   ├── cp <src> <dst>     # Copy one object (local ↔ remote, s3 → s3; ≤ 5 GiB)
│   ├── sync <src> <dst>   # Sync directory (--delete confirms; --yes)
│   └── presign <s3://...> # Presigned GET URL (expiry ≤ 7 days)
├── k8s                    # Kubernetes cluster management
│   ├── list               # List EKS / GKE clusters in context
│   ├── get <name>         # Cluster details
│   ├── use <name>         # Update kubeconfig + switch kubectl context
│   ├── connect <name>     # AWS: SSM tunnel to context bastion + subshell with HTTPS_PROXY (--local-port)
│   └── contexts           # List kubectl contexts
├── use                    # Context management
│   ├── <context>          # Switch active context
│   ├── add <context>      # Add context (--profile/--project, --region, bastion flags)
│   ├── update <context>   # Update specific fields of a context
│   └── delete <context>   # Remove context
├── status                 # Show current context and auth info
├── contexts (ctx)         # List all contexts
├── ec2                    # Legacy AWS EC2 commands
│   ├── ls                 # List instances
│   └── ssh                # Interactive SSM session selector (bubbletea TUI)
├── profile                # Legacy AWS profile management (set → aws:<profile> context)
├── vpc                    # VPC listing/selection
├── asg                    # Auto Scaling Group commands
├── lb                     # Load Balancer commands
├── aws
│   ├── ssm param          # SSM Parameter Store (list, get, set)
│   └── iam whoami         # Show caller identity (STS)
└── version                # CLI version info
```

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

## UI Patterns

Interactive selectors (bubbletea `tea.Model`) in `internal/ui/` follow a consistent pattern:
search/filter input, cursor navigation, details panel, status bar. Table renderers use Unicode
box-drawing characters from `internal/ui/styles.go`. State indicators: `●` running, `○` stopped,
`◐` pending.

## Version Injection

Version metadata is injected at build time via ldflags into `cmd.Version`, `cmd.Commit`,
`cmd.BuildDate` (see Makefile).
