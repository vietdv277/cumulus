# Cumulus CLI

## Project Overview
Cumulus (`cml`) is a personal CLI tool to simplify AWS and GCP resource management, written in Go.

## Tech Stack
- Language: Go 1.23+
- CLI Framework: Cobra + Viper
- AWS SDK: aws-sdk-go-v2
- Output: fatih/color + rodaine/table

## Project Structure
```
cumulus/
├── cmd/           # Cobra commands
├── internal/
│   ├── aws/       # AWS client and services
│   ├── gcp/       # GCP client (future)
│   └── ui/        # Interactive UI components
├── pkg/types/     # Shared types
├── main.go
└── go.mod
```

## Module Path
`github.com/vietdv277/cumulus`

All imports must use this full path.

## Conventions
- Use `aws.String()` helper for AWS SDK pointer fields
- Keep commands in `cmd/` package
- Keep cloud provider logic in `internal/<provider>/`
- Shared types go in `pkg/types/`
- Use functional options pattern for client configuration

## Commands Structure
```
cml
├── ec2
│   ├── ls         # List instances
│   └── ssh        # Interactive SSM session (planned)
├── profile        # Switch AWS profile (planned)
├── whoami         # Show current identity (planned)
└── version        # Show version info
```

## Build & Test
```bash
# Build
go build -o cml .

# Run
./cml ec2 ls

# Test
go test -v ./...
```

## Current Focus
Week 1-2: AWS EC2 listing and SSM session management
