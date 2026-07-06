# Tasks: Operator-Generated Server Configuration (v1alpha2)

**Input**: Design documents from `/specs/002-operator-generated-config/`
**Prerequisites**: plan.md, spec.md, research.md, data-model.md, contracts/

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Phase 1: Setup

**Purpose**: Project scaffolding for v1alpha2 API version

- [ ] T001 (llama-stack-k8s-operator-6nd.1) Create v1alpha2 package scaffolding in api/v1alpha2/doc.go and api/v1alpha2/groupversion_info.go
- [ ] T002 (llama-stack-k8s-operator-6nd.2) Define v1alpha2 LlamaStackDistribution root type with kubebuilder markers in api/v1alpha2/llamastackdistribution_types.go
- [ ] T003 (llama-stack-k8s-operator-6nd.3) [P] Define DistributionSpec type (name/image mutually exclusive via CEL) in api/v1alpha2/llamastackdistribution_types.go
- [ ] T004 (llama-stack-k8s-operator-6nd.4) [P] Define ProvidersSpec and ProviderConfig types with typed []ProviderConfig slices in api/v1alpha2/llamastackdistribution_types.go
- [ ] T005 (llama-stack-k8s-operator-6nd.5) [P] Define SecretKeyRef type in api/v1alpha2/llamastackdistribution_types.go
- [ ] T006 (llama-stack-k8s-operator-6nd.6) [P] Define ResourcesSpec and ModelConfig types (name required, provider optional) in api/v1alpha2/llamastackdistribution_types.go
- [ ] T007 (llama-stack-k8s-operator-6nd.7) [P] Define StorageSpec, KVStorageSpec, SQLStorageSpec types with enum validation in api/v1alpha2/llamastackdistribution_types.go
- [ ] T008 (llama-stack-k8s-operator-6nd.8) [P] Define NetworkingSpec, TLSSpec, ExposeConfig, CABundleConfig types in api/v1alpha2/llamastackdistribution_types.go
- [ ] T009 (llama-stack-k8s-operator-6nd.9) [P] Define WorkloadSpec, WorkloadOverrides, AutoscalingSpec, PodDisruptionBudgetSpec types in api/v1alpha2/llamastackdistribution_types.go
- [ ] T010 (llama-stack-k8s-operator-6nd.10) [P] Define OverrideConfigSpec type in api/v1alpha2/llamastackdistribution_types.go
- [ ] T011 (llama-stack-k8s-operator-6nd.11) Define v1alpha2 status types (ResolvedDistributionStatus, ConfigGenerationStatus, condition constants) in api/v1alpha2/llamastackdistribution_types.go
- [ ] T012 (llama-stack-k8s-operator-6nd.12) Add CEL validation rules to LlamaStackDistributionSpec: mutual exclusivity, provider ID uniqueness, TLS/storage conditional fields, disabled+provider conflict in api/v1alpha2/llamastackdistribution_types.go
- [ ] T013 (llama-stack-k8s-operator-6nd.13) Run controller-gen to generate deepcopy and CRD manifests, verify CEL rules compile in config/crd/bases/llamastack.io_llamastackdistributions.yaml

---

## Phase 2: Foundational (Config Generation Engine)

**Purpose**: Core config generation pipeline that all user stories depend on

**CRITICAL**: No user story work can begin until this phase is complete

