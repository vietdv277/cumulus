# Changelog

All notable changes to this project are documented here. The format is based on
[Keep a Changelog](https://keepachangelog.com/en/1.1.0/), and this project
adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

### Fixed
- `cml logs -f` on GCP no longer exits on the first rate-limit (429) or
  transient server error (500/503): the poll backs off, doubling the wait up
  to a minute, and gives up after five failures in a row.

### Removed
- `defaults.interactive` and `defaults.region_fallback` from the config
  schema; no command ever read them. Existing configs still load and the keys
  are dropped on the next save.

## [0.12.0] — 2026-10-14

### Added
- Recovery hints: a failing command prints `Hint:` lines after the error for
  expired or missing credentials (`aws sso login --profile <p>`,
  `gcloud auth application-default login`), IAM denials (naming the denied
  action when the API reports it), and disabled GCP APIs
  (`gcloud services enable …`). `cml status` and `cml doctor --online` use
  the same hints.
- "Did you mean" suggestions when a context, alias, saved tunnel or VM name is
  not found.

### Fixed
- `cml use delete <unknown>` exits 1 with a not-found error instead of
  succeeding silently.

## [0.11.1] — 2026-10-12

### Fixed
- Saving the config tightens an existing group/world-accessible config
  directory to 0700, so the `cml doctor` permissions hint now clears the failure.
- Runtime errors (for example a failed `cml doctor`) no longer print the
  command's usage text; flag and argument errors still do.
- Errors are printed once instead of twice.

### Changed
- Release binaries are stripped (`-s -w`), about 30% smaller.

## [0.11.0] — 2026-10-12

### Added
- `cml logs <target>` (alias `cml log`): query or tail (`-f`) logs with
  `--since`, `--filter` and `--limit`; `cml logs list [prefix]` lists targets.
  AWS reads CloudWatch log groups, GCP reads Cloud Logging log names. With
  `--follow`, `-o json` prints one JSON object per line.
- Global `-o/--output table|json|yaml` on every data-printing command; honours
  `defaults.output`. With json/yaml stdout carries only the document and empty
  results print `[]`.
- `cml alias list/add/rm` to manage context aliases.
- `cml doctor`: read-only checks for config, contexts, aliases, tunnels and the
  required CLIs; `--online` adds a credentials check. Exits 1 on failures.
- Saved tunnels: `cml vm tunnel … --save <name>` and `cml tunnel list/start/rm`;
  a saved tunnel always starts in its own context.
- `-c/--context` on `cml aws ssm` and `cml aws iam`.
- GCP Secret Manager behind `cml secrets` (deletion on GCP is permanent).
- GCP Cloud SQL behind `cml db` (`db connect` runs `cloud-sql-proxy`).
- GCS behind `cml storage` with `gs://` paths: ls, cp, sync (via
  `gcloud storage rsync`) and presign (V4, service-account key credentials).
- `--yes/-y` and a confirmation prompt on `vm stop`, `vm reboot`, the interactive
  Stop action in `vm list -i`, `secrets delete` and `storage sync --delete`.
  Without a TTY and without `--yes` these commands refuse instead of proceeding.
- `secrets delete --force` for immediate, unrecoverable Secrets Manager deletion.
- `ssm:` / `sm:` name prefixes to force the secrets backend.
- Unit tests for `cmd`, `internal/aws`, `internal/config`, `internal/gcp` and
  `internal/kubeconfig`.

### Changed
- `secrets delete` on Secrets Manager is now recoverable for 7 days by default.
- `--profile/-p` and `--region/-r` are rejected by context-aware commands (`vm`,
  `k8s`, `secrets`, `storage`, `db`) instead of being silently ignored.
- A VM name matching more than one instance (AWS) or zone (GCP) is an error that
  lists the matches; terminated instances no longer match.
- `cml profile set` stores the profile as an `aws:<profile>` context.
- Config is written atomically with mode 0600 (directory 0700).
- `db connect` falls back to the context's `bastion` when `--via` is omitted.
- Go 1.26; `golang.org/x/net`, `google.golang.org/grpc` and `golang.org/x/text`
  upgraded past reachable vulnerabilities.

### Fixed
- Aliases now resolve in `--context` and `cml use`; `cml use <unknown>` exits 1.
- `cml use add AWS:prod` stores a lower-case provider.
- `--ssm-only` / `--sm-only` are honoured.
- Plain secret names that exist only in SSM resolve on `get`/`set`; `set` no
  longer turns any error into `CreateSecret`.
- `cml vm get i` no longer panics; `ec2 --name` matches the `Name` tag.
- List commands return every page (EC2, VPC, subnets, load balancers, buckets).
- API errors such as AccessDenied are no longer reported as "not found".
- `vm tunnel` and `k8s connect` tear down the whole SSM process group, refuse a
  local port that is already in use, and fail fast if the tunnel exits.
- S3 copy encodes `CopySource`, honours `dir/` destinations, writes downloads
  atomically and bounds presign expiry to 7 days.
- Legacy config migration no longer wipes a config it cannot parse or overwrites
  an existing context.
- GCP: malformed zone/region values and a missing `bastion_zone` are errors;
  GKE partial results and node counts are reported correctly.
- Every file in `$KUBECONFIG` is read, like kubectl.
- Interactive selector failures are reported instead of exiting 0.

## [0.10.0] — 2026-04-23

### Added
- `cml k8s connect <cluster>` — opens an SSM port-forwarding session to the
  AWS context's bastion and launches an interactive subshell with
  `HTTPS_PROXY` set, so `kubectl` reaches a private EKS API through the
  bastion. Exiting the subshell tears the tunnel down. AWS only; GCP
  contexts return a clear error.
- `bastion` / `bastion_port` fields on AWS contexts, and matching
  `--bastion` / `--bastion-port` flags on `cml use add` and `cml use update`.

### Changed
- Extracted `AWSVMProvider.StartPortForward` from `Tunnel` so `vm tunnel`
  and `k8s connect` share the SSM session plumbing. No behaviour change
  for `vm tunnel`.

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

[Unreleased]: https://github.com/vietdv277/cumulus/compare/v0.12.0...HEAD
[0.12.0]: https://github.com/vietdv277/cumulus/compare/v0.11.1...v0.12.0
[0.11.1]: https://github.com/vietdv277/cumulus/compare/v0.11.0...v0.11.1
[0.11.0]: https://github.com/vietdv277/cumulus/compare/v0.10.0...v0.11.0
[0.10.0]: https://github.com/vietdv277/cumulus/compare/v0.9.1...v0.10.0
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
