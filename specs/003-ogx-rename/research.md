# Research: Rename to OGX (Open GenAI Stack) Operator

**Branch**: `003-ogx-rename`
**Date**: 2026-04-16 (updated 2026-04-24)
**Spec**: `specs/003-ogx-rename/spec.md`

## Naming Decisions

### D-001: New API Group

- **Decision**: `ogx.io`
- **Rationale**: Short, memorable, matches the "OGX" branding. Standard Kubernetes convention for API groups.
- **Alternatives considered**:
  - `opengenaistack.io` — too long for daily CLI use
  - `ogx.ai` — `.ai` TLD less conventional for K8s API groups; reserve for the GitHub org

### D-002: New CRD Kind

- **Decision**: `OGXServer`
- **Rationale**: Per FR-003, the Kind should reflect that the resource encompasses the full server deployment, not just distribution selection. `OGXServer` is concise and accurately describes the resource.
- **Alternatives considered**:
  - `OGXDistribution` — still uses "distribution" which the spec wants to move away from
  - `OGXStack` — ambiguous (stack of what?)
  - `OpenGenAIStackServer` — too long

### D-003: New Short Name

- **Decision**: `ogxs`
- **Rationale**: Follows the pattern of `{kind-abbreviation}` short names. `ogx` alone could conflict with future resource types in the same API group.
- **Alternatives considered**:
  - `ogx` — simpler but may collide if the API group later adds more Kinds

### D-004: New Resource Plural

- **Decision**: `ogxservers` (auto-generated from Kind by controller-gen)
- **Rationale**: Standard Kubernetes plural convention applied to `OGXServer`.

### D-005: Status Field Rename (FR-004)

- **Decision**: Rename `llamaStackServerVersion` to `serverVersion`
- **Rationale**: The field reports the version of the server binary. With the rename, "LlamaStack" branding is removed. The field name becomes generic and future-proof.
- **Impact**: Breaking change for status consumers. Print column JSONPath changes from `.status.version.llamaStackServerVersion` to `.status.version.serverVersion`.

### D-006: Label and Annotation Values

| Old Value | New Value | Notes |
|-----------|-----------|-------|
| `app: llama-stack` | `app: ogx` | Default label on workloads. New deployments use `app: ogx`. Old deployments are not re-parented (scaled to zero instead); see D-022. |
| `app.kubernetes.io/managed-by: llama-stack-operator` | `app.kubernetes.io/managed-by: ogx-operator` | Used in cache selector, managed ConfigMap labels, Ingress labels. |
| `app.kubernetes.io/part-of: llama-stack` | `app.kubernetes.io/part-of: ogx` | Kustomize-injected label on manifests. |
| `app.kubernetes.io/name: llama-stack-k8s-operator` | `app.kubernetes.io/name: ogx-k8s-operator` | Kustomize default label. |
| `llamastack.io/watch: "true"` | `ogx.io/watch: "true"` | Watch label key for ConfigMap cache filter. During transition, accept both. |

> **Future work (out of scope for this spec)**: All ConfigMaps referenced by an OGXServer CR (e.g., for overrideConfig) MUST carry the `ogx.io/watch: "true"` label so the operator's informer cache can discover them without polling or using an uncached client. The operator should fail reconciliation and set a status condition if a referenced ConfigMap is missing this label. This requirement should be formalized in a dedicated FR in a follow-up spec, as it is a general operator correctness concern rather than a rename-specific change.

### D-007: Leader Election ID

- **Decision**: `54e06e98.ogx.io`
- **Rationale**: Keep the hash prefix (arbitrary, unique), change the domain suffix to match the new API group.

### D-008: Operator Namespace

- **Decision**: `ogx-k8s-operator-system`
- **Rationale**: Mirrors the existing pattern `{project}-system`.

### D-009: Operator Config ConfigMap Name

- **Decision**: `ogx-operator-config`
- **Rationale**: Matches the new branding. Users manually configure the new ConfigMap; no automatic migration from the old one.

### D-010: Go Module Path

- **Decision**: `github.com/ogx-ai/ogx-k8s-operator`
- **Rationale**: Matches the new GitHub organization and repo name. All import paths update accordingly.

### D-011: Container Image (Deferred per FR-006)

- **Decision**: Container image registry (`quay.io/llamastack/llama-stack-k8s-operator`) MAY remain temporarily.
- **Rationale**: FR-006 explicitly allows deferral. The plan enumerates the deferred items so they can be tracked.
- **Deferred items**: `quay.io/llamastack/` registry prefix, `docker.io/llamastack/distribution-*` distribution images, GitHub Actions image references.

