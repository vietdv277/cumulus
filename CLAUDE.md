# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Cumulus (`cml`) is a personal CLI tool for AWS (and future GCP) resource management, written in Go.

## Module Path

`github.com/vietdv277/cumulus`

All imports must use this full path.

## Tech Stack

- Language: Go 1.24+
- CLI Framework: Cobra + Viper
- AWS SDK: `aws-sdk-go-v2`
- TUI: charmbracelet/bubbletea (interactive selectors) + charmbracelet/lipgloss (styling)

## Build & Test

```bash
# Build (injects version/commit/date via ldflags)
make build

# Run without building
make run ARGS="ec2 ls"

# Test
make test

# Test with coverage report
make test-cover

# Format, vet, test, and build
make all

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

Config lives at `~/.config/cml/config.yaml`. A context encodes provider + named environment:

```yaml
current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
```

Use `ParseContextName("aws:prod")` → `{provider: "aws", name: "prod"}`.
`MigrateFromOldConfig()` handles migration from the legacy `~/.cml/config.yaml`.

### Provider Interface (`pkg/provider/interfaces.go`)

All cloud resources are abstracted behind interfaces: `VMProvider`, `SecretsProvider`,
`DBProvider`, `StorageProvider`, `LogsProvider`, `K8sProvider`. The `CloudProvider`
interface aggregates them all. AWS implementations live in `internal/aws/`; GCP is planned.

### AWS Client (`internal/aws/client.go`)

Uses the functional options pattern:

```go
client, err := aws.NewClient(ctx, aws.WithProfile("prod"), aws.WithRegion("us-east-1"))
```

The `Client` struct holds sub-clients for EC2, SSM, STS, Auto Scaling, ELBv2, and Secrets Manager.

### Secrets Routing (`internal/aws/secrets.go`)

Names starting with `/` route to SSM Parameter Store; others route to AWS Secrets Manager.
Both are aggregated by `List()`.

## Full Command Tree

```text
cml
├── vm                     # Context-aware unified VM management
│   ├── list               # List VMs (--state, --name, --tag)
│   ├── get <id|name>      # Get VM details
│   ├── connect <id|name>  # SSH/SSM session
│   ├── tunnel <id> ...    # Port forwarding via SSM
│   ├── start/stop/reboot  # Lifecycle
├── secrets                # Unified secrets (SSM + Secrets Manager)
│   ├── list               # List all secrets from both sources
│   ├── get <name>         # Get secret value
│   ├── set <name> <val>   # Create/update secret
│   └── delete <name>      # Delete secret
├── use                    # Context management
│   ├── <context>          # Switch active context
│   ├── add <context>      # Add context (--profile, --region)
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
