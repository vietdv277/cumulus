# Cumulus CLI - Multi-Cloud Command Strategy

## Project Overview

**Name:** Cumulus
**Binary:** `cml`
**Language:** Go
**Purpose:** Personal CLI tool to simplify AWS and GCP resource management with interactive selection and sensible defaults.

### Goals

- Unified interface for AWS and GCP resources
- Context-aware commands (set provider once, commands follow)
- Interactive selection with search-as-you-type
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

# Interactive context switcher (like kubectx)
cml ctx -i
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

| Command | AWS | GCP |
|---------|-----|-----|
| `vm` | EC2 | Compute Engine |
| `db` | RDS | Cloud SQL |
| `secrets` | Secrets Manager + SSM Parameter Store | Secret Manager |
| `storage` (alias `s3`) | S3 | Cloud Storage |
| `logs` (alias `log`) | CloudWatch Logs | Cloud Logging |
| `k8s` | EKS | GKE |

NoSQL, serverless functions and DNS are not built; see the backlog under Development Phases.

### Command Reference

Context-aware commands (`vm`, `db`, `secrets`, `storage`, `logs`, `k8s`, `aws`) take
`-c/--context <name>` (aliases work) and reject the legacy global `-p/--profile` and
`-r/--region`. Data-printing commands take the global `-o/--output table|json|yaml`.

#### VM Management

```bash
cml vm list [flags]                          # alias: ls
  -i, --interactive       Interactive selector (Enter: connect · Ctrl+S: start · Ctrl+X: stop · Esc: quit)
  -s, --state <state>     running (default), stopped, all
      --name <pattern>    Filter by name pattern
  -t, --tag <key=value>   Filter by tag (repeatable)
cml vm get <name-or-id>
cml vm connect <name-or-id>                  # SSM session (AWS) / gcloud compute ssh (GCP)
cml vm tunnel <name-or-id> <remote-port> [local-port]
      --save <name>       Save as a named tunnel (--force overwrites)
cml vm start <name-or-id>
cml vm stop <name-or-id>                     # confirms; -y/--yes skips
cml vm reboot <name-or-id>                   # confirms; -y/--yes skips
```

#### Saved Tunnels

```bash
cml tunnel list
cml tunnel start <name> [--local-port <port>]   # always runs in the tunnel's saved context
cml tunnel rm <name>
```

#### Database Management

```bash
cml db list [--engine <engine>]              # alias: ls
cml db get <name-or-id>
cml db connect <name-or-id> [flags]          # AWS: SSM port-forward via a bastion; GCP: cloud-sql-proxy
      --via <instance-id>   AWS bastion (defaults to the context's bastion)
      --local-port <port>   Local port (defaults to the remote port)
```

#### Secrets Management

```bash
cml secrets list [prefix] [--ssm-only | --sm-only]   # backend filters: AWS only
cml secrets get <name>                               # ssm:<name> / sm:<name> force the AWS backend
cml secrets set <name> <value>
cml secrets delete <name> [flags]                    # confirms
      --force    Secrets Manager: no 7-day recovery window
  -y, --yes      Skip the confirmation prompt
```

#### Storage Management

```bash
cml storage ls [s3://bucket|gs://bucket [prefix]]
cml storage cp <src> <dst>                           # local ↔ remote, remote → remote
cml storage sync <src> <dst> [--delete] [--dry-run] [-y]   # --delete confirms
cml storage presign <s3://bucket/key|gs://bucket/key> [--expires <seconds>]   # default 3600, max 7 days
```

#### Logs Management

```bash
cml logs <target> [flags]                    # alias: log
      --since <duration>  How far back to start (default 15m)
      --filter <expr>     CloudWatch filter pattern (AWS) / Logging query (GCP)
      --limit <n>         Max entries, query only (default 100)
  -f, --follow            Tail until Ctrl-C
cml logs list [prefix]                       # log groups (AWS) / log names (GCP)
```