### D-012: FieldOwner String

- **Decision**: `ogx-operator`
- **Rationale**: Used in server-side apply as the field owner identity.

## Upstream Runtime Contracts (Out of Scope)

These are contracts with the upstream server container image. They are currently being updated upstream and may change. Preserving or renaming these is **out of scope for the initial PRs** — handle in a follow-up once upstream stabilizes.

| Contract | Current Value | File(s) |
|----------|---------------|---------|
| Python module (core) | `llama_stack.core.server.server` | `controllers/resource_helper.go` |
| Python module (distribution) | `llama_stack.distribution.server.server` | `controllers/resource_helper.go` |
| Version check | `version('llama_stack')` | `controllers/resource_helper.go` |
| Config env var | `LLAMA_STACK_CONFIG` | `controllers/resource_helper.go` |
| Config mount path | `/etc/llama-stack/config.yaml` | `controllers/resource_helper.go` |
| Default storage mount | `/.llama` | `api/v1beta1/ogxserver_types.go` |
| HuggingFace home | `HF_HOME` → `/.llama` | `controllers/resource_helper.go` |

## Migration Approach

### D-020: Adoption Strategy

- **Decision**: Annotation-driven adoption within the reconciler. Users create new OGXServer CRs manually (migrating config from old LLSD CRs and ConfigMaps) and annotate them with `ogx.io/adopt-storage` and/or `ogx.io/adopt-networking` to adopt legacy PVCs and Ingresses.
- **Rationale**: A clean break from the old schema avoids the immutable Deployment selector problem that plagued the automatic adoption approach (see D-022). Users manually migrating config is acceptable for an alpha API, and this approach keeps all resource state reproducible from the CR spec plus annotations.
- **Alternatives considered**:
  - Automatic adoption controller at startup (re-parent all children, create OGXServer from legacy CR) — rejected because Deployment `spec.selector.matchLabels` is immutable, causing persistent reconciliation failures and cascading label mismatches across Service, NetworkPolicy, and HPA. See D-022 for details.
  - Standalone migration CLI tool — rejected because annotation-driven adoption within the operator is simpler and doesn't require a separate tool
  - Conversion webhook — out of scope per spec assumptions (alpha API, one-way rename acceptable)

### D-021: Legacy Resource Cleanup

- **Decision**: Users delete old LLSD CRs using `kubectl delete llsd <name> --cascade=orphan` to preserve child resources (Deployment, PVC, Service, etc.), or remove the old operator first so that no controller is running to trigger cascade deletion.
- **Rationale**: Normal CR deletion cascade-deletes all owned children, which would destroy the PVC data. The `--cascade=orphan` flag prevents this by removing the CR without deleting its children. Alternatively, if the old operator is already removed, there is no active controller to enforce cascade behavior.
- **Rollback consideration**: Using `--cascade=orphan` also preserves the old Deployment (scaled to zero by the adoption logic), enabling rollback if needed.

### D-022: Immutable Deployment Selector Problem (why automatic adoption was rejected)

- **Problem**: Deployment `spec.selector.matchLabels` is immutable after creation. The old Deployment uses `app: llama-stack` in its selector. If the new reconciler tries to patch this to `app: ogx`, the API server rejects it on every reconcile, causing persistent failures.
- **Cascading effects**:
  - Service, NetworkPolicy, and HPA selectors also use `app: llama-stack`. If the reconciler updates those to `app: ogx` while pods still carry `app: llama-stack`, traffic and scaling break.
  - The reconciler can no longer derive resource specs purely from the CR. It must inspect the existing Deployment's immutable selector at reconcile time to generate correct selectors for dependent resources. This complicates the current pipeline which renders all manifests from the CR spec in a single kustomize pass.
  - Resources recreated during normal operations (toggling `exposeRoute` recreates Ingress, disabling and re-enabling NetworkPolicy recreates it, accidental deletion triggers recreation on next reconcile) would all use `app: ogx` from the spec, breaking the selector link to adopted Deployment pods.
- **Decision**: Do not re-parent the old Deployment at all. Instead, scale it to zero and create a new Deployment with clean `app: ogx` labels. This means a brief downtime window (the adopted PVC must transfer between pods) but avoids all the immutable selector problems.

### D-023: Orphaned Stateless Resources Must Be Deleted Before Upgrade

