# Tasks: OGX (Open GenAI Stack) Operator

**Branch**: `003-ogx-rename`
**Date**: 2026-04-24
**Plan**: `specs/003-ogx-rename/plan.md`
**Spec**: `specs/003-ogx-rename/spec.md`

## Overview

This is a **breaking change**. The old `LlamaStackDistribution` CRD (`llamastack.io/v1alpha1`) is replaced by a new `OGXServer` CRD (`ogx.io/v1beta1`) that incorporates both the rename and the expanded API surface from spec 002 (providers, resources, state storage, **`spec.network`**, workload, overrideConfig). No conversion webhooks. The OGX controller handles the new CR and will later handle config generation. **`v1beta1`** is required for downstream consumers that only integrate non-alpha API versions.

**NOTE**: Upstream runtime contracts (`llama_stack.core.server.server`, `LLAMA_STACK_CONFIG`, `/etc/llama-stack/config.yaml`, `/.llama`, etc.) are currently being updated upstream and may change. Preserving or renaming these is out of scope for the initial PRs — handle in a follow-up once upstream stabilizes.

### PR Strategy

| PR | Scope | Phase |
|----|-------|-------|
| **PR 1** | API types + generated artifacts | Phase 1 |
| **PR 2** | Controller with basic reconciliation (distribution, configmap, storage, network) | Phase 2–4 |
| **PR 3** | Config generation logic (provider expansion, resource registration, storage config, secret resolution) | Future — spec 002 |

---

## Phase 1: API Changes (PR 1)

**Goal**: Introduce the new `OGXServer` CRD under `ogx.io/v1beta1` with the full expanded spec (distribution, providers, resources, state storage, **`network`**, workload, overrideConfig). Generate CRD YAML and deepcopy. No controller changes yet.

### Go module and API group

- [ ] T001 Update `go.mod` module path from `github.com/llamastack/llama-stack-k8s-operator` to `github.com/ogx-ai/ogx-k8s-operator`
- [ ] T002 Create `api/v1beta1/groupversion_info.go` for the new API group: `+groupName=ogx.io`, `Group: "ogx.io"`, `Version: "v1beta1"`

### OGXServer types

- [ ] T003 Create `api/v1beta1/ogxserver_types.go` with the new CRD types incorporating the expanded API surface:
  - Constants: `DefaultContainerName = "ogx"`, `DefaultLabelValue = "ogx"`, `DefaultServerPort = 8321`, `DefaultMountPath = "/.llama"`, `OGXServerKind = "OGXServer"`
  - `DistributionSpec` — exactly one of `name` or `image` (CEL: mutual exclusivity)
  - `ProviderConfig` — `id`, `provider`, `endpoint`, `apiKey` (`*SecretKeyRef`), `settings` (`*apiextensionsv1.JSON`)
  - `ProvidersSpec` — `inference`, `safety`, `vectorIo`, `toolRuntime`, `telemetry` (each `*apiextensionsv1.JSON` for polymorphic form)
  - `ModelConfig` — `name`, `provider`, `contextLength`, `modelType`, `quantization`
  - `ResourcesSpec` — `models` (`[]apiextensionsv1.JSON`), `tools`, `shields` (`[]string`)
  - `KVStorageSpec` — `type` (sqlite/redis), `endpoint`, `password` (`*SecretKeyRef`)
  - `SQLStorageSpec` — `type` (sqlite/postgres), `connectionString` (`*SecretKeyRef`)
  - `StateStorageSpec` — `kv`, `sql`
  - `CABundleConfig` — `configMapName`
  - `TLSSpec` — `enabled`, `secretName`, `caBundle`
  - `AllowedFromSpec` — `namespaces`, `labels`
  - `NetworkSpec` (JSON field **`network`**) — `port`, `tls`, `expose` (`*apiextensionsv1.JSON`), `allowedFrom`
  - `PVCStorageSpec` — `size`, `mountPath`
  - `PodDisruptionBudgetSpec` — `minAvailable`, `maxUnavailable`
  - `AutoscalingSpec` — `minReplicas`, `maxReplicas`, `targetCPUUtilizationPercentage`, `targetMemoryUtilizationPercentage`
  - `WorkloadOverrides` — `serviceAccountName`, `env`, `command`, `args`, `volumes`, `volumeMounts`
  - `WorkloadSpec` — `replicas`, `workers`, `resources`, `autoscaling`, `storage`, `podDisruptionBudget`, `topologySpreadConstraints`, `overrides`
  - `OverrideConfigSpec` — `configMapName`
  - `OGXServerSpec` — `distribution`, `providers`, `resources`, `storage`, `disabled`, **`network`**, `workload`, `externalProviders`, `overrideConfig` (CEL: `providers`/`resources`/`storage`/`disabled` mutually exclusive with `overrideConfig`)
  - `OGXServerPhase` — `Pending`, `Initializing`, `Ready`, `Failed`, `Terminating`
  - Status types: `ProviderHealthStatus`, `ProviderInfo`, `DistributionConfig`, `VersionInfo` (with `ServerVersion` not `LlamaStackServerVersion`), `ResolvedDistributionStatus`, `ConfigGenerationStatus`, `OGXServerStatus`
  - Root: `OGXServer`, `OGXServerList` with kubebuilder markers (`shortName=ogxs`, printer columns, subresource:status)
  - `init()` registering types with SchemeBuilder
