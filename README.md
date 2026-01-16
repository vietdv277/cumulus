# Cumulus (cml)

A fast, intuitive CLI for AWS cloud management. Simplify complex AWS commands into memorable, discoverable operations with interactive selectors and beautiful output.

## Why Cumulus?

```bash
# Before (AWS CLI)
aws ec2 describe-instances --filters "Name=instance-state-name,Values=running" \
  --query "Reservations[*].Instances[*].[InstanceId,Tags[?Key=='Name'].Value|[0]]" --output table

# After (Cumulus)
cml ec2 ls
```

## Features

- **Intuitive Commands**: Simple, memorable command structure
- **Interactive Selectors**: No need to memorize IDs - just select from a list
- **Beautiful Output**: Styled tables with color-coded status indicators
- **Profile Support**: Easy switching between AWS profiles and regions
- **Unified Experience**: Consistent UX across all AWS services

## Installation

### From Source

```bash
# Clone the repository
git clone https://github.com/vietdv277/cumulus.git
cd cumulus

# Build
go build -o cml .

# Move to PATH (optional)
sudo mv cml /usr/local/bin/
```

### Prerequisites

- Go 1.23 or later
- AWS CLI configured with credentials
- AWS Session Manager plugin (for `ec2 ssh` command)

## Usage

### Global Flags

```bash
cml [command] -p, --profile string   # AWS profile to use
cml [command] -r, --region string    # AWS region to use
```

### EC2 Commands

```bash
# List running EC2 instances
cml ec2 ls

# List all instances (including stopped)
cml ec2 ls --all

# Filter by name pattern
cml ec2 ls --name web-server

# Filter by Auto Scaling Group
cml ec2 ls --asg my-asg

# Start interactive SSM session
cml ec2 ssh

# SSH with filters
cml ec2 ssh --name api --asg production
```

### Auto Scaling Group Commands

```bash
# List all ASGs
cml asg ls

# Describe an ASG (interactive selector if no name)
cml asg describe [asg-name]

# List instances in an ASG
cml asg instances [asg-name]

# Scale an ASG
cml asg scale [asg-name] --desired 3
```

### VPC Commands

```bash
# List all VPCs
cml vpc ls

# Describe a VPC with subnets (interactive selector if no ID)
cml vpc describe [vpc-id]

# List subnets in a VPC
cml vpc subnets [vpc-id]
```

### Load Balancer Commands

```bash
# List all load balancers (ALB/NLB)
cml lb ls

# Describe a load balancer with listeners and target groups
cml lb describe [name]

# List targets with health status
cml lb targets [name]
```

### Profile Management

```bash
# List available AWS profiles
cml profile ls

# Switch to a different profile
cml profile use [profile-name]

# Show current profile
cml profile current
```

### Other Commands

```bash
# Show version
cml version

# Generate shell completion
cml completion bash    # or zsh, fish, powershell
```

## Examples

```bash
# Use production profile in us-west-2
cml ec2 ls -p production -r us-west-2

# Quick SSH to a web server
cml ec2 ssh --name web

# Check load balancer health
cml lb targets my-api-lb

# View VPC networking
cml vpc describe vpc-12345678
```

## Build & Test

### Build

```bash
# Build binary
go build -o cml .

# Build with version info
go build -ldflags "-X main.version=1.0.0" -o cml .
```

### Test

```bash
# Run all tests
go test -v ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test -v ./internal/aws/...
```

### Development

```bash
# Download dependencies
go mod download

# Tidy dependencies
go mod tidy

# Run linter (if installed)
golangci-lint run

# Build and run
go build -o cml . && ./cml --help
```

## Project Structure

```
cumulus/
├── cmd/                    # Cobra command definitions
│   ├── root.go
│   ├── ec2.go
│   ├── asg.go
│   ├── vpc.go
│   ├── lb.go
│   └── profile.go
├── internal/
│   ├── aws/                # AWS SDK client and services
│   │   ├── client.go
│   │   ├── ec2.go
│   │   ├── asg.go
│   │   ├── vpc.go
│   │   └── lb.go
│   ├── ui/                 # Interactive UI components
│   │   ├── selector.go
│   │   ├── table.go
│   │   └── styles.go
│   └── config/             # Configuration management
├── pkg/types/              # Shared type definitions
├── main.go
└── go.mod
```

## Configuration

Cumulus uses your existing AWS CLI configuration:

- `~/.aws/credentials` - AWS credentials
- `~/.aws/config` - AWS profiles and regions

### Environment Variables

```bash
AWS_PROFILE=production     # Default AWS profile
AWS_REGION=us-east-1       # Default AWS region
```

## Dependencies

| Library | Purpose |
|---------|---------|
| [cobra](https://github.com/spf13/cobra) | CLI framework |
| [viper](https://github.com/spf13/viper) | Configuration |
| [aws-sdk-go-v2](https://github.com/aws/aws-sdk-go-v2) | AWS SDK |
| [bubbletea](https://github.com/charmbracelet/bubbletea) | Interactive TUI |
| [lipgloss](https://github.com/charmbracelet/lipgloss) | Styled output |

## Contributing

1. Fork the repository
2. Create your feature branch (`git checkout -b feat/amazing-feature`)
3. Commit your changes (`git commit -m 'feat: add amazing feature'`)
4. Push to the branch (`git push origin feat/amazing-feature`)
5. Open a Pull Request

## License

MIT License - see [LICENSE](LICENSE) for details.