- **Decision**: The upgrade guide instructs users to delete orphaned stateless resources (Deployment, NetworkPolicy, ServiceAccount, RoleBinding, HPA, PDB) after `--cascade=orphan` and before creating the OGXServer CR. Only PVC and optionally Service + Ingress are kept for adoption.
- **Rationale**: After `--cascade=orphan`, Kubernetes removes ownerReferences from child resources but the resources themselves persist with old configuration. The operator's `patchResource` function (`kustomizer.go`) has a safety check that skips resources not owned by the current CR instance (`ref.UID == ownerInstance.GetUID()`). Since orphaned resources have no ownerRefs, the check fails and the operator cannot update or replace them. The orphaned resource blocks creation of a new one with the same name.
- **NetworkPolicy is the critical case**: The orphaned NetworkPolicy's `podSelector.matchLabels.app` still targets `llama-stack`, which does not match the new `app: ogx` pods. This leaves new pods with **no NetworkPolicy protection** and is a security concern. The same label mismatch affects the orphaned Service selector and Deployment selector/template, but those are also covered by the adoption annotations or by simply deleting them.
- **Alternative rejected**: Modifying `patchResource` to automatically claim resources with no ownerReferences was considered but rejected because it would weaken the ownership safety check, potentially allowing the operator to overwrite resources created by users or other controllers.
- **Fallback**: The adopt-storage logic retains the scale-to-zero code path for the old Deployment as a safety net, in case users skip the manual deletion step.

### D-024: Annotation Persistence

- **Decision**: The `ogx.io/adopt-storage` annotation must remain on the OGXServer CR as long as the adopted PVC is in use. The `ogx.io/adopt-networking` annotation can be removed once ownership is transferred (the reconciler continues managing the same-name resources normally).
- **Rationale**: The adopted PVC has a different name (`{old-llsd-name}-pvc`) than the default convention (`{new-ogx-name}-pvc`). The reconciler uses `GetEffectivePVCName()` to resolve the correct name. Without the annotation, the reconciler would try to reference/create a PVC with the default name, breaking the deployment.
- **Networking (same-name)**: When the OGXServer CR uses the same name as the old LLSD, the adopted Service and Ingress names match the reconciler's naming convention. After ownership transfer, the kustomize pipeline manages them normally. Removing the annotation is a no-op.
- **Networking (different-name)**: When the names differ, the annotation MUST persist as long as clients depend on the legacy endpoints. Removing it causes the operator to delete the adopted legacy resources. The kustomize-created resources under the new name continue to exist.

### D-025: Adopt-Networking Includes Service (not just Ingress)

- **Decision**: The `ogx.io/adopt-networking` annotation adopts both the legacy Service (`{value}-service`) and legacy Ingress (`{value}-ingress`).
- **Rationale**: Cluster-internal clients may reference the old Service by name or ClusterIP. Adopting only the Ingress would break internal traffic. The adopted Service's `spec.selector` is updated to match new pod labels so it routes traffic to the new pods while preserving its name and ClusterIP.
- **Same-name case**: When the OGXServer CR name matches the old LLSD name, the adopted resource names match what the kustomize pipeline would create. No duplicate resources are needed. After ownership transfer the reconciler manages them directly.
- **Different-name case**: When the OGXServer CR name differs from the old LLSD name, the adopted resources (`{old-name}-service`, `{old-name}-ingress`) have different names from what the kustomize pipeline creates (`{new-name}-service`, `{new-name}-ingress`). Both sets of resources coexist: adopted resources preserve existing client endpoints, and the kustomize-created resources provide canonical endpoints for the new CR. Removing the `ogx.io/adopt-networking` annotation causes the operator to delete the adopted legacy resources once they are no longer needed.
- **Origin**: This refinement was requested by @eoinfennessy in PR review feedback.

### D-026: `spec.network` (not `spec.networking`)

- **Decision**: The OGXServer CRD uses the JSON field **`network`** (`spec.network`) for port, TLS, expose, and allowed-from policy. The Go type is named **`NetworkSpec`** (kubebuilder/json: `network`).
- **Rationale**: Shorter, conventional name; avoids conflating the spec block with the English word “networking” everywhere and with the transitional annotation `ogx.io/adopt-networking` (which remains unchanged).
- **Note**: Spec 002 and v1alpha2 drafts used `spec.networking`; when folding types into `OGXServer`, rename the field and update any CEL rules that referenced `self.networking` to **`self.network`**.

## Codebase Rename Inventory

### Files to Rename