- [ ] T004 Add adoption annotation constants and helpers to `api/v1beta1/ogxserver_types.go`:
  - `AdoptStorageAnnotation = "ogx.io/adopt-storage"`
  - `AdoptNetworkingAnnotation = "ogx.io/adopt-networking"`
  - `AdoptedFromAnnotation = "ogx.io/adopted-from"`
  - `AdoptedAtAnnotation = "ogx.io/adopted-at"`
  - `func (r *OGXServer) GetAdoptStorageSource() string`
  - `func (r *OGXServer) GetAdoptNetworkingSource() string`
  - `func (r *OGXServer) GetEffectivePVCName() string`
  - `func ValidateAdoptionAnnotation(value string) error` — validate non-empty and RFC 1123 DNS label (per FR-007)
- [ ] T005 Write unit tests for adoption helpers in `api/v1beta1/ogxserver_types_test.go`: `GetEffectivePVCName` returns adopted PVC name when annotation present, default name otherwise. Test `ValidateAdoptionAnnotation` with valid names, empty string, invalid characters, and names exceeding 63 characters.

### Generated artifacts

- [ ] T006 Run `make generate` to produce `zz_generated.deepcopy.go` for new types
- [ ] T007 Run `make manifests` to generate CRD YAML (`config/crd/bases/ogx.io_ogxservers.yaml`) and RBAC (`config/rbac/role.yaml`)
- [ ] T008 Delete old generated CRD: `config/crd/bases/llamastack.io_llamastackdistributions.yaml`
- [ ] T009 Remove legacy `llamastack.io` Go types and the `api/v1alpha1/` package; new OGX types live under `api/v1beta1/` (e.g. `ogxserver_types.go`)
- [ ] T010 Delete `api/v1alpha2/` directory (v1alpha2 types are now folded into `ogx.io/v1beta1`)

### Config scaffolding

- [ ] T011 Update `config/crd/kustomization.yaml`: base path to `ogx.io_ogxservers.yaml`, update patch references
- [ ] T012 Rename `config/crd/patches/cainjection_in_llamastackdistributions.yaml` → `config/crd/patches/cainjection_in_ogxservers.yaml` and update content
- [ ] T013 Rename `config/rbac/llsd_editor_role.yaml` → `config/rbac/ogxs_editor_role.yaml`, update role name and API group
- [ ] T014 Rename `config/rbac/llsd_viewer_role.yaml` → `config/rbac/ogxs_viewer_role.yaml`, update role name and API group
- [ ] T015 Update `config/default/kustomization.yaml`: namespace to `ogx-k8s-operator-system`, namePrefix to `ogx-k8s-operator-`, labels to `app.kubernetes.io/name: ogx-k8s-operator`
- [ ] T016 Update `config/default/manager_labels_patch.yaml`: label values
- [ ] T017 Update `config/manager/manager.yaml`, `config/manager/pdb.yaml`: label values
- [ ] T018 Update `config/manager/controller_manager_config.yaml`: `resourceName` to `54e06e98.ogx.io`
- [ ] T019 Update `PROJECT` file: domain to `ogx.io`, repo to `github.com/ogx-ai/ogx-k8s-operator`, kind to `OGXServer`, API version `v1beta1`