- [ ] T014 (llama-stack-k8s-operator-834.1) Define ConfigResolver interface and implement EmbeddedConfigResolver with go:embed in pkg/config/resolver.go
- [ ] T015 (llama-stack-k8s-operator-834.2) [P] Create embedded default config files for starter distribution in pkg/deploy/configs/starter/config.yaml
- [ ] T016 (llama-stack-k8s-operator-834.3) [P] Create embedded default config files for remote-vllm distribution in pkg/deploy/configs/remote-vllm/config.yaml
- [ ] T017 (llama-stack-k8s-operator-834.4) [P] Implement config version detection in pkg/config/version.go
- [ ] T018 (llama-stack-k8s-operator-834.5) Implement base config parsing and merge logic (full API-type replacement for providers, per-subsection for storage, additive for resources, subtractive for disabled) in pkg/config/merge.go
- [ ] T019 (llama-stack-k8s-operator-834.6) [P] Implement provider expansion (auto-ID generation, remote:: prefix, endpoint mapping, settings merge) in pkg/config/provider.go
- [ ] T020 (llama-stack-k8s-operator-834.7) [P] Implement secret reference collection from apiKey and secretRefs fields, env var name generation (LLSD_<ID>_<FIELD>) in pkg/config/secret.go
- [ ] T021 (llama-stack-k8s-operator-834.8) [P] Implement resource expansion (model registration with provider assignment, tool group registration, shield registration) in pkg/config/resource.go
- [ ] T022 (llama-stack-k8s-operator-834.9) [P] Implement storage config generation (kv and sql backend mapping) in pkg/config/storage.go
- [ ] T023 (llama-stack-k8s-operator-834.10) Implement GenerateConfig orchestrator that calls all expansion functions and produces GeneratedConfig (configYAML, contentHash, envVars, counts) in pkg/config/generator.go
- [ ] T024 (llama-stack-k8s-operator-834.11) [P] Write unit tests for EmbeddedConfigResolver in pkg/config/resolver_test.go
- [ ] T025 (llama-stack-k8s-operator-834.12) [P] Write unit tests for provider expansion (single provider, multiple providers, auto-ID, settings merge, secretRefs) in pkg/config/provider_test.go
- [ ] T026 (llama-stack-k8s-operator-834.13) [P] Write unit tests for resource expansion (model with/without provider, tools, shields, missing provider error) in pkg/config/resource_test.go
- [ ] T027 (llama-stack-k8s-operator-834.14) [P] Write unit tests for secret collection (apiKey, secretRefs, storage secrets, env var naming normalization) in pkg/config/secret_test.go
- [ ] T028 (llama-stack-k8s-operator-834.15) [P] Write unit tests for storage config (sqlite defaults, redis with endpoint, postgres with connectionString) in pkg/config/storage_test.go
- [ ] T029 (llama-stack-k8s-operator-834.16) [P] Write unit tests for merge logic (provider replacement, storage merge, resource additive, disabled subtractive) in pkg/config/merge_test.go
- [ ] T030 (llama-stack-k8s-operator-834.17) Write integration tests for GenerateConfig end-to-end with golden file comparison for determinism in pkg/config/generator_test.go

**Checkpoint**: Config generation pipeline is complete and independently testable via unit tests

---

## Phase 3: User Story 1 - Simple Inference Configuration (Priority: P1) MVP

**Goal**: Deploy a llama-stack instance with a vLLM backend using minimal YAML, operator generates config.yaml

**Independent Test**: Deploy a LLSD CR with minimal `providers.inference` configuration and verify the server starts with the provider accessible via the `/v1/providers` API

### Implementation for User Story 1

- [ ] T031 (llama-stack-k8s-operator-db3.1) [US1] Implement ConfigMap reconciler: create immutable ConfigMap with content-hash name, set owner reference, compare hash for no-op detection in controllers/configmap_reconciler.go
- [ ] T032 (llama-stack-k8s-operator-db3.2) [US1] Implement ConfigMap cleanup: delete generated ConfigMaps beyond the last 2 during reconciliation in controllers/configmap_reconciler.go
- [ ] T033 (llama-stack-k8s-operator-db3.3) [US1] Implement v1alpha2 controller helpers: determine config path (overrideConfig vs generate vs default), resolve distribution name to image in controllers/v1alpha2_helpers.go
- [ ] T034 (llama-stack-k8s-operator-db3.4) [US1] Modify reconcile loop to support v1alpha2 CRs: add config generation path between distribution resolution and Deployment creation in controllers/llamastackdistribution_controller.go
- [ ] T035 (llama-stack-k8s-operator-db3.5) [US1] Implement atomic Deployment update: apply image, ConfigMap volume, env vars, and hash annotation in a single client.Update() call in controllers/llamastackdistribution_controller.go
- [ ] T036 (llama-stack-k8s-operator-db3.6) [US1] Wire ConfigGenerated status condition (True on success, False on failure with reason) in controllers/status.go
- [ ] T037 (llama-stack-k8s-operator-db3.7) [US1] Emit Kubernetes Events for config generation success and failure (NFR-007) in controllers/llamastackdistribution_controller.go
- [ ] T038 (llama-stack-k8s-operator-db3.8) [US1] Write unit tests for ConfigMap reconciler (create, hash comparison, cleanup) in controllers/configmap_reconciler_test.go
- [ ] T039 (llama-stack-k8s-operator-db3.9) [US1] Write unit tests for v1alpha2 helpers (config path routing, distribution resolution) in controllers/v1alpha2_helpers_test.go
- [ ] T040 (llama-stack-k8s-operator-db3.10) [US1] Write controller test: minimal v1alpha2 CR with single inference provider generates ConfigMap and creates Deployment in controllers/llamastackdistribution_controller_test.go
- [ ] T041 (llama-stack-k8s-operator-db3.11) [US1] Create v1alpha2 sample CR: minimal inference setup in config/samples/v1alpha2/minimal-inference.yaml

