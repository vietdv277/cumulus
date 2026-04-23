# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Cumulus (`cml`) is a personal CLI tool for AWS and GCP resource management, written in Go.

## Module Path

`github.com/vietdv277/cumulus`

All imports must use this full path.

## Tech Stack

- Language: Go 1.24+
- CLI Framework: Cobra + Viper
- AWS SDK: `aws-sdk-go-v2`
- GCP SDK: `cloud.google.com/go/compute` + `google.golang.org/api`
- TUI: charmbracelet/bubbletea (interactive selectors) + charmbracelet/lipgloss (styling)

## Build & Test

> There are currently no `_test.go` files in the repository. The test commands below are valid but will report nothing to test.

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

# Install to $GOPATH/bin
make install
```

## Architecture

### Two-Layer Command Design

The codebase has a legacy layer and a newer context-aware layer. Both coexist:

- **Legacy**: `cmd/ec2.go`, `cmd/profile.go` — direct AWS commands with `--profile`/`--region` flags
- **Context-aware**: `cmd/vm.go`, `cmd/secrets.go`, `cmd/use.go`, `cmd/status.go`, `cmd/contexts.go`
  — use a saved context (`~/.config/cml/config.yaml`) to route to the right provider/profile/region

### Context System (`internal/config/context.go`)

Config lives at `~/.config/cml/config.yaml` (XDG-compliant; respects `$XDG_CONFIG_HOME`). A context encodes provider + named environment:

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
tunnels:                           # saved tunnel configs (used by vm tunnel --save)
  db-prod:
    context: aws:prod
    target: i-0abc123
    remote_port: 5432
    local_port: 5432
defaults:
  output: table                    # table | json | yaml
  interactive: false
  region_fallback: us-east-1
```

Use `ParseContextName("aws:prod")` → `{provider: "aws", name: "prod"}`.

Context-aware commands (`vm`, `secrets`, etc.) accept `--context <name>` to temporarily override the active context without switching it.

Three migration helpers run at startup for legacy config locations:
- `MigrateFromOldConfig()` — from `~/.cml/config.yaml` (handled by `internal/config/config.go`, the legacy-only package)
- `MigrateFromDotFileConfig()` — from `~/.cml.yaml`
- `MigrateFromMacOSConfig()` — from `~/Library/Application Support/cml/config.yaml`

`internal/config/config.go` is legacy-only; all active config logic lives in `internal/config/context.go`.

### Provider Interface (`pkg/provider/interfaces.go`)

All cloud resources are abstracted behind interfaces: `VMProvider`, `SecretsProvider`,
`DBProvider`, `StorageProvider`, `LogsProvider`, `K8sProvider`. The `CloudProvider`
interface aggregates them all. AWS implementations (vm, secrets, db, storage, k8s) live
in `internal/aws/`; GCP implementations (vm, k8s) live in `internal/gcp/`.

### Kubeconfig (`internal/kubeconfig/`)

Read-only reader for `~/.kube/config` (or `KUBECONFIG`). Extracts contexts and
current-context for `cml k8s contexts`. Writing is delegated to cloud CLIs
(`aws eks update-kubeconfig`, `gcloud container clusters get-credentials`) — do not
add kubeconfig mutation here.

`cml k8s connect <cluster>` (AWS only) uses `AWSVMProvider.StartPortForward`
(in `internal/aws/vm.go`) to open an SSM port-forward to the context's
`bastion` instance on `bastion_port`, then spawns `$SHELL -i` with
`HTTPS_PROXY`/`HTTP_PROXY` set. On subshell exit (or SIGINT), the tunnel
is torn down in the deferred handler. `StartPortForward` is shared with
`vm tunnel`.

### AWS Client (`internal/aws/client.go`)

Uses the functional options pattern:

```go
client, err := aws.NewClient(ctx, aws.WithProfile("prod"), aws.WithRegion("us-east-1"))
```

The `Client` struct holds sub-clients for EC2, SSM, STS, Auto Scaling, ELBv2, and Secrets Manager.

### GCP Client (`internal/gcp/client.go`)

Uses Application Default Credentials (ADC) via `google.FindDefaultCredentials`. Auth resolves in order: `GOOGLE_APPLICATION_CREDENTIALS` env var → gcloud user credentials → GCE metadata server.

```go
client, err := gcp.NewClient(ctx, gcp.WithProject("my-project"), gcp.WithRegion("us-central1"))
```

GCP VM connectivity uses `gcloud compute ssh`, optionally routing through an IAP bastion configured on the context.

GCP `region` field behaviour: if the value looks like a zone (e.g. `us-central1-a` — two or more hyphens, ends with letter a–f), `internal/gcp/vm.go` uses a zone-scoped `instances.List`; otherwise it uses `instances.AggregatedList` filtered to zones prefixed by the region string.

### Secrets Routing (`internal/aws/secrets.go`)

Names starting with `/` route to SSM Parameter Store; others route to AWS Secrets Manager.
Both are aggregated by `List()`.

## Full Command Tree

```text
cml
├── vm                     # Context-aware unified VM management (AWS + GCP)
│   ├── list               # List VMs (--state, --name, --tag, --interactive)
│   ├── get <id|name>      # Get VM details
│   ├── connect <id|name>  # SSH/SSM (AWS) or gcloud ssh (GCP)
│   ├── tunnel <id> ...    # Port forwarding via SSM or gcloud
│   ├── start/stop/reboot  # Lifecycle
├── secrets                # Unified secrets (SSM + Secrets Manager)
│   ├── list               # List all secrets from both sources
│   ├── get <name>         # Get secret value
│   ├── set <name> <val>   # Create/update secret
│   └── delete <name>      # Delete secret
├── db                     # Managed databases (AWS RDS / GCP Cloud SQL)
│   ├── list               # List databases (--engine filter)
│   ├── get <name>         # Details
│   └── connect <name>     # Port-forward tunnel (AWS: --via <bastion-instance-id>)
├── storage (s3)           # Object storage (S3 / GCS); paths use s3://bucket/key
│   ├── ls [s3://b [pfx]]  # List buckets, or objects under prefix
│   ├── cp <src> <dst>     # Copy one object (local ↔ remote)
│   ├── sync <src> <dst>   # Sync directory (--delete)
│   └── presign <s3://...> # Presigned GET URL
├── k8s                    # Kubernetes cluster management
│   ├── list               # List EKS / GKE clusters in context
│   ├── get <name>         # Cluster details
│   ├── use <name>         # Update kubeconfig + switch kubectl context
│   ├── connect <name>     # AWS: SSM tunnel to context bastion + subshell with HTTPS_PROXY
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
├── profile                # Legacy AWS profile management
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

## UI Patterns

Interactive selectors (bubbletea `tea.Model`) in `internal/ui/` follow a consistent pattern:
search/filter input, cursor navigation, details panel, status bar. Table renderers use Unicode
box-drawing characters from `internal/ui/styles.go`. State indicators: `●` running, `○` stopped,
`◐` pending.

## Version Injection

Version metadata is injected at build time via ldflags into `cmd.Version`, `cmd.Commit`,
`cmd.BuildDate` (see Makefile).