### Samples

- [ ] T020 Delete old sample: `config/samples/_v1alpha1_llamastackdistribution.yaml`
- [ ] T021 Create `config/samples/_v1beta1_ogxserver.yaml` — minimal OGXServer sample with distribution name
- [ ] T022 Update all `config/samples/example-*.yaml`: `apiVersion: ogx.io/v1beta1`, `kind: OGXServer`, restructure spec fields to new schema
- [ ] T023 Update `config/samples/kustomization.yaml`: resource references

### Build and CI

- [ ] T024 Update `Makefile`: `IMAGE_TAG_BASE` to `quay.io/ogx-ai/ogx-k8s-operator`, update comments
- [ ] T025 Update `crd-ref-docs.config.yaml`: API group to `ogx.io`
- [ ] T026 Update `.github/workflows/build-image.yml`: image name to `ogx-k8s-operator`
- [ ] T027 Update `.github/workflows/release-image.yml`: image name
- [ ] T028 Update `.github/workflows/run-e2e-test.yml`: namespace to `ogx-k8s-operator-system`, deployment name, image refs, `kubectl get ogxservers`
- [ ] T029 Update `.github/workflows/generate-release.yml`: image name, repo URL references

### Verification (PR 1)

- [ ] T030 Run `make generate manifests` — verify CRD and deepcopy regenerate cleanly
- [ ] T031 Run `go build ./api/...` — verify API package compiles
- [ ] T032 Run `go test ./api/...` — verify type tests pass

---

## Phase 2: Controller Foundation (PR 2)

**Goal**: Rename and restructure the controller to reconcile the new `OGXServer` CR. Support basic reconciliation: distribution image resolution, user-provided ConfigMap, PVC storage, **`spec.network`** (Service, Ingress, NetworkPolicy), and workload (Deployment, HPA, PDB).

### Controller rename

- [ ] T033 Rename `controllers/llamastackdistribution_controller.go` → `controllers/ogxserver_controller.go`
- [ ] T034 Rename `controllers/llamastackdistribution_controller_test.go` → `controllers/ogxserver_controller_test.go`
- [ ] T035 Rename `controllers/llamastackdistribution_controller_ca_whitespace_test.go` → `controllers/ogxserver_controller_ca_whitespace_test.go`
- [ ] T036 Rename `LlamaStackDistributionReconciler` → `OGXServerReconciler` and `NewLlamaStackDistributionReconciler` → `NewOGXServerReconciler` across all controller files
- [ ] T037 Update controller constants: `operatorConfigData` → `"ogx-operator-config"`, `WatchLabelKey` → `"ogx.io/watch"`, managed-by label → `"ogx-operator"`

### Controller adaptation to new spec

- [ ] T038 Update reconciler to work with new `OGXServerSpec` structure: map `spec.distribution` to container image, `spec.workload.replicas` to deployment replicas, **`spec.network.port`** to container/service port, etc.
- [ ] T039 Update `controllers/resource_helper.go`: adapt to new spec shape (distribution, workload.storage, workload.resources, workload.overrides, **`spec.network.tls`**). Preserve all upstream runtime contract strings per FR-002.
- [ ] T040 Update `controllers/status.go`: rename all `LlamaStackDistribution*` type references to `OGXServer*`, add new status fields (`ResolvedDistribution`, `ConfigGeneration`)
- [ ] T041 Update `controllers/network_resources.go`: adapt to **`spec.network`** shape (expose, allowedFrom, tls), rename type references, update managed-by label to `"ogx-operator"`
- [ ] T042 Update `controllers/kubebuilder_rbac.go`: change RBAC markers from `llamastack.io` to `ogx.io`, `llamastackdistributions` to `ogxservers`
- [ ] T043 Update `controllers/suite_test.go`: rename scheme registration
- [ ] T044 Update `controllers/testing_support_test.go`: rename builder and reconciler references, adapt to new spec structure

### ConfigMap support (overrideConfig path)