#### Kubernetes Management

```bash
cml k8s list [-i] [--name <substring>]       # alias: ls
cml k8s get <name>
cml k8s use <name>                           # updates kubeconfig via aws/gcloud, switches kubectl context
cml k8s connect <cluster> [--bastion <instance-id>] [--local-port <port>]   # AWS only
cml k8s contexts
```

#### Contexts and Setup

```bash
cml use <context>                            # switch (aliases work)
cml use add <context> [--profile <p> | --project <p>] [--region <r>] [--bastion ...]
cml use update <context> [flags]
cml use delete <context>                     # alias: rm
cml contexts [-i]                            # alias: ctx
cml status
cml alias list | add <name> <context> [--force] | rm <name>
cml doctor [--online]
```

---

## Provider-Specific Commands

For features without a cross-provider equivalent, use the provider namespace.

### AWS-Specific

```bash
cml aws ssm param list [prefix]
cml aws ssm param get <name> [--decode]
cml aws ssm param set <name> <value> [--secure]
cml aws iam whoami
```

### GCP-Specific

None yet; the planned `cml gcp` commands are in the backlog.

---

## Configuration

### Config File Location

```bash
~/.config/cml/config.yaml   # XDG-compliant (respects $XDG_CONFIG_HOME)
```

Legacy paths (`~/.cml.yaml`, `~/.cml/config.yaml`, `~/Library/Application Support/cml/config.yaml`)
are auto-migrated on first run. The file is written atomically with mode 0600 in a 0700 directory.

### Config Structure

```yaml
# ~/.config/cml/config.yaml
current_context: aws:prod

contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: ap-southeast-1
    bastion: i-0abc123def456       # optional; used by db connect and k8s connect
    bastion_port: 8888             # optional; k8s connect proxy port (default 8888)
  gcp:staging:
    provider: gcp
    project: example-staging
    region: asia-southeast1        # a region or a zone
    bastion: jump-server-01        # optional IAP/SSH bastion
    bastion_project: example-infra # defaults to project
    bastion_zone: asia-southeast1-b
    bastion_iap: true

# Short names for contexts
aliases:
  prod: aws:prod
  stg: gcp:staging

# Saved tunnels (cml vm tunnel --save, cml tunnel list/start/rm)
tunnels:
  prod-db:
    context: aws:prod
    target: i-0abc123def456
    remote_port: 5432
    local_port: 5432

defaults:
  output: table                    # table | json | yaml; the --output flag wins
```

A saved tunnel may also carry `bastion` / `remote_host`; `cml tunnel start` rejects those as not
supported yet.

### Alias Usage

```bash
cml use prod                  # switch to aws:prod
cml vm list -c stg            # one-off command in gcp:staging
cml tunnel start prod-db      # saved tunnel, in its own context
```

---

## Project Structure

```bash
cumulus/
├── main.go
├── cmd/                  # Cobra commands: context-aware (vm, db, secrets, storage, logs, k8s,
│   │                     #   tunnel, use, contexts, status, alias, doctor) and legacy (ec2,
│   │                     #   profile, asg, lb, vpc); output.go, confirm.go, context_resolve.go
│   └── aws/              # cml aws ssm / iam
├── internal/
│   ├── aws/              # AWS providers (vm, db, secrets, storage, logs, k8s) + legacy helpers
│   ├── gcp/              # GCP providers (vm, db, secrets, storage, logs, k8s), V4 signing
│   ├── config/           # config file, contexts, aliases, tunnels, migrations
│   ├── kubeconfig/       # read-only kubeconfig reader
│   ├── output/           # table/json/yaml rendering
│   ├── errhint/          # recovery hints for auth/permission errors
│   ├── suggest/          # "did you mean" suggestions
│   └── ui/               # bubbletea selectors, tables, styles
├── pkg/
│   ├── provider/         # provider interfaces
│   └── types/            # shared domain types
└── docs/
```

