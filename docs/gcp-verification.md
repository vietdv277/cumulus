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

Fill in one row per check and paste the table back; the leader records the outcome in
`cli-strategy.md` and opens issues for failures.

| # | Result (ok / error) | Notes |
| --- | --- | --- |
| 1 | - | - |