- [ ] T045 Implement `overrideConfig` path in reconciler: when `spec.overrideConfig.configMapName` is set, use the referenced ConfigMap as the server config (mount it into the pod). Skip any config generation logic.
- [ ] T046 When neither `overrideConfig` nor `providers`/`resources`/`storage` are set, deploy with the distribution's embedded default config (no ConfigMap mount needed — server uses its built-in config)

### Operand manifests

- [ ] T047 Update `controllers/manifests/base/kustomization.yaml`: labels to `app.kubernetes.io/managed-by: ogx-operator`, `app.kubernetes.io/part-of: ogx`, `ogx.io/watch: "true"`
- [ ] T048 Update `controllers/manifests/base/deployment.yaml`: `app: llama-stack` → `app: ogx`
- [ ] T049 Update `controllers/manifests/base/service.yaml`, `networkpolicy.yaml`, `hpa.yaml`, `pdb.yaml`: `app: llama-stack` → `app: ogx`

### Package updates

- [ ] T050 Update `pkg/deploy/kustomizer.go`: rename type refs, `FieldOwner` to `"ogx-operator"`, NS fallback to `"ogx-k8s-operator-system"`
- [ ] T051 Update `pkg/deploy/deploy.go`: rename type refs and `FieldOwner`
- [ ] T052 Update `pkg/deploy/utils.go` and `pkg/deploy/networkpolicy.go`: rename type refs
- [ ] T053 Update `pkg/deploy/plugins/networkpolicy_transformer.go` and `plugins/field_mutator.go`: rename type refs
- [ ] T054 Update `pkg/cluster/cluster.go`: managed-by label check from `"llama-stack-operator"` to `"ogx-operator"`

### Main entrypoint

- [ ] T055 Update `main.go`: rename import alias to `ogxiov1beta1`, update `LeaderElectionID` to `"54e06e98.ogx.io"`, update cache selector managed-by label, update reconciler call
- [ ] T056 Update all import statements across the codebase to the new module path `github.com/ogx-ai/ogx-k8s-operator`

### Verification (PR 2 — build)

- [ ] T057 Run `make generate manifests` — regenerate after controller changes
- [ ] T058 Run `make build-installer` to regenerate `release/operator.yaml`
- [ ] T059 Delete old generated files if any remain (old CRD YAML, old release artifacts)
- [ ] T060 Run `go build ./...` — verify full codebase compiles
- [ ] T061 Run `make lint` — verify no linter errors

---

## Phase 3: Adoption Logic (PR 2, continued)

**Goal**: Implement annotation-driven adoption so users can preserve PVCs and networking resources from old LlamaStackDistribution workloads.

- [ ] T062 Create `controllers/legacy_adoption.go` with `adoptLegacyResources(ctx, instance)` entry point. Validate adoption annotation values using `ValidateAdoptionAnnotation()` (FR-007); if invalid, set `AdoptionConfigInvalid` condition and skip adoption. Normal reconciliation MUST proceed (a typo in an annotation must not block the workload).
- [ ] T063 Implement `adoptStorage(ctx, instance, legacyName)`: find legacy PVC (`{legacyName}-pvc`), scale old Deployment to zero if still running, wait for pod termination (requeue), replace PVC ownerRef (remove existing controller ownerRef, then `ctrl.SetControllerReference` to new CR), annotate PVC with `ogx.io/adopted-from` and `ogx.io/adopted-at` (FR-017a), emit event
- [ ] T064 Implement `adoptNetworking(ctx, instance, legacyName)`: adopt legacy Service (`{legacyName}-service`) by updating selectors to new pod labels + replacing ownerRef (remove old controller ref, `SetControllerReference` to new CR); adopt legacy Ingress (`{legacyName}-ingress`) by replacing ownerRef (same pattern). Annotate both with `ogx.io/adopted-from` and `ogx.io/adopted-at` (FR-017a). When the OGXServer CR name differs from `legacyName`, allow the kustomize pipeline to create new resources under the CR name in addition to the adopted legacy resources (FR-015 name-mismatch case).
- [ ] T065 Implement idempotency: check `metav1.IsControlledBy` before adoption, skip if already adopted
- [ ] T066 Update `configurePersistentStorage()` in `controllers/resource_helper.go` to use `instance.GetEffectivePVCName()` when `adopt-storage` annotation is present
- [ ] T067 Update `determineKindsToExclude()` in reconciler: exclude PVC from kustomize ResMap when `ogx.io/adopt-storage` annotation is present. For `ogx.io/adopt-networking` when the CR name matches the legacy name, exclude Service and Ingress from kustomize ResMap (same-name case); when names differ, allow kustomize to create new resources alongside adopted ones.
- [ ] T068 Update `reconcileResources()`: call `adoptLegacyResources()` before manifest reconciliation, propagate requeue signal
- [ ] T069 Add `StorageAdopted`, `NetworkingAdopted`, and `AdoptionConfigInvalid` condition types to `controllers/status.go`, set from adoption functions
- [ ] T070 Add/verify RBAC markers in `controllers/kubebuilder_rbac.go` for adoption: Deployments (`get`, `list`, `update`), Pods (`list`), PVCs (`get`, `list`, `update`), Services (`get`, `list`, `update`), Ingresses (`get`, `list`, `update`). Mark as `// TRANSITIONAL`
- [ ] T071 Run `make manifests` to regenerate RBAC after marker changes
- [ ] T071a Implement cleanup of adopted networking resources when `ogx.io/adopt-networking` annotation is removed and the CR name differs from the legacy name (different-name case only — same-name case is a no-op per D-024)