**Checkpoint**: User Story 1 complete. Minimal v1alpha2 CR with one provider generates config and deploys.

---

## Phase 4: User Story 2 - Multiple Providers Configuration (Priority: P1)

**Goal**: Configure multiple inference providers (primary and fallback) in a single LLSD with explicit IDs

**Independent Test**: Deploy a LLSD CR with multiple inference providers using list form, verify all providers appear in the `/v1/providers` API

### Implementation for User Story 2

- [ ] T042 (llama-stack-k8s-operator-een.1) [US2] Write controller test: v1alpha2 CR with two inference providers with explicit IDs, verify both appear in generated config in controllers/llamastackdistribution_controller_test.go
- [ ] T043 (llama-stack-k8s-operator-een.2) [US2] Write controller test: v1alpha2 CR with duplicate provider IDs rejected by CEL validation in controllers/llamastackdistribution_controller_test.go
- [ ] T044 (llama-stack-k8s-operator-een.3) [US2] Create v1alpha2 sample CR: multiple providers with explicit IDs in config/samples/v1alpha2/multiple-providers.yaml

**Checkpoint**: User Story 2 complete. Multiple providers per API type work with ID validation.

---

## Phase 5: User Story 3 - Resource Registration (Priority: P1)

**Goal**: Register models and tools declaratively in the CR, available immediately on server start

**Independent Test**: Deploy a LLSD CR with `resources.models` and `resources.tools`, verify resources appear in the respective API endpoints

### Implementation for User Story 3

- [ ] T045 (llama-stack-k8s-operator-onm.1) [US3] Write controller test: CR with models (default provider assignment) and tools, verify registered_resources in generated config in controllers/llamastackdistribution_controller_test.go
- [ ] T046 (llama-stack-k8s-operator-onm.2) [US3] Write controller test: CR with model specifying explicit provider assignment in controllers/llamastackdistribution_controller_test.go
- [ ] T047 (llama-stack-k8s-operator-onm.3) [US3] Write controller test: tools without toolRuntime provider fails with actionable error (FR-043) in controllers/llamastackdistribution_controller_test.go
- [ ] T048 (llama-stack-k8s-operator-onm.4) [US3] Create v1alpha2 sample CR: models and tools registration in config/samples/v1alpha2/with-resources.yaml

**Checkpoint**: User Story 3 complete. Models, tools, and shields are registered in generated config.

---

## Phase 6: User Story 4 - State Storage Configuration (Priority: P1)

**Goal**: Configure PostgreSQL/Redis for state storage via CR spec

**Independent Test**: Deploy a LLSD CR with `storage.sql` configuration, verify the server uses PostgreSQL for state storage

### Implementation for User Story 4

- [ ] T049 (llama-stack-k8s-operator-l4o.1) [US4] Write controller test: CR with postgres storage and secretKeyRef, verify storage section in config and env vars on Deployment in controllers/llamastackdistribution_controller_test.go
- [ ] T050 (llama-stack-k8s-operator-l4o.2) [US4] Write controller test: CR with redis kv storage, verify endpoint in config in controllers/llamastackdistribution_controller_test.go
- [ ] T051 (llama-stack-k8s-operator-l4o.3) [US4] Write controller test: CR without storage section preserves distribution defaults in controllers/llamastackdistribution_controller_test.go
- [ ] T052 (llama-stack-k8s-operator-l4o.4) [US4] Create v1alpha2 sample CR: postgres state storage in config/samples/v1alpha2/with-postgres-storage.yaml

**Checkpoint**: User Story 4 complete. Storage configuration generates correct config.yaml sections.

---

## Phase 7: User Story 8 - Runtime Configuration Updates (Priority: P1)

**Goal**: Update LLSD CR and have running instance pick up changes automatically

**Independent Test**: Deploy a LLSD CR, wait for Ready, modify the CR's providers section, verify the Pod restarts with the updated config.yaml

### Implementation for User Story 8

