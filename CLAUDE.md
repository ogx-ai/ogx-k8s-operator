# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Kubernetes operator for OGX (Open GenAI Stack) servers, built with operator-sdk (v4 layout) and controller-runtime. Manages a single CRD: `OGXServer` (group `ogx.io`, version `v1beta1`). The operator deploys and manages OGX server workloads including Deployments, Services, PVCs, NetworkPolicies, PDBs, HPAs, and Ingresses.

The project was renamed from "LlamaStack" to "OGX" — `api/v1alpha1/` contains the legacy `LlamaStackDistribution` CRD (read-only, used for resource adoption). The controller in `controllers/` supports adopting resources from legacy LlamaStackDistribution CRs into OGXServer CRs via annotations (`ogx.io/adopt-storage`, `ogx.io/adopt-networking`).

## Build & Development Commands

```bash
make build                    # Build manager binary (includes generate, fmt, vet)
make test                     # Run unit/integration tests (all packages except e2e)
make lint                     # Run golangci-lint (v2, config in .golangci.yml)
make lint-fix                 # Auto-fix lint issues
make fmt                      # Format code (go fmt + gci import ordering via golangci-lint)
make manifests                # Regenerate CRDs, RBAC, webhook configs via controller-gen
make generate                 # Regenerate DeepCopy methods
make api-docs                 # Regenerate docs/api-overview.md from CRD types
```

### Running specific tests

```bash
make test TEST_PKGS=./pkg/deploy                                    # Single package
make test TEST_PKGS=./pkg/deploy TEST_FLAGS="-v -run TestRenderManifest"  # Single test
make test TEST_PKGS="./pkg/deploy ./controllers"                    # Multiple packages
```

### E2E tests (requires a running cluster + deployed operator)

```bash
make test-e2e       # Deploys ollama via hack/deploy-quickstart.sh, then runs tests
```

### Image and deployment

```bash
make image IMG=quay.io/<user>/ogx-k8s-operator:<tag>   # Build and push image
make deploy IMG=<img>           # Deploy on vanilla K8s (cert-manager overlay)
make deploy-openshift IMG=<img> # Deploy on OpenShift (service-serving-cert-signer)
make undeploy                   # Remove from vanilla K8s
make release VERSION=0.2.1 LLAMASTACK_VERSION=0.2.12   # Prepare release
```

Create `local.mk` to override Makefile variables (e.g., `IMG`, `CONTAINER_TOOL`).

## Architecture

### Reconciliation Pipeline

`main.go` → `OGXServerReconciler.Reconcile()` in `controllers/ogxserver_controller.go`:

1. **Refresh operator config** — reads `ogx-operator-config` ConfigMap for image overrides via a direct (non-cached) client
2. **Adopt legacy resources** — `legacy_adoption.go` handles PVC/Service/Ingress migration from LlamaStackDistribution
3. **Reconcile ConfigMaps** — validates user override ConfigMap, gathers and validates CA bundles (explicit + auto-detected ODH trusted CA), creates managed CA bundle ConfigMap
4. **Render manifests via kustomize** — `pkg/deploy/kustomizer.go` runs kustomize on `controllers/manifests/base/`, then applies Go-based transformer plugins (`pkg/deploy/plugins/`)
5. **Apply resources** — `pkg/deploy/kustomizer.go:ApplyResources()` creates or patches each resource using server-side apply with field owner `ogx-operator`
6. **Reconcile Ingress** — external access via `network_resources.go`
7. **Update status** — deployment readiness, storage/service conditions, provider info from `/v1/providers`, server version from `/v1/version`

### Key Packages

- **`api/v1beta1/`** — OGXServer CRD types, webhook validation (CEL rules + Go webhook), provider type definitions split across `provider_types_*.go`
- **`controllers/`** — reconciler, status management, network resources, legacy adoption, resource helpers
- **`controllers/manifests/base/`** — kustomize base manifests (deployment, service, pvc, networkpolicy, pdb, hpa, serviceaccount, rolebinding)
- **`pkg/deploy/`** — kustomize rendering, resource application (SSA), manifest context building
- **`pkg/deploy/plugins/`** — Go-based kustomize transformer plugins (name prefix, namespace, field mutator, NetworkPolicy transformer)
- **`pkg/cluster/`** — cluster info initialization, upgrade cleanup
- **`pkg/compare/`** — resource comparison utilities
- **`distributions.json`** — embedded at compile time, maps distribution names to container images

### Distribution Resolution

Distribution images are resolved via: `distributions.json` (embedded) → operator ConfigMap `image-overrides` (runtime override) → `spec.distribution.image` (direct override per CR).

### Resource Ownership

- Namespace-scoped resources get ownerRef → garbage collected on CR deletion
- PVCs are intentionally excluded from ownerRef to prevent data loss
- PVCs use `Recreate` deployment strategy to avoid RWO multi-attach deadlock
- Resources patched via SSA must be owned by the current OGXServer instance (safety check prevents "stealing")

### ConfigMap Cache Design

The operator cache filters ConfigMaps by label `ogx.io/watch: "true"`. Operator-managed ConfigMaps (CA bundles) get this label automatically and are watched via `Owns()`. User-referenced ConfigMaps need this label for instant reconciliation. The operator config ConfigMap and user ConfigMaps without the label are read via a direct (non-cached) API client with 5-minute periodic requeue for eventual consistency.

## Code Conventions

- **Error messages**: All wrapped errors must start with `"failed to"` — enforced by `hack/check_go_errors.py` pre-commit hook
- **Import ordering**: standard → default → blank → dot (enforced by gci via golangci-lint)
- **Linter config**: golangci-lint v2 with `default: all` and specific disables in `.golangci.yml`; max line length 180, max function length 100 lines/statements
- **Tests**: table-driven with descriptive names explaining behavior; use `require.Eventually` for async K8s operations; integration tests use `envtest` (kubebuilder assets v1.31.0)
- **Pre-commit hooks**: linters, manifest generation, installer rebuild, API docs generation, error message format check, GitHub Actions SHA-pinning check
- **Code generation**: run `make manifests generate` after changing CRD types or RBAC markers; generated files include `zz_generated.deepcopy.go` and CRDs in `config/crd/bases/`
