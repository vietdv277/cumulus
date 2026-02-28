# Cumulus CLI - Multi-Cloud Command Strategy

## Project Overview

**Name:** Cumulus
**Binary:** `cml`
**Language:** Go
**Purpose:** Personal CLI tool to simplify AWS and GCP resource management with interactive selection and sensible defaults.

### Goals

- Unified interface for AWS and GCP resources
- Context-aware commands (set provider once, commands follow)
- Interactive selection with fuzzy search
- Replace complex CLI commands and aliases
- Easy to extend for additional providers (Azure, OCI, etc.)

---

## Command Strategy: Context-Aware

### Core Principle

User sets context once, all subsequent commands operate within that context. No need to specify provider for every command.

### Command Pattern

```
cml <resource> <action> [target] [flags]
```

### Context Commands

```bash
# Set active context
cml use <context-name>

# Show current context and auth status
cml status

# List all configured contexts
cml contexts
```

### Example Workflow

```bash
# Morning: working on AWS production
cml use aws:prod
cml vm list                    # Lists EC2 instances
cml vm connect web-01          # SSM session
cml secrets get /app/db-pass   # SSM Parameter Store

# Afternoon: switch to GCP staging
cml use gcp:staging
cml vm list                    # Lists GCE instances
cml vm connect api-server      # SSH via gcloud
cml secrets get db-password    # GCP Secret Manager
```

### Context Override (When Needed)

```bash
# Temporarily use different context without switching
cml vm list --context aws:dev

# Short form
cml vm list -c gcp:prod
```

---

## Unified Resource Commands

### Resource Mapping

| Command | AWS | GCP | Description |
|---------|-----|-----|-------------|
| `vm` | EC2 | Compute Engine | Virtual machines |
| `db` | RDS | Cloud SQL | Managed databases |
| `db:nosql` | DynamoDB | Firestore | NoSQL databases |
| `storage` | S3 | GCS | Object storage |
| `secrets` | Secrets Manager / SSM Param | Secret Manager | Secrets and config |
| `logs` | CloudWatch Logs | Cloud Logging | Log management |
| `k8s` | EKS | GKE | Kubernetes clusters |
| `functions` | Lambda | Cloud Functions | Serverless functions |
| `dns` | Route53 | Cloud DNS | DNS management |

### Command Reference

#### VM Management

```bash
cml vm list [flags]
  -i, --interactive       Interactive selection mode
  -s, --state <state>     Filter by state (running, stopped)
  -t, --tag <key=value>   Filter by tag
  -o, --output <format>   Output format (table, json, yaml)

cml vm get <name>
cml vm start <name>
cml vm stop <name>
cml vm reboot <name>
cml vm connect <name>
cml vm tunnel <name> <remote-port> [local-port]
cml vm tunnel <name> <remote-host> <remote-port> [local-port]
```

#### Database Management

```bash
cml db list
cml db get <name>
cml db connect <name> [flags]
  --via <bastion>         Connect via bastion/jump host
  --local-port <port>     Local port for tunnel
```

#### Secrets Management

```bash
cml secrets list [prefix]
cml secrets get <name> [flags]
  --decode                Base64 decode value
  --copy                  Copy to clipboard

cml secrets set <name> <value>
cml secrets delete <name>
```

#### Storage Management

```bash
cml storage ls [bucket] [prefix]
cml storage cp <source> <destination>
cml storage sync <source> <destination>
cml storage presign <path> [flags]
  --expires <duration>    URL expiration (default: 1h)
```

#### Logs Management

```bash
cml logs <target> [flags]
  -f, --follow            Tail mode
  -s, --since <duration>  Time range (e.g., 1h, 30m)
  --filter <pattern>      Filter pattern
```

#### Kubernetes Management

```bash
cml k8s list
cml k8s use <cluster>           # Updates kubeconfig
cml k8s contexts                # List k8s contexts
```

---

## Provider-Specific Commands

For features without cross-provider equivalents, use provider namespace:

### AWS-Specific

```bash
cml aws ssm param list [prefix]
cml aws ssm param get <name>
cml aws ssm param set <name> <value>
cml aws iam whoami
cml aws sts assume-role <role-arn>
```

### GCP-Specific

```bash
cml gcp projects list
cml gcp projects use <project>
cml gcp iap tunnel <instance> <port>
cml gcp iam test-permissions <resource>
```

---

## Configuration

### Config File Location

```bash
~/.cml.yaml
```

### Config Structure

