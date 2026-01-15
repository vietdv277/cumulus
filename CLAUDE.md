# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Cumulus (`cml`) is a multi-cloud CLI wrapper for AWS and GCP that simplifies complex cloud commands into intuitive, discoverable interfaces. The goal is to replace verbose cloud CLI commands with simple alternatives (e.g., `cml ec2 ls` instead of long `aws ec2 describe-instances` commands).

## Build and Run Commands

```bash
# Build the CLI
go build -o cml .

# Run directly
go run .

# Run with commands
go run . ec2 ls
go run . ec2 ls --all
go run . ec2 ls --name <pattern>
```

## Architecture

### Layer Structure

```
main.go           → Entry point, calls cmd.Execute()
cmd/              → Cobra command definitions (root.go, ec2.go)
internal/aws/     → AWS SDK wrappers (client.go, ec2.go)
pkg/types/        → Shared types used across cloud providers
```

### Key Patterns

**Cobra Command Structure**: Commands are defined in `cmd/` with the pattern:
- `rootCmd` in `root.go` with global flags (`--profile`, `--region`)
- Subcommands (e.g., `ec2Cmd`, `ec2LsCmd`) added via `rootCmd.AddCommand()`

**AWS Client with Options Pattern**: The AWS client (`internal/aws/client.go`) uses functional options:
```go
client, err := aws.NewClient(ctx, region, aws.WithProfile(profile))
```

**Cloud-Agnostic Types**: `pkg/types/Instance` is a unified struct for both AWS EC2 and GCP GCE instances, with a `Cloud` field to distinguish providers.

### Configuration Priority

1. CLI flags (`--profile`, `--region`)
2. Environment variables with `CML_` prefix
3. Standard AWS environment variables (`AWS_PROFILE`, `AWS_REGION`, `AWS_DEFAULT_REGION`)

## Key Libraries

- CLI: `github.com/spf13/cobra` + `github.com/spf13/viper`
- AWS: `github.com/aws/aws-sdk-go-v2`
- Output: `github.com/charmbracelet/lipgloss`, `github.com/fatih/color`
