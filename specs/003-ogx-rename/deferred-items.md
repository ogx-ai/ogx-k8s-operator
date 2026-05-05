# Deferred Items: OGX Rename

Items deferred from the initial rename PRs per FR-006. These use legacy naming intentionally and will be addressed in follow-up work.

## Container Registry URLs

The following container registry references still use the old naming convention. These depend on external infrastructure (registry org creation, image migration) and will be updated once the `ogx-ai` Quay.io org and image pipeline are established:

- `quay.io/llamastack/` image references in `distributions.json`
- Hardcoded fallback images in `pkg/cluster/cluster.go`
- CI workflow image push targets (`.github/workflows/build-image.yml`, `release-image.yml`)

## Git Organization URLs

The Go module path has been updated to `github.com/ogx-ai/ogx-k8s-operator`, but the following depend on the GitHub org existing with the repo:

- `go.mod` module path (currently updated but repo not yet migrated)
- Import paths across all Go files (updated to new module path)
- GitHub Actions workflow references to the repo

## Upstream Runtime Contracts

Per the spec, the following upstream runtime contract strings are preserved as-is because upstream is still stabilizing them:

- `LLAMA_STACK_CONFIG` environment variable
- `/etc/llama-stack/config.yaml` config mount path
- `/.llama` legacy upstream data directory (operator uses `/.ogx` for its own mount)
- `llama_stack.core.server.server` Python module entrypoint

These will be updated once the upstream `llama-stack` Python package completes its own rename.

## Adoption Annotations / Labels

The following annotations and labels are transitional and will be removed once migration tooling is no longer needed:

- `ogx.io/adopt-storage` (annotation on OGXServer CR)
- `ogx.io/adopt-networking` (annotation on OGXServer CR)
- `ogx.io/adopted-from` (label on adopted PVC/Service/Ingress)
- `ogx.io/adopted-at` (annotation on adopted resources)

Target removal: 2 minor releases after adoption feature ships.

## RBAC Markers

Transitional RBAC permissions in `controllers/kubebuilder_rbac.go` for legacy resource access:

- Pods (`list`) — for checking termination status during adoption scale-down

These will be removed alongside the adoption annotations.