- [ ] T053 (llama-stack-k8s-operator-9v9.1) [US8] Implement no-op detection: skip Pod restart when generated config.yaml content hash is identical (FR-096) in controllers/configmap_reconciler.go
- [ ] T054 (llama-stack-k8s-operator-9v9.2) [US8] Implement failure preservation: on config generation error, keep current running Deployment unchanged, set ConfigGenerated=False (FR-097) in controllers/llamastackdistribution_controller.go
- [ ] T055 (llama-stack-k8s-operator-9v9.3) [US8] Wire DeploymentUpdated and SecretsResolved status conditions in controllers/status.go
- [ ] T056 (llama-stack-k8s-operator-9v9.4) [US8] Write controller test: modify CR providers, verify new ConfigMap created and Deployment updated with new hash annotation in controllers/llamastackdistribution_controller_test.go
- [ ] T057 (llama-stack-k8s-operator-9v9.5) [US8] Write controller test: modify CR with identical config output, verify no Pod restart in controllers/llamastackdistribution_controller_test.go
- [ ] T058 (llama-stack-k8s-operator-9v9.6) [US8] Write controller test: modify CR with invalid config, verify running Deployment preserved and error in status in controllers/llamastackdistribution_controller_test.go

**Checkpoint**: User Story 8 complete. Runtime updates work with no-op detection and failure preservation.

---

## Phase 8: User Story 5 - Network Exposure Configuration (Priority: P2)

**Goal**: Expose llama-stack service externally with TLS

**Independent Test**: Deploy a LLSD CR with `networking.expose: {enabled: true}` and `networking.tls`, verify Ingress/Route is created with TLS configured

### Implementation for User Story 5

- [ ] T059 (llama-stack-k8s-operator-khz.1) [US5] Implement networking overrides: apply port from spec to server config and Deployment containerPort in controllers/v1alpha2_helpers.go
- [ ] T060 (llama-stack-k8s-operator-khz.2) [US5] Extend existing Ingress/Route reconciliation for v1alpha2 ExposeConfig (enabled + hostname fields) in controllers/network_resources.go
- [ ] T061 (llama-stack-k8s-operator-khz.3) [US5] Write controller test: CR with expose enabled creates Ingress with auto-generated hostname in controllers/network_resources_test.go
- [ ] T062 (llama-stack-k8s-operator-khz.4) [US5] Write controller test: CR with expose hostname creates Ingress with specified hostname in controllers/network_resources_test.go
- [ ] T063 (llama-stack-k8s-operator-khz.5) [US5] Create v1alpha2 sample CR: TLS and expose configuration in config/samples/v1alpha2/with-networking.yaml

**Checkpoint**: User Story 5 complete. Networking exposure works with TLS.

---

## Phase 9: User Story 6 - Full ConfigMap Override (Priority: P2)

**Goal**: Power users can provide their own complete config.yaml via ConfigMap

**Independent Test**: Deploy a LLSD CR with `overrideConfig.configMapName`, verify the server uses the ConfigMap contents

### Implementation for User Story 6

- [ ] T064 (llama-stack-k8s-operator-8xi.1) [US6] Implement overrideConfig path: use referenced ConfigMap contents as config.yaml directly, skip generation in controllers/v1alpha2_helpers.go
- [ ] T065 (llama-stack-k8s-operator-8xi.2) [US6] Write controller test: CR with overrideConfig uses referenced ConfigMap in controllers/llamastackdistribution_controller_test.go
- [ ] T066 (llama-stack-k8s-operator-8xi.3) [US6] Write controller test: CR with both providers and overrideConfig rejected by CEL in controllers/llamastackdistribution_controller_test.go
- [ ] T067 (llama-stack-k8s-operator-8xi.4) [US6] Create v1alpha2 sample CR: full ConfigMap override in config/samples/v1alpha2/with-override-config.yaml

**Checkpoint**: User Story 6 complete. Override escape hatch works.

---

## Phase 10: User Story 7 - Migration from v1alpha1 (Priority: P2)

**Goal**: Existing v1alpha1 CRs continue working after operator upgrade

**Independent Test**: Apply a v1alpha1 CR, upgrade operator, verify the CR continues to work and can be retrieved as v1alpha2

### Implementation for User Story 7

