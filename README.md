# Cumulus (cml)

A fast, intuitive CLI for multi-cloud resource management (AWS + GCP). Manage EC2 and GCE instances, secrets, and more through a unified, context-aware interface with interactive selectors and clean output.

## Why Cumulus?

```bash
# Before
aws ec2 describe-instances --filters "Name=instance-state-name,Values=running" \
  --query "Reservations[*].Instances[*].[InstanceId,Tags[?Key=='Name'].Value|[0]]" --output table

# After
cml vm list
```

## Features

- **Multi-cloud** — AWS and GCP in one tool; same commands, same UX
- **Context system** — switch between environments with `cml use <context>`
- **Unified VM management** — list, connect, tunnel, start/stop/reboot across providers
- **Interactive selectors** — pick instances from a filterable TUI without memorizing IDs
- **GCP bastion / IAP** — connect to private GCE instances through a bastion with optional IAP tunneling
- **Unified secrets** — AWS SSM Parameter Store, Secrets Manager, and GCP Secret Manager behind one command
- **Beautiful output** — styled tables with color-coded state indicators

## Installation

### Prerequisites

- Go 1.25+
- **AWS**: credentials configured (`~/.aws/credentials` or environment variables), [AWS Session Manager plugin](https://docs.aws.amazon.com/systems-manager/latest/userguide/session-manager-working-with-install-plugin.html) for `vm connect`
- **GCP**: `gcloud` CLI installed and authenticated (`gcloud auth application-default login`)

### From source

```bash
git clone https://github.com/vietdv277/cumulus.git
cd cumulus
make install        # builds and copies to $GOPATH/bin
```

Or build manually:

```bash
make build          # produces ./cml
sudo mv cml /usr/local/bin/
```

## Quick start

```bash
# 1. Add contexts for each environment
cml use add aws:prod  --profile prod-sso  --region us-east-1
cml use add gcp:prod  --project my-project --region asia-southeast1

# 2. Switch to a context
cml use aws:prod

# 3. Start using it
cml vm list
cml vm connect web-01
```

## Context management

Contexts store the provider, credentials reference, and region so you never have to repeat them.

```bash
# Switch active context
cml use aws:prod
cml use gcp:staging

# Add a new context
cml use add aws:dev  --profile dev --region eu-west-1
cml use add gcp:dev  --project dev-project --region europe-west1

# Update individual fields without replacing the whole context
cml use update aws:prod --region us-west-2
cml use update gcp:prod --bastion bastion-host --bastion-project infra-proj \
    --bastion-zone asia-southeast1-b --bastion-iap

# Remove the bastion from a context
cml use update gcp:prod --bastion ""

# Delete a context
cml use delete aws:old-env

# List all contexts
cml ctx                   # or: cml contexts
cml ctx -i                # interactive selector

# Show current context and auth status
cml status
```

### Context config

Stored at `~/.config/cml/config.yaml`:

```yaml
current_context: aws:prod
contexts:
  aws:prod:
    provider: aws
    profile: prod-sso
    region: us-east-1
  gcp:prod:
    provider: gcp
    project: my-project
    region: asia-southeast1
    bastion: bastion-host
    bastion_project: infra-project
    bastion_zone: asia-southeast1-b
    bastion_iap: true
```

## VM commands

All `vm` subcommands operate in the current context. Pass `--context <name>` to target a different one without switching.

```bash
# List running VMs (default)
cml vm list
cml vm list -s all              # include stopped
cml vm list -s stopped
cml vm list --name web          # filter by name pattern
cml vm list -t env=prod         # filter by label/tag
cml vm list -i                  # interactive TUI selector

# Get details for a specific VM
cml vm get web-01

# SSH (AWS: SSM session) / gcloud compute ssh (GCP)
cml vm connect web-01

# Port forwarding
cml vm tunnel db-01 5432            # forward local 5432 → remote 5432
cml vm tunnel db-01 5432 15432      # forward local 15432 → remote 5432

# Lifecycle
cml vm start  web-01
cml vm stop   web-01
cml vm reboot web-01
```

### GCP bastion tunneling

When a bastion is configured on a GCP context, `vm connect` and `vm tunnel` automatically route through it:

```bash
cml use update gcp:prod \
  --bastion bastion-host \
  --bastion-project infra-project \
  --bastion-zone asia-southeast1-b \
  --bastion-iap

cml vm connect my-private-instance  # → gcloud ssh to bastion, then ssh <private-ip>
cml vm tunnel  my-private-instance 5432  # → SSH tunnel via bastion
```

## Secrets

```bash
# AWS: aggregates SSM Parameter Store (/prefix) and Secrets Manager
# GCP: Secret Manager

cml secrets list                        # list all secrets
cml secrets list /app/                  # filter by prefix
cml secrets get  /app/db-password       # get value
cml secrets set  /app/db-password s3cr3t  # create or update
cml secrets delete /app/old-param       # delete
```

## AWS-specific commands

```bash
# SSM Parameter Store
cml aws ssm param list
cml aws ssm param get  /my/param
cml aws ssm param set  /my/param value

# IAM identity
cml aws iam whoami
```

## Legacy commands

These predated the context system and are still available:

```bash
cml ec2 ls                          # list EC2 instances
cml ec2 ls --all                    # include stopped
cml ec2 ls --name web --asg my-asg  # filter
cml ec2 ssh                         # interactive SSM session (TUI)

cml asg ls
cml asg describe [name]
cml asg instances [name]
cml asg scale    [name] --desired 3

cml vpc ls
cml vpc describe [id]
cml vpc subnets  [id]

cml lb ls
cml lb describe [name]
cml lb targets  [name]

cml profile ls
cml profile use [name]
cml profile current
```

## Build & test

```bash
make build          # build with version metadata injected
make run ARGS="vm list"
make test
make test-cover     # test + HTML coverage report
make all            # fmt + vet + test + build
make install        # install to $GOPATH/bin
```

## Project structure

```
cumulus/
├── cmd/                    # Cobra command definitions
│   ├── root.go
│   ├── vm.go               # context-aware VM commands
│   ├── secrets.go          # context-aware secrets commands
│   ├── use.go              # context management
│   ├── status.go
│   ├── contexts.go
│   ├── ec2.go              # legacy AWS EC2
│   ├── asg.go
│   ├── vpc.go
│   ├── lb.go
│   └── profile.go
├── internal/
│   ├── aws/                # AWS client and provider implementations
│   ├── gcp/                # GCP client and GCE provider implementations
│   ├── ui/                 # bubbletea TUI components (selectors, tables)
│   └── config/             # context config (load, save, migrate)
├── pkg/
│   ├── provider/           # VMProvider, SecretsProvider interfaces
│   └── types/              # shared domain types (VM, Secret, …)
├── main.go
└── go.mod
```

## Dependencies

| Library | Purpose |
|---------|---------|
| [cobra](https://github.com/spf13/cobra) + [viper](https://github.com/spf13/viper) | CLI framework and config |
| [aws-sdk-go-v2](https://github.com/aws/aws-sdk-go-v2) | AWS SDK |
| [cloud.google.com/go/compute](https://pkg.go.dev/cloud.google.com/go/compute) | GCE SDK |
| [google.golang.org/api](https://pkg.go.dev/google.golang.org/api) | GCP auth and APIs |
| [bubbletea](https://github.com/charmbracelet/bubbletea) | Interactive TUI |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | Styled output |

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Commit your changes (`git commit -m 'feat: add my feature'`)
4. Push and open a Pull Request

## License

MIT License — see [LICENSE](LICENSE) for details.
