# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.9.1] — 2026-04-20

### Changed
- AWS sub-clients (EC2, ASG, ELBv2, RDS, S3, EKS) now constructed lazily on
  first access via `sync.Once` instead of eagerly in `NewClient`. Short-lived
  commands only pay initialization cost for the services they actually use.

### Removed
- Unused `SSM` field on `Client` (`cmd/secrets.go` builds its own `ssm.Client`
  from `Config()`).

## [0.9.0] — 2026-04-20

### Added
- `cml k8s` command tree for managing Kubernetes clusters across AWS EKS and
  GCP GKE: `list`, `get`, `use`, `contexts`.
- `internal/kubeconfig` read-only reader for `~/.kube/config` / `$KUBECONFIG`.
- EKS client wired into `internal/aws/client.go`.

## [0.8.0] — 2026-04-17

### Added
- `cml db` commands for AWS RDS (`list`, `get`, `connect` via SSM bastion).
- `cml storage` commands for S3 (`ls`, `cp`, `sync`, `presign`) with
  `s3://bucket/key` path syntax.

## [0.7.0] — 2026-03-02

### Added
- GCE VM provider for GCP (`cml vm list/get/connect/tunnel/start/stop/reboot`).
- Optional bastion + IAP tunneling for private GCE instances, configured on
  the context.
- `cml use update` to patch individual fields of an existing context.

## [0.6.0] — 2026-03-01

### Added
- Interactive TUI selectors (bubbletea) for VMs and contexts.
- Styled tables with color-coded state indicators.

## [0.5.1] — 2026-02-28

### Fixed
- Config path handling regressions.

## [0.5.0] — 2026-01-20

### Added
- Context-aware CLI architecture. Contexts (`~/.config/cml/config.yaml`)
  encode provider + credentials reference + region.
- `cml use`, `cml status`, `cml contexts` commands.
- Unified `cml vm` and `cml secrets` commands that route through the active
  context.

## [0.4.0] — 2026-01-16

### Added
- Networking commands: `cml vpc`, `cml lb`.

## [0.3.0] — 2026-01-16

### Added
- AWS profile management: `cml profile ls/use/current`.

## [0.2.0] — 2026-01-16

### Added
- Auto Scaling Group commands: `cml asg ls/describe/instances/scale`.

[Unreleased]: https://github.com/vietdv277/cumulus/compare/v0.9.1...HEAD
[0.9.1]: https://github.com/vietdv277/cumulus/compare/v0.9.0...v0.9.1
[0.9.0]: https://github.com/vietdv277/cumulus/compare/v0.8.0...v0.9.0
[0.8.0]: https://github.com/vietdv277/cumulus/compare/v0.7.0...v0.8.0
[0.7.0]: https://github.com/vietdv277/cumulus/compare/v0.6.0...v0.7.0
[0.6.0]: https://github.com/vietdv277/cumulus/compare/v0.5.1...v0.6.0
[0.5.1]: https://github.com/vietdv277/cumulus/compare/v0.5.0...v0.5.1
[0.5.0]: https://github.com/vietdv277/cumulus/compare/v0.4.0...v0.5.0
[0.4.0]: https://github.com/vietdv277/cumulus/compare/v0.3.0...v0.4.0
[0.3.0]: https://github.com/vietdv277/cumulus/compare/v0.2.0...v0.3.0
[0.2.0]: https://github.com/vietdv277/cumulus/releases/tag/v0.2.0