---

## Provider Interface Design

Interfaces live in `pkg/provider/interfaces.go`; `CloudProvider` aggregates them
(`VM()`, `Secrets()`, `DB()`, `Storage()`, `Logs()`, `K8s()`). Commands build the provider for the
resolved context with a `switch` on `provider` (`aws` / `gcp`) in each `cmd/<resource>.go`.

| Interface | Methods |
|-----------|---------|
| `VMProvider` | `List`, `Get`, `Start`, `Stop`, `Reboot`, `Connect`, `Tunnel` |
| `DBProvider` | `List`, `Get`, `Connect` |
| `SecretsProvider` | `List`, `Get`, `Set`, `Delete` |
| `StorageProvider` | `ListBuckets`, `ListObjects`, `Copy`, `Sync`, `Presign` |
| `LogsProvider` | `ListTargets`, `Query`, `Tail` |
| `K8sProvider` | `ListClusters`, `GetCluster`, `UpdateKubeconfig` |

### Feature Support Matrix

| Method | AWS | GCP | Missing Behavior |
|--------|-----|-----|------------------|
| VM.Connect | SSM session | `gcloud compute ssh` (optional IAP bastion) | - |
| VM.Tunnel | SSM port forward | IAP tunnel | - |
| DB.Connect | SSM port forward via bastion | `cloud-sql-proxy` | `--via` on GCP is an error |
| Storage.Presign | SigV4 | V4 signed URL | GCP needs service-account key credentials |
| Secrets.Delete | 7-day recovery (`--force` immediate) | Permanent | Prompt states it is permanent |
| Logs.Tail | Polls `FilterLogEvents` | Polls `entries.list` | - |
| `k8s connect` | SSM tunnel + `HTTPS_PROXY` subshell | - | Error: AWS-only for now |

Backend-specific options on the wrong provider (`--ssm-only`/`--sm-only`, `ssm:`/`sm:` prefixes,
`--via` on GCP) return a clear error.

---

## Unified Models

Shared types live in `pkg/types/`: `VM`/`VMState`, `Database`, `Secret`/`SecretValue`,
`Bucket`/`Object`, `LogTarget`/`LogEntry`, `K8sCluster`, plus legacy AWS types (`Instance`,
`AutoScalingGroup`, `LoadBalancer`, `VPC`, `Subnet`, `AWSProfile`). Their json/yaml tags are the
`--output json|yaml` schema.

---

## Development Phases

### Phase 1: Foundation — complete

- [x] Project setup (go mod, structure)
- [x] Cobra + Viper integration
- [x] Config file handling
- [x] Context management (use, status, contexts)
- [x] AWS authentication (SSO, profile)
- [x] GCP authentication (ADC)
- [x] Basic output formatting

### Phase 2: VM Management — complete

- [x] Provider interfaces (VMProvider)
- [x] AWS EC2 implementation
- [x] GCP GCE implementation
- [x] `vm list` with table output
- [x] `vm list -i` interactive TUI mode (bubbletea — connect/start/stop)
- [x] `vm connect` (SSM for AWS, gcloud SSH + IAP for GCP)
- [x] `vm tunnel` (port forwarding)
- [x] `vm start/stop/reboot`

### Phase 3: Database & Secrets — complete

- [x] `db list`, `db get`, and `db connect` (AWS)
- [x] RDS implementation (list/get + SSM port-forward via bastion for connect)
- [x] Cloud SQL implementation (list/get + `cloud-sql-proxy` for connect)
- [x] `secrets list/get/set`
- [x] SSM Parameter Store implementation
- [x] AWS Secrets Manager implementation
- [x] GCP Secret Manager implementation

### Phase 4: Storage & Logs — complete

