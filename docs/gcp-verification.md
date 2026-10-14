# GCP Verification Checklist

Manual checks of the GCP providers against a real project, for the Phase 6 item in
`cli-strategy.md`. Every step is read-only: nothing creates, changes or deletes cloud resources.
Unit tests cover the logic; this checklist covers what only a live API can show (permissions,
pagination, real error shapes).

## Setup

```bash
cml version                                   # v0.12.0 or later
gcloud auth application-default login         # ADC for the cml REST clients
cml use add gcp:verify --project <project-id> --region <region-or-zone>
cml doctor --online                           # expect: adc ok, online ok
```

Use `-c gcp:verify` on every command below so the current context does not matter. Replace
`<…>` with names from the project. Do not paste secret values or signed URLs into the results;
record only whether the step worked and any error text.

## Checks

| # | Area | Command | Expected |
| --- | --- | --- | --- |
| 1 | Auth | `cml status` | `✓ Authenticated (ADC)` with the account email |
| 2 | VMs | `cml vm list -c gcp:verify -s all` | Instances in the region/zone; zone column filled |
| 3 | VMs | `cml vm get <vm-name> -c gcp:verify -o json` | One VM document |
| 4 | Suggestions | `cml vm get <vm-name-with-a-typo> -c gcp:verify` | `instance not found: … — did you mean "<vm-name>"?` |
| 5 | Secrets | `cml secrets list -c gcp:verify` | Secret names, no values |
| 6 | Secrets | `cml secrets get <secret-name> -c gcp:verify` | The latest version's value (do not paste it) |
| 7 | Cloud SQL | `cml db list -c gcp:verify` | Instances with engine, version and endpoint |
| 8 | Cloud SQL | `cml db get <instance> -c gcp:verify` | One instance |
| 9 | Cloud SQL | `cml db connect <instance> -c gcp:verify` (optional; needs `cloud-sql-proxy`; Ctrl-C to stop) | Proxy listening on the local port |
| 10 | Storage | `cml storage ls -c gcp:verify` | Buckets in the project |
| 11 | Storage | `cml storage ls gs://<bucket> <prefix> -c gcp:verify` | Objects under the prefix |
| 12 | Storage | `cml storage cp gs://<bucket>/<key> ./gcp-verify.tmp -c gcp:verify` | File downloaded (delete it afterwards) |
| 13 | Storage | `cml storage presign gs://<bucket>/<key> --expires 300 -c gcp:verify` | With a service-account key: a URL. With user ADC: `presign needs service-account credentials (key file or impersonation)` |
| 14 | Logs | `cml logs list -c gcp:verify` | Log names |
| 15 | Logs | `cml logs <log-name> --since 1h --limit 5 -c gcp:verify` | Up to 5 entries |
| 16 | Logs | `cml logs <log-name> -f -c gcp:verify` (stop after a few seconds) | Entries stream; Ctrl-C exits 0 |
| 17 | GKE | `cml k8s list -c gcp:verify` | Clusters with region and node count |
| 18 | Hints | Any of the above in a project lacking the permission or API (optional) | A `Hint:` naming the permission or the `gcloud services enable` command |

## Recording results

Fill in one row per check; record the outcome in `cli-strategy.md` and open follow-ups for failures.
Resource names, secret values and signed URLs stay out of the record.

## Results — v0.12.0, 2026-10-14

Run with `cml` v0.12.0 against one project, with a user account that has partial permissions.
`cml doctor --online` passed 24/24, including the GCP token fetch (`doctor` has no `--context`
flag; it was run against a copy of the config with `gcp:verify` as the current context).

| # | Result | Notes |
| --- | --- | --- |
| 1 | ok | Authenticated (ADC) |
| 2 | ok | 79 VMs across zones; zone set on every row |
| 3 | ok | One VM document |
| 4 | ok | `did you mean` suggestion for a one-letter typo |
| 5 | ok | 25 secrets listed |
| 6 | expected error | 403 `secretmanager.versions.access` denied for this account; `Hint:` named the permission and project |
| 7 | ok | 6 instances; engine, version and endpoint set |
| 8 | ok | One instance |
| 9 | not run | Opens a live proxy connection; out of scope for a read-only pass |
| 10 | ok | 29 buckets |
| 11 | ok | Objects listed in the first readable bucket |
| 12 | ok | Object downloaded (8.6 KB), then deleted |
| 13 | expected error | `presign needs service-account credentials (key file or impersonation)` with user ADC |
| 14 | ok | 38 log names |
| 15 | ok | 5 entries (first attempt hit the project's shared 429 read quota; a retry passed with 1 request) |
| 16 | failed | 3 polls in ~7s, then a 429 from the project's shared Logging read quota ended the tail (exit 1) |
| 17 | ok | 1 cluster; node count set |
| 18 | ok | Covered by #6: permission hint shown |

Follow-ups:

- `cml logs -f` exits on the first 429/503; tailing should back off and keep polling.
- `cml doctor` has no `--context`; it checks only the current context.
