# `internal/gcp`

GCP VM connectivity uses `gcloud compute ssh`, optionally routing through an IAP bastion configured
on the context.

The context `region` field (`zoneRE`/`regionRE` in `vm.go`):

- a zone (e.g. `us-central1-a`) uses a zone-scoped `instances.List`;
- a region uses `instances.AggregatedList` filtered to that region's zones;
- anything else is an error.

`bastion_zone` falls back to `region` only when `region` is a zone. A VM name found in several zones
is an error.