- [x] `storage ls/cp/sync/presign` (AWS)
- [x] S3 implementation
- [x] GCS implementation (`gs://` paths; sync via `gcloud storage rsync`)
- [x] `logs` command with tail mode (`cml logs`, alias `log`; `logs list`)
- [x] CloudWatch / Cloud Logging implementations

### Phase 5: Kubernetes — complete

- [x] `k8s list`, `k8s get`, `k8s use`, `k8s contexts`
- [x] EKS / GKE implementations
- [x] Kubeconfig management (shells out to `aws eks update-kubeconfig` / `gcloud container clusters get-credentials`)

### Phase 6: Polish — complete

- [x] Shell completions (zsh, bash, fish) — provided by Cobra's built-in `cml completion`
- [x] `--output json/yaml/table` flag
- [x] Alias resolution in config layer
- [x] User-facing alias management commands (`cml alias list/add/rm`)
- [x] `doctor` command
- [x] Saved tunnels (`vm tunnel --save`, `cml tunnel list/start/rm`)
- [x] `--context` on `cml aws ssm/iam`

#### v0.11.1 (findings from the v0.11.0 install check)

- [x] Tighten an existing config directory to 0700 on save; `os.MkdirAll` leaves a pre-existing 0755
  `~/.config/cml` as is, so the `cml doctor` permissions hint never clears the failure
- [x] Stop printing usage text on runtime errors (a failed `cml doctor` prints it today); keep it for
  flag and argument errors
- [x] Strip release binaries with `-s -w` in `release.yml`, matching `make build`

#### Error handling — v0.12.0

- [x] Auth failures: detect an expired SSO session or missing ADC on every command and print the fix
  (`aws sso login --profile <p>`, `gcloud auth application-default login`) — `internal/errhint`
- [x] Not found: suggest close matches for contexts, aliases, saved tunnels and VM names —
  `internal/suggest`
- [x] Permission denied: name the denied action from the API error
- [x] Network errors: document the SDK retry defaults instead of adding a second retry layer. The AWS
  standard retryer and Compute Engine reads retry; the `google.golang.org/api` REST clients do not
  (see the backlog)

#### Documentation — after v0.12.0

- [x] Align this document with the code: Command Reference (flags from `--help`), Configuration,
  Project Structure, Provider Interface Design, Unified Models, Dependencies, Adding New Providers,
  Output Formats, Interactive Mode
- [x] Move unbuilt spec items out of the reference sections into the backlog below
- [x] `defaults.interactive` and `defaults.region_fallback`: dropped from the config schema (never
  read; existing configs still load)
- [x] Verify GCP parity against a real project (secrets, `db list/connect`, `gs://` storage and
  presign, logs) and record the results — `docs/gcp-verification.md`: 13 ok, 2 expected errors
  (permission, user-ADC presign), `db connect` not run, `logs -f` failed on a 429 (fixed: the GCP tail now backs off)

### Backlog — unscheduled

- `cml doctor --context <name>` to check a context other than the current one
- Retry idempotent GCP REST reads (Secret Manager, Cloud SQL Admin, Cloud Logging, GKE, Cloud
  Storage listing) on 429/5xx; these clients have no built-in retry
- `--output wide`
- Saved tunnels through a bastion / to a remote host (`bastion` / `remote_host` are stored but
  `cml tunnel start` rejects them)
- `secrets get --decode/--copy` (only `aws ssm param get --decode` exists)
- `cml aws sts assume-role`
- `cml gcp projects list/use`, `cml gcp iap tunnel`, `cml gcp iam test-permissions`
- `db:nosql` (DynamoDB / Firestore), `functions` (Lambda / Cloud Functions), `dns` (Route53 / Cloud DNS)
- Retire the legacy layer (`ec2`, `profile`, `asg`, `lb`, `vpc`) in favour of the context-aware commands

---

## Dependencies