---

## Phase 4: Tests (PR 2, continued)

**Goal**: Unit and integration tests for the controller and adoption logic.

### Controller tests

- [ ] T072 Update `controllers/resource_helper_test.go`: adapt to new spec structure
- [ ] T073 Update `controllers/network_resources_test.go`: adapt to new **`spec.network`** schema
- [ ] T074 Update all test files in `pkg/deploy/`: `kustomizer_test.go`, `deploy_test.go`, `suite_test.go`, `plugins/networkpolicy_transformer_test.go`, `plugins/field_mutator_test.go`

### Adoption tests

- [ ] T075 Write table-driven test: `adoptStorage` happy path — annotation present → old Deployment scaled to zero → PVC ownerRef transferred → `StorageAdopted` condition set (`controllers/legacy_adoption_test.go`)
- [ ] T076 Write table-driven test: PVC not found — annotation present but PVC missing → no error, new PVC created normally
- [ ] T077 Write table-driven test: idempotency — reconcile twice with annotation → no changes on second run
- [ ] T078 Write table-driven test: old pods still terminating → requeue with delay
- [ ] T079 Write table-driven test: old Deployment already gone → proceed to PVC ownership transfer
- [ ] T080 Write table-driven tests: `adoptNetworking` — old Service selector updated, old Ingress ownerRef transferred, `NetworkingAdopted` condition set
- [ ] T080a Write table-driven test: `adoptNetworking` with name mismatch — adopted legacy resources coexist alongside kustomize-created new resources, both owned by the new CR
- [ ] T080b Write table-driven test: annotation validation — invalid annotation values (empty, uppercase, special chars, >63 chars) → `AdoptionConfigInvalid` condition set, adoption skipped, normal reconciliation proceeds
- [ ] T080c Write table-driven test: adopted child resources carry `ogx.io/adopted-from` and `ogx.io/adopted-at` annotations after ownership transfer (FR-017a)
- [ ] T081 Write test: clean install without adoption annotations → no adoption code path triggered, normal PVC created

### E2E tests

- [ ] T082 Update all E2E test files in `tests/e2e/`: `creation_test.go`, `deletion_test.go`, `rollout_test.go`, `tls_test.go`, `validation_test.go`, `e2e_test.go`, `test_utils.go` — new API group, Kind, spec structure
- [ ] T083 Write E2E test: full adoption flow → verify `StorageAdopted` condition set, event emitted, adopted PVC accessible
- [ ] T084 Write E2E test: networking adoption → verify Service selector updated, Ingress ownerRef transferred

### Verification (PR 2 — tests)

- [ ] T085 Run `go test ./...` — all tests pass
- [ ] T086 Run grep audit for residual legacy naming: search for `llamastack`, `LlamaStack`, `llama-stack`, `llsd` in all operator-owned files (excluding upstream runtime contracts per FR-002 and deferred items per FR-006) — verify clean