```yaml
# Current active context
current_context: aws:prod

# Context definitions
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: ap-southeast-1
    
  aws:dev:
    provider: aws
    profile: dev-sso
    region: ap-southeast-1
    
  gcp:prod:
    provider: gcp
    project: mycompany-prod
    region: asia-southeast1
    
  gcp:staging:
    provider: gcp
    project: mycompany-staging
    region: asia-southeast1

# Resource aliases for quick access
aliases:
  bastion: aws:prod:i-0abc123def456
  jump: gcp:prod:jump-server-01
  
# Saved tunnel configurations
tunnels:
  prod-db:
    context: aws:prod
    bastion: bastion
    remote_host: prod-db.abc123.ap-southeast-1.rds.amazonaws.com
    remote_port: 3306
    local_port: 3306
    
  staging-redis:
    context: gcp:staging
    target: redis-proxy-01
    remote_port: 6379
    local_port: 6379

# Default settings
defaults:
  output: table
  interactive: false
  region_fallback: ap-southeast-1
```

### Alias Usage

```bash
# Using saved aliases
cml connect bastion           # Quick connect
cml tunnel prod-db            # Saved tunnel config
```

---

## Project Structure

```bash
cumulus/
├── cmd/
│   ├── root.go                   # Root command, global flags
│   ├── use.go                    # Context switching
│   ├── status.go                 # Status display
│   ├── contexts.go               # List contexts
│   │
│   ├── vm.go                     # Unified VM commands
│   ├── db.go                     # Unified DB commands
│   ├── secrets.go                # Unified secrets commands
│   ├── storage.go                # Unified storage commands
│   ├── logs.go                   # Unified logs commands
│   ├── k8s.go                    # Unified k8s commands
│   │
│   ├── aws/                      # AWS-specific commands
│   │   ├── aws.go                # AWS subcommand root
│   │   ├── ssm.go
│   │   └── iam.go
│   │
│   └── gcp/                      # GCP-specific commands
│       ├── gcp.go                # GCP subcommand root
│       ├── iap.go
│       └── projects.go
│
├── internal/
│   ├── config/
│   │   ├── config.go             # Config loading/saving
│   │   ├── context.go            # Context management
│   │   └── aliases.go            # Alias resolution
│   │
│   ├── provider/
│   │   ├── provider.go           # Provider interface definitions
│   │   ├── registry.go           # Provider registry
│   │   ├── errors.go             # Common errors
│   │   │
│   │   ├── aws/
│   │   │   ├── client.go         # AWS client initialization
│   │   │   ├── vm.go             # EC2 implementation
│   │   │   ├── db.go             # RDS implementation
│   │   │   ├── secrets.go        # Secrets Manager / SSM
│   │   │   ├── storage.go        # S3 implementation
│   │   │   ├── logs.go           # CloudWatch implementation
│   │   │   └── k8s.go            # EKS implementation
│   │   │
│   │   └── gcp/
│   │       ├── client.go         # GCP client initialization
│   │       ├── vm.go             # GCE implementation
│   │       ├── db.go             # Cloud SQL implementation
│   │       ├── secrets.go        # Secret Manager
│   │       ├── storage.go        # GCS implementation
│   │       ├── logs.go           # Cloud Logging
│   │       └── k8s.go            # GKE implementation
│   │
│   └── ui/
│       ├── table.go              # Table rendering
│       ├── prompt.go             # Interactive prompts
│       ├── spinner.go            # Progress indicators
│       └── colors.go             # Color output
│
├── pkg/
│   └── models/
│       ├── vm.go                 # Unified VM model
│       ├── db.go                 # Unified DB model
│       ├── secret.go             # Unified secret model
│       ├── bucket.go             # Unified storage model
│       └── cluster.go            # Unified k8s model
│
├── main.go
├── go.mod
└── go.sum
```

---

## Provider Interface Design

### Core Interfaces

```bash
Provider (main interface)
├── Name() string
├── VM() VMProvider
├── DB() DBProvider
├── Secrets() SecretsProvider
├── Storage() StorageProvider
├── Logs() LogsProvider
└── K8s() K8sProvider
```

### VMProvider Interface

```bash
VMProvider
├── List(ctx, filters) -> []VM, error
├── Get(ctx, nameOrID) -> *VM, error
├── Start(ctx, nameOrID) -> error
├── Stop(ctx, nameOrID) -> error
├── Reboot(ctx, nameOrID) -> error
├── Connect(ctx, nameOrID) -> error
└── Tunnel(ctx, nameOrID, opts) -> error
```

### DBProvider Interface

```bash
DBProvider
├── List(ctx) -> []Database, error
├── Get(ctx, nameOrID) -> *Database, error
└── Connect(ctx, nameOrID, opts) -> error
```

### SecretsProvider Interface

```bash
SecretsProvider
├── List(ctx, prefix) -> []Secret, error
├── Get(ctx, name) -> *SecretValue, error
├── Set(ctx, name, value) -> error
└── Delete(ctx, name) -> error
```

### StorageProvider Interface

```bash
StorageProvider
├── ListBuckets(ctx) -> []Bucket, error
├── ListObjects(ctx, bucket, prefix) -> []Object, error
├── Copy(ctx, src, dst) -> error
├── Sync(ctx, src, dst, opts) -> error
└── Presign(ctx, path, expiry) -> string, error
```

### Feature Support Matrix

Handle features not available in all providers:

| Method | AWS | GCP | Missing Behavior |
|--------|-----|-----|------------------|
| VM.Connect | SSM Session | gcloud SSH | - |
| VM.Tunnel | SSM Port Forward | IAP Tunnel | - |
| Secrets.List | ✓ | ✓ | - |
| DB.Connect | Via bastion | Via Cloud SQL Proxy | - |

For unsupported features, return `ErrNotSupported` with helpful message.

---

## Unified Models

### VM Model

```bash
VM
├── ID          string              # Provider-specific ID
├── Name        string              # Name tag or instance name
├── State       VMState             # running, stopped, pending
├── PrivateIP   string
├── PublicIP    string
├── Type        string              # t3.micro, e2-medium
├── Zone        string              # Availability zone
├── Tags        map[string]string
├── LaunchedAt  time.Time
├── Provider    string              # aws, gcp
└── Raw         interface{}         # Original API response
```

### Database Model

```bash
Database
├── ID          string
├── Name        string
├── Engine      string              # mysql, postgres, etc.
├── Version     string
├── Endpoint    string
├── Port        int
├── State       string
├── Provider    string
└── Raw         interface{}
```

### Secret Model

```bash
Secret
├── Name        string
├── ARN         string              # Provider-specific identifier
├── CreatedAt   time.Time
├── UpdatedAt   time.Time
├── Provider    string
└── Raw         interface{}

SecretValue
├── Secret
├── Value       string
└── Version     string
```

---

## Development Phases

### Phase 1: Foundation (Week 1-2)

- [x] Project setup (go mod, structure)
- [x] Cobra + Viper integration
- [x] Config file handling
- [x] Context management (use, status, contexts)
- [x] AWS authentication (SSO, profile)
- [ ] GCP authentication (ADC)
- [x] Basic output formatting

### Phase 2: VM Management (Week 3-4)

- [x] Provider interfaces (VMProvider)
- [x] AWS EC2 implementation
- [ ] GCP GCE implementation
- [x] `vm list` with table output
- [ ] `vm list -i` interactive mode
- [x] `vm connect` (SSM / gcloud SSH) - SSM done
- [x] `vm tunnel` (port forwarding)
- [x] `vm start/stop/reboot`

### Phase 3: Database & Secrets (Week 5-8)

- [ ] `db list` and `db connect`
- [ ] RDS / Cloud SQL implementations
- [x] `secrets list/get/set`
- [x] SSM Parameter Store implementation
- [ ] GCP Secret Manager implementation
- [x] AWS Secrets Manager implementation

### Phase 4: Storage & Logs (Week 9-12)

- [ ] `storage ls/cp/sync`
- [ ] S3 / GCS implementations
- [ ] `logs` command with tail mode
- [ ] CloudWatch / Cloud Logging implementations

### Phase 5: Kubernetes (Week 13-14)

- [ ] `k8s list` and `k8s use`
- [ ] EKS / GKE implementations
- [ ] Kubeconfig management

### Phase 6: Polish (Week 15-16)

- [ ] Shell completions (zsh, bash, fish)
- [ ] `--output json/yaml/table` flag
- [ ] Alias system
- [ ] `doctor` command
- [ ] Error handling improvements
- [ ] Documentation

---

## Dependencies

| Purpose | Package |
|---------|---------|
| CLI framework | `github.com/spf13/cobra` |
| Config management | `github.com/spf13/viper` |
| AWS SDK | `github.com/aws/aws-sdk-go-v2` |
| GCP SDK | `cloud.google.com/go` |
| Interactive prompts | `github.com/manifoldco/promptui` |
| Fuzzy finder | `github.com/ktr0731/go-fuzzyfinder` |
| Table output | `github.com/olekukonko/tablewriter` |
| Spinner | `github.com/briandowns/spinner` |
| Colors | `github.com/fatih/color` |

---

## Adding New Providers (Future)

To add a new provider (e.g., Azure):

1. Add provider config type in `internal/config/`
2. Create `internal/provider/azure/` directory
3. Implement all required interfaces
4. Register provider in `internal/provider/registry.go`
5. Add Azure-specific commands in `cmd/azure/`

No changes required to unified commands or models.

---

## Error Handling Strategy

| Error Type | Handling |
|------------|----------|
| Auth failure | Clear message with fix instructions |
| Resource not found | Suggest similar names |
| Permission denied | Show required permissions |
| Network error | Retry with backoff |
| Unsupported feature | Return `ErrNotSupported` with alternative |

---

## Output Formats

All list commands support:

```bash
--output table    # Default, human-readable
--output json     # Machine-readable
--output yaml     # Machine-readable
--output wide     # Extended table with more columns
```

---

## Interactive Mode

Commands with `-i` or `--interactive` flag enable:

- Fuzzy search through results
- Multi-select where applicable
- Preview pane with details
- Keyboard navigation

Example:

```bash
cml vm list -i
# Opens fuzzy finder, select VM, then choose action
```
