# Implementation Plan: OGX (Open GenAI Stack) Operator

**Branch**: `003-ogx-rename` | **Date**: 2026-04-24 | **Spec**: `specs/003-ogx-rename/spec.md`

## Summary

Replace the `LlamaStackDistribution` CRD (`llamastack.io/v1alpha1`) with a new `OGXServer` CRD (`ogx.io/v1beta1`). This is a **breaking change** — no conversion webhooks, no coexistence period. The new CRD incorporates both the rename and the expanded API surface from spec 002 (providers, resources, state storage, **`spec.network`**, workload, overrideConfig). The OGX controller handles the new CR and will later handle config generation.

Upstream runtime contracts (`LLAMA_STACK_CONFIG`, `/etc/llama-stack/config.yaml`, etc.) are being updated upstream and are out of scope for the initial PRs.

### PR Strategy

| PR | Scope | Description |
|----|-------|-------------|
| **PR 1** | API types + generated artifacts | New `OGXServer` CRD with expanded spec, generated deepcopy + CRD YAML, config scaffolding, samples, CI |
| **PR 2** | Controller + adoption + tests + docs | Reconciler adapted to new spec, annotation-driven adoption, unit/E2E tests, migration guide |
| **PR 3** | Config generation | Provider expansion, resource registration, storage config, secret resolution (spec 002 follow-up) |

## Technical Context

**Language/Version**: Go 1.25+ (operator), Python 3.11+ (server images — not modified)
**Primary Dependencies**: controller-runtime v0.22+, client-go v0.34+, kubebuilder v4
**Target Platform**: Kubernetes 1.32+, OpenShift 4.19+

## Constitution Check

- ✅ §1.1 Namespace-scoped — Adoption operates within the CR's namespace; no cluster-admin required.
- ✅ §1.2 Idempotent reconciliation — Adoption checks ownerRefs before acting.
- ✅ §1.3 Owner references — Adopted PVC/Service/Ingress get ownerRefs pointing to the new CR.
- ✅ §2.1 Kubebuilder validation — New CRD uses CEL validation markers.
- ✅ §3 Status has Phase + Conditions — `StorageAdopted` and `NetworkingAdopted` condition types.
- ✅ §4.1 Error wrapping — All new code wraps errors with `%w` and resource identifiers.
- ✅ §5.1 Logger in context — Adoption uses context logger with migration-specific fields.
- ✅ §6.1 Table-driven tests — All adoption test cases use table-driven pattern.
- ✅ §11 Feature flags — Adoption is opt-in via CR annotations; no global flag needed.
- ✅ §13.1 Conventional commits — All commits use `feat:`, `fix:`, `refactor:`, `docs:` prefixes.

## Project Structure

### New and Renamed Files

```text
api/v1beta1/
├── groupversion_info.go          # NEW: API group ogx.io, version v1beta1
├── ogxserver_types.go            # NEW: expanded OGXServer types (replaces llamastackdistribution_types.go)
├── ogxserver_types_test.go       # NEW: adoption helper tests
└── zz_generated.deepcopy.go      # REGENERATE

api/v1alpha1/                     # DELETE after migration (legacy llamastack.io types removed)
api/v1alpha2/                     # DELETE (folded into ogx.io/v1beta1)

controllers/
├── ogxserver_controller.go        # RENAME from llamastackdistribution_controller.go
├── ogxserver_controller_test.go   # RENAME
├── ogxserver_controller_ca_whitespace_test.go  # RENAME
├── legacy_adoption.go             # NEW
├── legacy_adoption_test.go        # NEW
├── kubebuilder_rbac.go            # MODIFY: ogx.io markers
├── status.go                      # MODIFY: OGXServer types, new conditions
├── resource_helper.go             # MODIFY: new spec shape
├── network_resources.go           # MODIFY: new `spec.network` shape
└── manifests/base/*.yaml          # MODIFY: app: ogx labels

config/
├── crd/bases/ogx.io_ogxservers.yaml   # REGENERATE
├── crd/patches/cainjection_in_ogxservers.yaml  # RENAME
├── rbac/ogxs_editor_role.yaml     # RENAME from llsd_editor_role.yaml
├── rbac/ogxs_viewer_role.yaml     # RENAME from llsd_viewer_role.yaml
├── samples/_v1beta1_ogxserver.yaml  # RENAME + restructure to new schema
└── samples/example-*.yaml         # MODIFY: new apiVersion, Kind, spec shape
```