---

## Phase 5: Documentation (PR 2, continued)

**Goal**: Update all docs, create migration guide, update specs.

### Migration guide

- [ ] T087 Create `docs/migration-guide.md` with:
  - Complete old-to-new name mapping table (API group, Kind, plural, short name, labels, annotations, status field paths, CLI commands)
  - Breaking changes section (this is a breaking change — no coexistence period, no webhooks)
  - Step-by-step upgrade instructions:
    1. Remove old LLS operator (via meta-operator or manual)
    2. Delete orphaned stateless resources (`kubectl delete deploy,networkpolicy,sa,rolebinding -l app.kubernetes.io/instance=<name>` + HPA/PDB)
    3. Install new OGX operator
    4. Create OGXServer CR with `ogx.io/adopt-storage` annotation
    5. (Optional) Add `ogx.io/adopt-networking` annotation
    6. Clean up: `kubectl delete crd llamastackdistributions.llamastack.io`
  - NetworkPolicy impact explanation (old `app: llama-stack` selector vs new `app: ogx`)
  - Verification commands (`kubectl get ogxs`, check conditions, check events)
  - Rollback section
  - Deprecation notice for adoption annotations (future removal target)

### Existing docs

- [ ] T088 Update `README.md`: rename all operator/CRD references
- [ ] T089 Update `CONTRIBUTING.md`: update repo references
- [ ] T090 Update `docs/create-operator.md`: rename all references
- [ ] T091 Update `docs/api-overview.md`: rename CRD type references, short name, API group
- [ ] T092 Update `docs/additional/ca-bundle-configuration.md`: rename all references
- [ ] T093 Regenerate API reference docs: `make api-docs`

### Specs

- [ ] T094 Update `specs/constitution.md`: rename examples/references from LlamaStack to OGX
- [ ] T095 Update `specs/001-deploy-time-providers-l1/*.md`: rename operator references
- [ ] T096 Update `specs/002-operator-generated-config/*.md`: rename operator references, note that v1alpha2 is now folded into OGXServer `ogx.io/v1beta1`, and that the folded network block is **`spec.network`** on OGXServer (not `spec.networking`)

---

## Phase 6: Final Verification

**Goal**: Full build and audit pass.

- [ ] T097 Run full verification: `make generate manifests build-installer api-docs lint` and `go test ./...` — zero errors
- [ ] T098 Run grep audit for residual legacy naming (final pass)
- [ ] T099 Create tracking list of deferred items per FR-006 (container registry URLs, Git org URLs) in `specs/003-ogx-rename/deferred-items.md`

---

## User Upgrade Steps

This is a **breaking change**. Only the OGX operator will be available after the upgrade — there is no period where both operators coexist in the same workload. Users must manually create new OGXServer CRs and migrate configuration.

### Key label change: NetworkPolicy impact

The rename changes `DefaultLabelValue` from `llama-stack` to `ogx`. This affects every resource that uses `podSelector` or `selector` with the `app` label:

**Before (old operator)**:
```yaml
spec:
  podSelector:
    matchLabels:
      app: llama-stack
      app.kubernetes.io/instance: my-server
  ingress:
    - from:
        - podSelector: {}
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: llama-stack-k8s-operator-system
      ports:
        - { protocol: TCP, port: 8321 }
```

**After (new operator)**:
```yaml
spec:
  podSelector:
    matchLabels:
      app: ogx
      app.kubernetes.io/instance: my-server
  ingress:
    - from:
        - podSelector: {}
        - namespaceSelector:
            matchLabels:
              kubernetes.io/metadata.name: ogx-k8s-operator-system
      ports:
        - { protocol: TCP, port: 8321 }
```

### Upgrade Path (with PVC data preservation)