- [ ] T068 (llama-stack-k8s-operator-wwz.1) [US7] Implement v1alpha2 hub conversion marker in api/v1alpha2/llamastackdistribution_conversion.go
- [ ] T069 (llama-stack-k8s-operator-wwz.2) [US7] Implement v1alpha1 spoke ConvertTo (v1alpha1 → v1alpha2) with field mapping per migration table in api/v1alpha1/llamastackdistribution_conversion.go
- [ ] T070 (llama-stack-k8s-operator-wwz.3) [US7] Implement v1alpha1 spoke ConvertFrom (v1alpha2 → v1alpha1) with annotation preservation for v1alpha2-only fields in api/v1alpha1/llamastackdistribution_conversion.go
- [ ] T071 (llama-stack-k8s-operator-wwz.4) [US7] Write round-trip conversion tests: v1alpha1 → v1alpha2 → v1alpha1, verify field preservation in api/v1alpha1/llamastackdistribution_conversion_test.go
- [ ] T072 (llama-stack-k8s-operator-wwz.5) [US7] Write conversion test: v1alpha2 with new fields → v1alpha1 → v1alpha2, verify annotation round-trip in api/v1alpha1/llamastackdistribution_conversion_test.go
- [ ] T073 (llama-stack-k8s-operator-wwz.6) [US7] Move existing v1alpha1 sample CRs to config/samples/v1alpha1/ subdirectory

**Checkpoint**: User Story 7 complete. v1alpha1 CRs continue working via conversion webhook.

---

## Phase 11: Webhooks & Deployment

**Purpose**: Validating webhook and kustomize deployment manifests

- [ ] T074 (llama-stack-k8s-operator-7hy.1) Implement validating webhook: distribution name validation against distributions.json in api/v1alpha2/llamastackdistribution_webhook.go
- [ ] T075 (llama-stack-k8s-operator-7hy.2) Implement validating webhook: Secret existence checks for all secretKeyRef references in api/v1alpha2/llamastackdistribution_webhook.go
- [ ] T076 (llama-stack-k8s-operator-7hy.3) Implement validating webhook: ConfigMap existence for overrideConfig and caBundle in api/v1alpha2/llamastackdistribution_webhook.go
- [ ] T077 (llama-stack-k8s-operator-7hy.4) Implement validating webhook: provider ID cross-references in resources.models[].provider in api/v1alpha2/llamastackdistribution_webhook.go
- [ ] T078 (llama-stack-k8s-operator-7hy.5) Implement validating webhook: disabled+provider conflict check in api/v1alpha2/llamastackdistribution_webhook.go
- [ ] T079 (llama-stack-k8s-operator-7hy.6) [P] Write webhook tests: valid CRs accepted, invalid distribution name rejected with available names in api/v1alpha2/llamastackdistribution_webhook_test.go
- [ ] T080 (llama-stack-k8s-operator-7hy.7) [P] Write webhook tests: missing Secret rejected, missing ConfigMap rejected, invalid provider reference rejected in api/v1alpha2/llamastackdistribution_webhook_test.go
- [ ] T081 (llama-stack-k8s-operator-7hy.8) [P] Create cert-manager manifests for webhook TLS in config/certmanager/
- [ ] T082 (llama-stack-k8s-operator-7hy.9) [P] Create CRD webhook patch in config/crd/patches/webhook_in_llamastackdistributions.yaml
- [ ] T083 (llama-stack-k8s-operator-7hy.10) [P] Create manager webhook volume patch in config/default/manager_webhook_patch.yaml
- [ ] T084 (llama-stack-k8s-operator-7hy.11) Update config/default/kustomization.yaml to enable webhook and certmanager
- [ ] T085 (llama-stack-k8s-operator-7hy.12) [P] Create OpenShift overlay for service-ca webhook certificates in config/openshift/

**Checkpoint**: Webhooks deployed, admission validation working.

---

## Phase 12: Polish & Cross-Cutting Concerns

**Purpose**: E2E tests, documentation, cleanup

