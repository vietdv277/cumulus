# cumulus

Wrapper CLI for AWS and GCP cloud services.

## Core Philosophy

```bash
Original CLI:
aws ec2 describe-instances --filters "Name=instance-state-name,Values=running" \
  --query "Reservations[*].Instances[*].[InstanceId,Tags[?Key=='Name'].Value|[0]]" --output table

My CLI:
cml ec2 ls
cml ec2 ssh  # interactive selector
```

## Principles

- Discoverability: Interactive menus over memorizing flags
- Sensible defaults: Common queries built-in
- Consistency: Same UX across AWS/GCP
- Speed: Faster than typing long commands

## Technical Architecture

### Project Structure

```bash
cloudctl/
├── cmd/
│   └── root.go              # Main entry, Cobra setup
│   └── ec2.go
│   └── gce.go
│   └── profile.go
│   └── project.go
├── internal/
│   ├── aws/
│   │   ├── client.go        # AWS SDK wrapper
│   │   ├── ec2.go
│   │   ├── ssm.go
│   │   ├── asg.go
│   │   └── profile.go
│   ├── gcp/
│   │   ├── client.go        # GCP SDK wrapper
│   │   ├── compute.go
│   │   └── project.go
│   ├── ui/
│   │   ├── selector.go      # fzf-like interactive selection
│   │   ├── table.go         # Pretty table output
│   │   └── spinner.go       # Loading indicators
│   └── config/
│       └── config.go        # App configuration
├── pkg/
│   └── types/               # Shared types
├── configs/
│   └── default.yaml         # Default settings
├── go.mod
├── go.sum
├── main.go
└── Makefile
```

### Key Libraries


| Purpose        | Library                            | Why                         |
| -------------- | ---------------------------------- | --------------------------- |
| CLI framework  | github.com/spf13/cobra             | Industry standard, great UX |
| Config         | github.com/spf13/viper             | Works well with Cobra       |
| AWS SDK        | github.com/aws/aws-sdk-go-v2       | v2 is more modern           |
| GCP SDK        | cloud.google.com/go                | Official SDK                |
| Interactive UI | github.com/charmbracelet/bubbletea | Beautiful TUI               |
| Selection      | github.com/charmbracelet/huh       | Forms and selection         |
| Tables         | github.com/charmbracelet/lipgloss  | Styled output               |
| Spinners       | github.com/briandowns/spinner      | Loading indicators          |
| Colors         | github.com/fatih/color             | Colored output              |


### Development Roadmap

| Week  | Milestone                                      |
| ----- | ---------------------------------------------- |
| 1-2   | Project setup, Cobra/Viper, AWS client, ec2 ls |
| 3-4   | Interactive UI with Bubbletea, ec2 ssh         |
| 5-6   | GCP compute integration                        |
| 7-8   | ASG/MIG management, profile/project switching  |
| 9-10  | Networking commands (vpc, subnet, lb)          |
| 11-12 | Config file, shortcuts, polish                 |


## Learning Path (Go Concepts)

| Feature           | Go Concepts                           |
| ----------------- | ------------------------------------- |
| CLI structure     | Packages, interfaces, Cobra patterns  |
| AWS/GCP clients   | Context, error handling, SDK patterns |
| Interactive UI    | Goroutines, channels (Bubbletea)      |
| Config management | Struct tags, Viper binding            |
| Table output      | Formatting, reflection                |
| Testing           | Table-driven tests, mocks             |