```text
Step 1: Remove the old LLS operator
  ├─ Via meta-operator: set dsc.spec.components.lls to "Removed"
  └─ Manual: delete operator manifests (operator Deployment, RBAC, ServiceAccount)
  (The operator Deployment is removed. The operand Deployments, CRD, and CRs remain.)

Step 2: Delete orphaned stateless resources
  └─ kubectl delete deploy,networkpolicy,sa,rolebinding -l app.kubernetes.io/instance=<name>
  └─ Also delete HPA/PDB if they exist: kubectl delete hpa,pdb -l app.kubernetes.io/instance=<name>
  (KEEP: PVC (adopted in Step 4), and optionally Service + Ingress (adopted in Step 5).
   WHY: After operator removal, orphaned resources have no active controller.
   The new operator's patchResource safety check skips resources it does not own,
   so it cannot update or replace them. Deleting them lets the new operator
   create fresh versions with correct "ogx" labels and configuration.
   NetworkPolicy is especially critical: the old policy's podSelector targets
   "app: llama-stack" which no longer matches the new "app: ogx" pods,
   leaving them unprotected until the orphan is removed.)

Step 3: Install the new OGX operator
  ├─ Via meta-operator: set dsc.spec.components.ogx to "Managed"
  └─ Manual: apply release/operator.yaml

Step 4: Create OGXServer CR with adopt-storage annotation
  └─ Translate fields from old LLSD CR + ConfigMap into the new OGXServer spec
  └─ Include annotation: ogx.io/adopt-storage: "<old-llsd-name>"
  (Operator adopts the orphaned PVC, sets ownerReference, creates new Deployment
   that mounts the adopted PVC. ~30-60s until new pod is ready.)

Step 5 (optional): Adopt networking (preserve ClusterIP / external endpoint)
  └─ Add annotation: ogx.io/adopt-networking: "<old-llsd-name>"
  (Operator adopts orphaned Service + Ingress, updates Service selectors to
   new pod labels, sets ownerReferences. If the CR name matches the old LLSD
   name, resource names are unchanged -- no client updates needed. If names
   differ, adopted resources coexist alongside new ones created by the
   kustomize pipeline.)

Step 6: Clean up legacy resources
  └─ Delete old CRD: kubectl delete crd llamastackdistributions.llamastack.io
```

---

## Dependencies

```text
T001–T002 → T003 (module path + API group before types)
T003 → T004–T005 (types before helpers/tests)
T003 → T006–T007 (types before generation)
T006–T010 → T011–T019 (generated artifacts before config scaffolding)
T003 → T020–T023 (types before samples)
T030–T032 → T033 (PR 1 verified before controller work)

T033–T037 → T038–T044 (controller renamed before adaptation)
T038–T044 → T045–T046 (controller adapted before ConfigMap support)
T038–T044 → T047–T049 (controller adapted before manifest updates)
T038–T044 → T050–T054 (controller adapted before package updates)
T050–T056 → T057–T061 (all code updated before build verification)

T057–T061 → T062–T071 (build clean before adoption logic)
T062–T071 → T072–T086 (adoption code before tests)
T087–T096 (docs) can run in parallel with T062–T086

T097–T099 must run last
```

## Parallel Execution Opportunities

**Within Phase 1** (after T003):
- T004, T005 can run in parallel (adoption helpers + tests)
- T011–T019 can all run in parallel (config scaffolding files)
- T020–T023 can all run in parallel (samples)
- T024–T029 can all run in parallel (build/CI)

**Within Phase 2** (after T038):
- T039, T040, T041, T042 can run in parallel (independent controller files)
- T047, T048, T049 can run in parallel (manifest files)
- T050, T051, T052, T053, T054 can run in parallel (package files)

**Across phases**:
- Phase 5 (docs) can run in parallel with Phase 3 + Phase 4
- T094, T095, T096 (spec updates) can run in parallel with everything

## Summary

- **Total tasks**: 104
- **Phase 1 (API — PR 1)**: 32 tasks (T001–T032)
- **Phase 2 (Controller — PR 2)**: 29 tasks (T033–T061)
- **Phase 3 (Adoption — PR 2)**: 11 tasks (T062–T071a)
- **Phase 4 (Tests — PR 2)**: 18 tasks (T072–T086, including T080a, T080b, T080c)
- **Phase 5 (Docs — PR 2)**: 10 tasks (T087–T096)
- **Phase 6 (Final verification)**: 3 tasks (T097–T099)
- **Config generation (PR 3)**: Deferred to spec 002 follow-up