| Purpose | Package |
|---------|---------|
| CLI framework | `github.com/spf13/cobra` |
| Config management | `github.com/spf13/viper`, `gopkg.in/yaml.v3` |
| AWS SDK | `github.com/aws/aws-sdk-go-v2` (EC2, Auto Scaling, ELBv2, RDS, S3, EKS, SSM, Secrets Manager, CloudWatch Logs, STS) |
| GCP SDK | `cloud.google.com/go/compute` (VMs), `google.golang.org/api` (Secret Manager, Cloud SQL Admin, Cloud Storage, Cloud Logging, GKE), `golang.org/x/oauth2` |
| TUI framework | `github.com/charmbracelet/bubbletea` |
| TUI styling | `github.com/charmbracelet/lipgloss` |
| Unicode display width | `github.com/mattn/go-runewidth` |

External tools: `session-manager-plugin` (AWS SSM sessions), `gcloud` (GCP SSH, IAP, storage
sync, kubeconfig), `cloud-sql-proxy` (GCP `db connect`), `aws` (EKS kubeconfig).

---

## Adding New Providers (Future)

To add a new provider (e.g., Azure):

1. Accept the provider name in `cml use add` (`cmd/use.go`) and add any config fields to `Context`
   in `internal/config/context.go`
2. Implement the `pkg/provider` interfaces in `internal/azure/`
3. Add a `case "azure"` to the provider `switch` in each `cmd/<resource>.go`
4. Add its tool and credential checks to `cml doctor` and its error classes to `internal/errhint`

Unified commands, output rendering and the shared types need no changes.

---

## Error Handling Strategy

| Error Type | Handling |
|------------|----------|
| Auth failure | `Hint:` with the login command (`aws sso login --profile <p>`, `gcloud auth application-default login`) |
| Resource not found | "did you mean" suggestions for contexts, aliases, saved tunnels and VM names |
| Permission denied | `Hint:` naming the denied IAM action / GCP permission when the API reports it |
| Network error | SDK retries: AWS standard retryer (3 attempts, backoff ≤ 20s); Compute Engine reads on 503/504; other GCP REST clients do not retry |
| Unsupported feature | Clear "not supported for GCP" error with alternative |

---

## Output Formats

Data-printing commands take the global `-o/--output`; `defaults.output` sets the default and the
flag wins.

```bash
--output table    # Default, human-readable
--output json     # Machine-readable; stdout carries only the document, empty results print []
--output yaml     # Machine-readable
```

`cml logs -f -o json` prints one JSON object per line; `-f` rejects `yaml`. `--output wide` is in
the backlog.

---

## Interactive Mode

Commands with `-i` / `--interactive` open a bubbletea selector with:

- Search/filter by typing (all printable characters go to search)
- Arrow key navigation with scrolling
- Details panel for the highlighted item
- Status bar with count and key hints

Esc, `q` or Ctrl+C cancels with `selection cancelled` (exit 0). `-i` cannot be combined with
`-o json|yaml`.

### Key Bindings

| Command | Key | Action |
|---------|-----|--------|
| All | `↑` / `↓` | Navigate list |
| All | Type | Filter results |
| All | `Backspace` | Delete last search character |
| All | `Esc` / `Ctrl+C` | Quit without action |
| `vm list -i` | `Enter` | Connect (SSM on AWS, `gcloud compute ssh` on GCP) |
| `vm list -i` | `Ctrl+S` | Start VM |
| `vm list -i` | `Ctrl+X` | Stop VM (confirms) |
| `k8s list -i` | `Enter` | `k8s use` the selected cluster |
| `ctx -i` | `Enter` | Switch to selected context |

### Implemented

```bash
cml vm list -i     # Search VMs, then connect / start / stop
cml k8s list -i    # Search clusters, then switch kubeconfig to one
cml ctx -i         # Search contexts and switch (like kubectx)
```

Legacy commands open selectors when the target is omitted: `cml profile`, `cml ec2 ssh`,
`cml vpc describe|subnets`, `cml lb describe|targets`, `cml asg describe`.