## Implementation Phases

Phases map 1:1 to the task list in `tasks.md`. See that file for the complete task breakdown.

### Phase 1: API Changes (PR 1)

New `OGXServer` types under `ogx.io/v1beta1` with the expanded spec from 002:
- `OGXServerSpec` with `distribution`, `providers`, `resources`, `storage`, `disabled`, **`network`**, `workload`, `externalProviders`, `overrideConfig`
- CEL validation: `providers`/`resources`/`storage`/`disabled` mutually exclusive with `overrideConfig`; `distribution.name` mutually exclusive with `distribution.image`
- Status types: `OGXServerStatus` with `ResolvedDistribution`, `ConfigGeneration`, `ServerVersion`
- Adoption annotation helpers (`GetAdoptStorageSource`, `GetEffectivePVCName`)
- Generated CRD YAML, deepcopy, config scaffolding, samples, CI updates

### Phase 2: Controller Foundation (PR 2)

Rename controller files and adapt the reconciler to the new spec structure:
- Map `spec.distribution` → image, `spec.workload` → Deployment, **`spec.network`** → Service/Ingress/NetworkPolicy
- `overrideConfig` path: mount user-provided ConfigMap
- Default path: deploy with distribution's embedded config (no ConfigMap mount)
- Update all packages (`pkg/deploy`, `pkg/cluster`), `main.go`, manifests

### Phase 3: Adoption Logic (PR 2)

Annotation-driven adoption of legacy resources:
- Validate annotation values are non-empty, valid RFC 1123 DNS labels (FR-007); set `AdoptionConfigInvalid` condition if invalid, skip adoption, proceed with normal reconciliation
- `ogx.io/adopt-storage` → adopt PVC, scale old Deployment to zero, replace ownerRef (remove old controller ref, then `SetControllerReference` to new CR), annotate adopted resources with `ogx.io/adopted-from` and `ogx.io/adopted-at` (FR-017a)
- `ogx.io/adopt-networking` → adopt Service (update selectors) + Ingress, transfer ownerRefs. When CR name differs from legacy name, adopted resources coexist alongside new resources from the kustomize pipeline (FR-015)
- Idempotency via `metav1.IsControlledBy`
- `StorageAdopted` / `NetworkingAdopted` / `AdoptionConfigInvalid` conditions; audit annotations on child resources (FR-017a)
- RBAC markers for legacy resource access (marked `// TRANSITIONAL`)

### Phase 4–6: Tests, Docs, Verification (PR 2)

Tests (unit + E2E), migration guide, doc updates, residual naming audit.

## User Upgrade Steps

This is a **breaking change**. Only the OGX operator will be available after upgrade.

```text
Step 1: Remove the old LLS operator
  ├─ Via meta-operator: set dsc.spec.components.lls to "Removed"
  └─ Manual: delete operator manifests (operator Deployment, RBAC, ServiceAccount)

Step 2: Delete orphaned stateless resources
  └─ kubectl delete deploy,networkpolicy,sa,rolebinding -l app.kubernetes.io/instance=<name>
  └─ kubectl delete hpa,pdb -l app.kubernetes.io/instance=<name>
  (KEEP: PVC, and optionally Service + Ingress for adoption.
   WHY: Orphaned resources have no ownerRefs. The new operator's patchResource
   safety check skips resources it doesn't own. Orphaned NetworkPolicy with
   app: llama-stack selector leaves new app: ogx pods unprotected.)

Step 3: Install the new OGX operator
  ├─ Via meta-operator: set dsc.spec.components.ogx to "Managed"
  └─ Manual: apply release/operator.yaml

Step 4: Create OGXServer CR with adopt-storage annotation
  └─ Translate fields from old LLSD CR + ConfigMap into new OGXServer spec
  └─ Include annotation: ogx.io/adopt-storage: "<old-llsd-name>"

Step 5 (optional): Adopt networking
  └─ Add annotation: ogx.io/adopt-networking: "<old-llsd-name>"

Step 6: Clean up legacy resources
  └─ kubectl delete crd llamastackdistributions.llamastack.io
```

## Adoption Design

### Storage adoption flow

