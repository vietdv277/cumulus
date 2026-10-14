# `internal/kubeconfig`

Read-only reader for `~/.kube/config` (or `KUBECONFIG`). Every file in `KUBECONFIG` is merged like
kubectl does (first file wins for duplicates and `current-context`). Extracts contexts and
current-context for `cml k8s contexts`. Writing is delegated to cloud CLIs
(`aws eks update-kubeconfig`, `gcloud container clusters get-credentials`) — do not add kubeconfig
mutation here.