- [ ] T086 (llama-stack-k8s-operator-app.1) [P] Write E2E test: deploy v1alpha2 CR with inference provider, verify server starts and responds in tests/e2e/
- [ ] T087 (llama-stack-k8s-operator-app.2) [P] Write E2E test: update v1alpha2 CR, verify rolling update with new config in tests/e2e/
- [ ] T088 (llama-stack-k8s-operator-app.3) [P] Create v1alpha2 migration documentation in docs/migration-v1alpha1-to-v1alpha2.md
- [ ] T089 (llama-stack-k8s-operator-app.4) Update config/samples/kustomization.yaml to reference new v1alpha2 samples
- [ ] T090 (llama-stack-k8s-operator-app.5) Wire Available status condition based on Deployment readiness in controllers/status.go
- [ ] T091 (llama-stack-k8s-operator-app.6) Run quickstart.md validation: verify all 7 examples parse as valid YAML and match CRD schema

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies, can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion, BLOCKS all user stories
- **User Stories (Phases 3-10)**: All depend on Foundational phase completion
  - US1 (Phase 3): No story dependencies, MVP target
  - US2 (Phase 4): Builds on US1 controller (same file, sequential)
  - US3 (Phase 5): Builds on US1 controller (same file, sequential)
  - US4 (Phase 6): Builds on US1 controller (same file, sequential)
  - US8 (Phase 7): Builds on US1 controller (same file, sequential)
  - US5 (Phase 8): Independent from US2-US4 (different files), can parallel with US2+
  - US6 (Phase 9): Builds on US1 controller (same file), can parallel with US5
  - US7 (Phase 10): Independent (different package), can parallel with US2+
- **Webhooks (Phase 11)**: Depends on Phase 1 types, can parallel with US2+
- **Polish (Phase 12)**: Depends on all phases

### Parallel Opportunities

After Phase 2 (Foundational), these can run in parallel:
- **Stream A**: US1 → US2 → US3 → US4 → US8 (controller modifications, sequential)
- **Stream B**: US7 (conversion webhook, separate package)
- **Stream C**: Phase 11 webhooks (separate package)
- **Stream D**: US5 (networking, mostly separate files)

---

## Parallel Example: Phase 2 (Foundational)

```bash
# These can all run in parallel (different files):
Task: "Implement EmbeddedConfigResolver in pkg/config/resolver.go"
Task: "Create embedded config for starter in pkg/deploy/configs/starter/config.yaml"
Task: "Implement provider expansion in pkg/config/provider.go"
Task: "Implement secret collection in pkg/config/secret.go"
Task: "Implement resource expansion in pkg/config/resource.go"
Task: "Implement storage config in pkg/config/storage.go"
Task: "Implement config version detection in pkg/config/version.go"

# Then sequentially (depends on above):
Task: "Implement merge logic in pkg/config/merge.go"
Task: "Implement GenerateConfig orchestrator in pkg/config/generator.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (v1alpha2 types + CEL rules)
2. Complete Phase 2: Foundational (config generation pipeline)
3. Complete Phase 3: User Story 1 (controller integration)
4. **STOP and VALIDATE**: Deploy minimal v1alpha2 CR, verify config.yaml generated and server starts
5. This is a deployable, testable increment

### Incremental Delivery

1. Setup + Foundational → Config pipeline works in isolation
2. Add US1 → Minimal inference config works end-to-end (MVP!)
3. Add US2-US4 → Multiple providers, resources, storage
4. Add US8 → Runtime updates with no-op detection
5. Add US5-US6 → Networking and override escape hatch
6. Add US7 → v1alpha1 backward compatibility
7. Add webhooks → Admission-time validation
8. Polish → E2E tests, docs, cleanup

### PR Strategy

Split into focused PRs aligned with phases:
- **PR 1**: Phase 1 + Phase 2 (types + config pipeline, ~15 files)
- **PR 2**: Phase 3 (US1 controller integration, ~8 files)
- **PR 3**: Phases 4-7 (US2-US4, US8 controller tests, ~5 files)
- **PR 4**: Phases 8-10 (US5 networking, US6 override, US7 conversion, ~10 files)
- **PR 5**: Phase 11-12 (webhooks, e2e, docs, ~15 files)

---

## Notes

- [P] tasks = different files, no dependencies
- [Story] label maps task to specific user story for traceability
- Each user story should be independently completable and testable
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Config generation pipeline (Phase 2) is pure Go with no K8s dependencies, enabling fast unit tests

## Beads Task Management

This project uses beads (`bd`) for persistent task tracking across sessions:
- Run `/sdd:beads-task-sync` to create bd issues from this file
- `bd ready --json` returns unblocked tasks (dependencies resolved)
- `bd close <id>` marks a task complete (use `-r "reason"` for close reason, NOT `--comment`)
- `bd comments add <id> "text"` adds a detailed comment to an issue
- `bd backup` persists state to git
- `bd create "DISCOVERED: [short title]" --labels discovered` tracks new work
  - Keep titles crisp (under 80 chars); add details via `bd comments add <id> "details"`
- Run `/sdd:beads-task-sync --reverse` to update checkboxes from bd state
- **Always use `jq` to parse bd JSON output, NEVER inline Python one-liners**