1. Validate annotation value is non-empty and a valid RFC 1123 DNS label (FR-007); set `AdoptionConfigInvalid` condition if invalid, skip adoption, proceed with normal reconciliation
2. Resolve legacy PVC name: `{legacyName}-pvc`
3. If PVC not found, log warning and skip (no error)
4. If PVC already has ownerRef pointing to this CR, return early (idempotent)
5. If old Deployment still exists and is running, scale to zero and requeue
6. Wait for old pods to terminate (requeue with delay)
7. Replace PVC ownerRef: remove existing controller ownerRef (legacy CR), then call `ctrl.SetControllerReference` to set new CR as controller owner. Annotate PVC with `ogx.io/adopted-from` and `ogx.io/adopted-at`. Emit event.

### Networking adoption flow

1. Validate annotation value is non-empty and a valid RFC 1123 DNS label (FR-007); set `AdoptionConfigInvalid` condition if invalid, skip adoption, proceed with normal reconciliation
2. Adopt Service: update `spec.selector` to new pod labels, replace ownerRef (remove old controller ref, `SetControllerReference` to new CR), annotate with `ogx.io/adopted-from` and `ogx.io/adopted-at`
3. Adopt Ingress: replace ownerRef (same pattern), annotate with `ogx.io/adopted-from` and `ogx.io/adopted-at`
4. **Same-name case** (CR name == legacy name): resource names match the kustomize pipeline's naming convention — managed normally after ownership transfer, no duplicate resources
5. **Different-name case** (CR name != legacy name): adopted legacy resources coexist alongside new resources created by the kustomize pipeline. Both sets are owned by the new CR. Removing the `ogx.io/adopt-networking` annotation causes the operator to delete the adopted legacy resources.

### Rollback

Scaling old Deployment to zero (not deleting) preserves a rollback path: remove annotation, scale old Deployment back up, recreate old LLSD CR.

## Alternative Approaches Considered

| Approach | Why Rejected |
|----------|-------------|
| Automatic adoption controller (startup scan) | Deployment `spec.selector.matchLabels` is immutable — can't patch `app: llama-stack` to `app: ogx`, causing persistent reconciliation failures and cascading label mismatches |
| Conversion webhook | Not needed for this breaking cut (new group/kind; users recreate CRs); webhook would add operational complexity without serving legacy `ogx.io` versions |
| Standalone migration CLI | Annotation-driven adoption in the operator is simpler; no separate tool to install |
| In-place CRD rename | API group can't be changed this way; not supported by Kubernetes |
| Parallel reconcilers | Two reconcilers fighting over same child resources; complex ownership |

## Complexity Tracking

| Decision | Why Needed | Simpler Alternative Rejected Because |
|----------|------------|--------------------------------------|
| Annotation-driven adoption | Users need to preserve PVC data during migration | Automatic adoption rejected due to immutable Deployment selector |
| Scale-to-zero (not delete) old Deployment | Preserves rollback path | Deleting is simpler but irreversible |
| `GetEffectivePVCName()` helper | Adopted PVC has different name than default convention | Hardcoding `instance.Name + "-pvc"` breaks adoption |
| Requeue on pending pod termination | RWO PVC can't be mounted by two nodes | Without requeue, new pod stuck in Pending |
| Delete orphaned stateless resources (Step 2) | `patchResource` ownership check blocks updates to orphans; NetworkPolicy with old selector leaves pods unprotected | Modifying `patchResource` to claim unowned resources would weaken safety check |

## Risks and Mitigations

| Risk | Impact | Mitigation |
|------|--------|------------|
| User deletes LLSD CR without removing operator first (cascade-deletes PVC) | High — data loss | Migration guide prominently warns; instruct to remove operator first |
| RWO PVC transfer causes brief downtime | Medium — 30-60s interruption | Documented and expected; status conditions report progress |
| User removes `adopt-storage` annotation while adopted PVC in use | Medium — reconciler references wrong PVC name | Status condition warning; document that annotation must persist |
| Module path change breaks downstream importers | Medium — downstream must update imports | Coordinate with ODH/RHOAI before merge |

## Dependencies and Prerequisites

- Confirm `ogx-ai` GitHub org exists and `ogx-k8s-operator` repo is created
- Confirm `ogx.io` API group has no conflicts
- Coordinate with downstream consumers (ODH, RHOAI) on migration timeline
- Decide if container registry migration is in this release or deferred (FR-006)

## Success Metrics

- PVC data preserved byte-for-byte after adoption (SC-001)
- Clean-install startup time unchanged (SC-002)
- Migration guide enables unassisted migration (SC-003)
- No residual legacy naming in operator-owned artifacts (SC-005)
- `kubectl get ogxs` returns new resources (SC-006)
- PVC adoption downtime under 90 seconds (SC-009)