| Old Path | New Path |
|----------|----------|
| `api/v1alpha1/llamastackdistribution_types.go` | `api/v1beta1/ogxserver_types.go` (new file with expanded API from spec 002); remove `api/v1alpha1/` after cutover |
| `api/v1alpha2/` (entire directory) | DELETE (folded into `ogx.io/v1beta1`) |
| `controllers/llamastackdistribution_controller.go` | `controllers/ogxserver_controller.go` |
| `controllers/llamastackdistribution_controller_test.go` | `controllers/ogxserver_controller_test.go` |
| `controllers/llamastackdistribution_controller_ca_whitespace_test.go` | `controllers/ogxserver_controller_ca_whitespace_test.go` |
| `config/crd/bases/llamastack.io_llamastackdistributions.yaml` | `config/crd/bases/ogx.io_ogxservers.yaml` (generated) |
| `config/crd/patches/cainjection_in_llamastackdistributions.yaml` | `config/crd/patches/cainjection_in_ogxservers.yaml` |
| `config/samples/_v1alpha1_llamastackdistribution.yaml` | `config/samples/_v1beta1_ogxserver.yaml` |

### Go Identifier Renames (representative, not exhaustive)

| Old Identifier | New Identifier |
|----------------|----------------|
| `LlamaStackDistribution` | `OGXServer` |
| `LlamaStackDistributionList` | `OGXServerList` |
| `LlamaStackDistributionSpec` | `OGXServerSpec` |
| `LlamaStackDistributionStatus` | `OGXServerStatus` |
| `LlamaStackDistributionReconciler` | `OGXServerReconciler` |
| `NewLlamaStackDistributionReconciler` | `NewOGXServerReconciler` |
| `LlamaStackDistributionPhasePending` | `OGXServerPhasePending` |
| `LlamaStackDistributionPhaseReady` | `OGXServerPhaseReady` |
| `LlamaStackDistributionPhaseFailed` | `OGXServerPhaseFailed` |
| `LlamaStackDistributionPhaseInitializing` | `OGXServerPhaseInitializing` |
| `LlamaStackDistributionPhaseTerminating` | `OGXServerPhaseTerminating` |
| `DefaultLabelValue` ("llama-stack") | `DefaultLabelValue` ("ogx") |
| `DefaultContainerName` ("llama-stack") | `DefaultContainerName` ("ogx") |
| `LlamaStackServerVersion` (field) | `ServerVersion` (field) |
| `llamaStackUpdatePredicate` | `ogxServerUpdatePredicate` |
| `llamaxk8siov1alpha1` (import alias) | `ogxiov1beta1` |

### Constant Value Renames

| Constant/String | Old Value | New Value |
|-----------------|-----------|-----------|
| `operatorConfigData` | `llama-stack-operator-config` | `ogx-operator-config` |
| `WatchLabelKey` | `llamastack.io/watch` | `ogx.io/watch` |
| `LeaderElectionID` | `54e06e98.llamastack.io` | `54e06e98.ogx.io` |
| `FieldOwner` | `llama-stack-operator` | `ogx-operator` |
| Default NS fallback | `llama-stack-k8s-operator-system` | `ogx-k8s-operator-system` |
| Managed-by label | `llama-stack-operator` | `ogx-operator` |
| Part-of label | `llama-stack` | `ogx` |
| App label | `llama-stack` | `ogx` |
| Kustomize namePrefix | `llama-stack-k8s-operator-` | `ogx-k8s-operator-` |
| Kustomize namespace | `llama-stack-k8s-operator-system` | `ogx-k8s-operator-system` |

### Files Requiring Content Changes (by category)

**Go source (33 files)**: All files under `api/`, `controllers/`, `pkg/`, `tests/e2e/`, `main.go` that import the module path or reference renamed types/constants.

**YAML manifests (20+ files)**: `config/default/`, `config/crd/`, `config/rbac/`, `config/manager/`, `config/samples/`, `controllers/manifests/base/`, `release/operator.yaml`.

**Build/CI (5 files)**: `Makefile`, `go.mod`, `.github/workflows/build-image.yml`, `.github/workflows/release-image.yml`, `.github/workflows/run-e2e-test.yml`.

**Documentation (5+ files)**: `README.md`, `CONTRIBUTING.md`, `docs/create-operator.md`, `docs/additional/ca-bundle-configuration.md`, `docs/api-overview.md`.

**Specs (carry-forward)**: `specs/constitution.md`, `specs/001-*/`, `specs/002-*/` — update references in existing spec documents.
