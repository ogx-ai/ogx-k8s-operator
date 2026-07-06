# Tasks: Deploy-Time Modularity - Level 1

**Input**: Design documents from `/specs/001-deploy-time-providers-l1/`
**Prerequisites**: plan.md (required), spec.md (required)

**Tests**: This feature includes comprehensive unit, integration, and E2E tests as specified in the plan.

**Organization**: Tasks are grouped by user story to enable independent implementation and testing of each story.

**Note on Phase Organization**: Task phases are organized by user story (not 1:1 with plan.md implementation phases) to enable independent parallel development. Plan uses technical component phases (CRD → Metadata → Init Containers → Merge → Controller → Status → Testing), while tasks group by user story for clearer acceptance testing.

## Format: `[ID] [P?] [Story] Description`

- **[P]**: Can run in parallel (different files, no dependencies)
- **[Story]**: Which user story this task belongs to (e.g., US1, US2, US3)
- Include exact file paths in descriptions

## Path Conventions

This is a Kubernetes operator project in Go:
- **API types**: `api/v1alpha1/`
- **Controllers**: `controllers/`
- **Packages**: `pkg/provider/`, `pkg/deploy/`
- **Tests**: `tests/unit/`, `tests/integration/`

---

## Phase 1: Setup (Shared Infrastructure)

**Purpose**: CRD updates and basic structure for external provider support

- [ ] T001 Add ExternalProvidersSpec struct to api/v1alpha1/llamastackdistribution_types.go
- [ ] T002 Add ExternalProviderRef struct with validation tags to api/v1alpha1/llamastackdistribution_types.go
- [ ] T003 Add ExternalProviderStatus struct to api/v1alpha1/llamastackdistribution_types.go
- [ ] T004 Generate CRD manifests with `make manifests`
- [ ] T005 [P] Create pkg/provider/ directory for provider-specific logic
- [ ] T006 [P] Create pkg/deploy/ subdirectories if needed for deployment logic

---

## Phase 2: Foundational (Blocking Prerequisites)

**Purpose**: Core provider metadata and validation infrastructure that ALL user stories depend on

**⚠️ CRITICAL**: No user story work can begin until this phase is complete

- [ ] T007 Create ProviderMetadata struct in pkg/provider/metadata.go
- [ ] T008 Implement LoadProviderMetadata() function in pkg/provider/metadata.go
- [ ] T009 Implement ValidateMetadata() function in pkg/provider/validation.go
- [ ] T010 [P] Create metadata parsing unit tests in tests/unit/metadata_test.go
- [ ] T011 [P] Create validation unit tests in tests/unit/validation_test.go

**Checkpoint**: Foundation ready - user story implementation can now begin in parallel

---

## Phase 3: User Story 1 - Deploy Custom Provider (Priority: P1) 🎯 MVP

**Goal**: Enable deployment of llama-stack with external provider packages injected at pod startup

**Independent Test**: Deploy a LLSD CR with one external provider and verify:
1. Init container successfully installs provider packages
2. Provider appears in `/v1/providers` API endpoint
3. Provider is functional through llama-stack API

### Implementation for User Story 1

#### Init Container Generation

**Architecture**: Two-phase init container approach (not three):
- **Phase 1**: Provider install containers (N containers, generated in CRD order)
- **Phase 2**: Merge config container (1 container)

- [ ] T012 [P] [US1] Create GenerateInitContainers() function in pkg/deploy/initcontainer.go
- [ ] T013 [P] [US1] Create collectProvidersInCRDOrder() helper function - collect providers in CRD specification order in pkg/deploy/initcontainer.go
- [ ] T014 [US1] Implement generateProviderInitContainer() to generate provider install init containers (Phase 1) in pkg/deploy/initcontainer.go
- [ ] T015 [US1] Implement AddExternalProvidersVolume() for emptyDir volume in pkg/deploy/initcontainer.go
- [ ] T016 [US1] Implement MountExternalProvidersVolume() for main container in pkg/deploy/initcontainer.go
- [ ] T017 [US1] Implement UpdatePythonPath() to prepend external provider path in pkg/deploy/initcontainer.go
- [ ] T017a [US1] Create generateMergeConfigInitContainer() for Phase 2 merge step in pkg/deploy/initcontainer.go

#### run.yaml Merging

- [ ] T018 [P] [US1] Create RunYamlConfig struct in pkg/deploy/runyaml.go
- [ ] T019 [P] [US1] Create ProviderConfigEntry struct in pkg/deploy/runyaml.go
- [ ] T020 [US1] Implement MergeRunYaml() function with base → user → external merge logic in pkg/deploy/runyaml.go
- [ ] T021 [US1] Implement mergeExternalProviders() function in pkg/deploy/runyaml.go
- [ ] T022 [US1] Implement validateNoDuplicateExternalProviderIDs() in pkg/deploy/runyaml.go
- [ ] T023 [US1] Implement findProviderIndexByID() helper in pkg/deploy/runyaml.go
- [ ] T024 [US1] Create APIPlacementError type with formatted error message in pkg/deploy/runyaml.go
- [ ] T024a [P] [US1] Create ExtraProvidersYaml struct in pkg/provider/extra_providers.go
- [ ] T024b [P] [US1] Implement GenerateExtraProvidersFromMetadata() function in pkg/provider/extra_providers.go
- [ ] T024c [US1] Unit test extra-providers.yaml generation in tests/unit/extra_providers_test.go

#### Merge Tool Binary (Operator Image)

- [ ] T024d [P] [US1] Create cmd/merge-run-yaml/main.go with CLI argument parsing
- [ ] T024e [US1] Implement merge logic: generate extra-providers.yaml from metadata
- [ ] T024f [US1] Implement merge logic: combine user run.yaml + extra-providers.yaml
- [ ] T024g [US1] Update Dockerfile to include merge-run-yaml binary in operator image
- [ ] T024h [P] [US1] Unit test merge tool logic in tests/unit/merge_tool_test.go

#### Controller Integration

- [ ] T025 [US1] Update buildManifestContext() to call GenerateInitContainers() in controllers/llamastackdistribution_controller.go
- [ ] T026 [US1] Add init containers to pod template spec in deployment generation in controllers/llamastackdistribution_controller.go
- [ ] T027 [US1] Add external providers volume to pod spec in controllers/llamastackdistribution_controller.go
- [ ] T028 [US1] Mount external providers volume in main container in controllers/llamastackdistribution_controller.go
- [ ] T029 [US1] Update PYTHONPATH environment variable in main container in controllers/llamastackdistribution_controller.go
- [ ] T030 [US1] Call MergeRunYaml() during run.yaml generation in controllers/llamastackdistribution_controller.go

#### Status Tracking

- [ ] T031 [P] [US1] Create updateExternalProviderStatus() function in controllers/external_providers.go
- [ ] T032 [P] [US1] Implement getCurrentPod() helper in controllers/external_providers.go
- [ ] T033 [US1] Implement findInitContainerStatus() helper in controllers/external_providers.go
- [ ] T034 [US1] Implement extractErrorMessage() for init container failures in controllers/external_providers.go
- [ ] T035 [US1] Call updateExternalProviderStatus() in reconciliation loop in controllers/llamastackdistribution_controller.go
- [ ] T036 [US1] Update LLSD conditions based on external provider status in controllers/llamastackdistribution_controller.go

#### Testing

- [ ] T037 [P] [US1] Unit test init container generation in tests/unit/initcontainer_test.go
- [ ] T038 [P] [US1] Unit test init container ordering (CRD order by API type) in tests/unit/initcontainer_test.go
- [ ] T039 [P] [US1] Unit test volume mount generation in tests/unit/initcontainer_test.go
- [ ] T040 [P] [US1] Unit test run.yaml merge scenarios in tests/unit/merge_test.go
- [ ] T041 [P] [US1] Unit test duplicate provider ID detection in tests/unit/merge_test.go
- [ ] T042 [P] [US1] Unit test API placement validation in tests/unit/merge_test.go
- [ ] T043 [US1] Integration test: single external provider installation in tests/integration/external_providers_test.go
- [ ] T044 [US1] Integration test: LLSD status reflects init container progress in tests/integration/external_providers_test.go
- [ ] T045 [US1] E2E test: deploy LLSD with external provider and verify /v1/providers endpoint in tests/integration/external_providers_test.go

#### Documentation

- [ ] T046 [P] [US1] Create example LLSD YAML with single external provider in config/samples/
- [ ] T047 [P] [US1] Document lls-provider-spec.yaml format in docs/external-providers.md
- [ ] T048 [P] [US1] Document provider image creation process in docs/external-providers.md
- [ ] T048a [P] [US1] Document extra-providers.yaml schema in docs/external-providers.md
- [ ] T048b [P] [US1] Document forward compatibility plan (Phase 2 migration) in docs/external-providers.md
- [ ] T048c [P] [US1] Document that FR-001 to FR-005 (Provider Image Contract) are requirements for provider authors, not operator implementation, in docs/external-providers.md

**Note**: FR-001 to FR-005 define the provider image contract - requirements that provider authors must follow when packaging providers. The operator validates these requirements at runtime (via metadata parsing and preflight), but doesn't implement them.

**Checkpoint**: At this point, User Story 1 should be fully functional - users can deploy custom providers

---

## Phase 4: User Story 2 - Override Distribution Providers (Priority: P2)

**Goal**: Allow external providers to override built-in distribution providers with same provider ID

**Independent Test**: Deploy LLSD with base distribution, add external provider with same ID as distribution provider, verify external provider takes precedence and warning is logged

### Implementation for User Story 2

- [ ] T049 [US2] Implement provider override logic with precedence rules in mergeExternalProviders() in pkg/deploy/runyaml.go
- [ ] T050 [US2] Add warning logging when external provider overrides base provider in pkg/deploy/runyaml.go
- [ ] T051 [US2] Add warning logging when external provider overrides ConfigMap provider in pkg/deploy/runyaml.go
- [ ] T052 [P] [US2] Unit test provider override precedence (external > ConfigMap > base) in tests/unit/merge_test.go
- [ ] T053 [P] [US2] Unit test warning message generation for overrides in tests/unit/merge_test.go
- [ ] T054 [US2] Integration test: external provider overrides distribution provider in tests/integration/external_providers_test.go
- [ ] T055 [US2] Integration test: verify warning appears in operator logs in tests/integration/external_providers_test.go
- [ ] T056 [P] [US2] Create example LLSD YAML with provider override in config/samples/
- [ ] T057 [P] [US2] Document provider override behavior in docs/external-providers.md

**Checkpoint**: At this point, User Stories 1 AND 2 should both work independently

---

## Phase 5: User Story 3 - Diagnose Provider Failures (Priority: P2)

**Goal**: Provide clear, actionable error messages when provider installation or validation fails

**Independent Test**: Trigger various failure scenarios and verify error messages in LLSD status match specification format

### Implementation for User Story 3

#### Error Message Formatting

- [ ] T058 [P] [US3] Implement metadata file missing error message in init container script in pkg/deploy/initcontainer.go
- [ ] T059 [P] [US3] Implement package directory missing error message in init container script in pkg/deploy/initcontainer.go
- [ ] T060 [US3] Implement dependency conflict error formatting per spec in pkg/deploy/runyaml.go
- [ ] T061 [US3] Implement API placement error formatting per spec (already in APIPlacementError) in pkg/deploy/runyaml.go
- [ ] T062 [US3] Enhance extractErrorMessage() to parse and format init container errors in controllers/external_providers.go

#### Error Scenario Testing

- [ ] T063 [P] [US3] Integration test: missing lls-provider-spec.yaml file in tests/integration/external_providers_test.go
- [ ] T064 [P] [US3] Integration test: invalid YAML in metadata file in tests/integration/external_providers_test.go
- [ ] T065 [P] [US3] Integration test: missing packages directory in tests/integration/external_providers_test.go
- [ ] T066 [P] [US3] Integration test: pip install failure (simulated) in tests/integration/external_providers_test.go
- [ ] T067 [P] [US3] Integration test: API placement mismatch error in tests/integration/external_providers_test.go
- [ ] T068 [P] [US3] Integration test: duplicate external provider IDs in tests/integration/external_providers_test.go
- [ ] T069 [US3] Verify all error messages include: provider ID, image, context, resolution in tests/integration/external_providers_test.go

#### Documentation

- [ ] T070 [P] [US3] Create troubleshooting guide with all error scenarios in docs/troubleshooting.md
- [ ] T071 [P] [US3] Document error message formats and meanings in docs/troubleshooting.md
- [ ] T072 [P] [US3] Add example debugging workflows in docs/troubleshooting.md

**Checkpoint**: All three user stories should now be independently functional with excellent error handling

---

## Phase 6: Edge Cases & Validation

**Purpose**: Handle edge cases and implement preflight validation integration

### Edge Case Handling

- [ ] T073 [P] Implement provider image update handling (rolling update trigger) in controllers/llamastackdistribution_controller.go
- [ ] T074 [P] Test metadata field mismatch warning generation (metadata vs get_provider_spec()) in tests/integration/external_providers_test.go
- [ ] T075 [P] Test external provider package precedence (PYTHONPATH ordering) in tests/integration/external_providers_test.go
- [ ] T076 Test pod restart behavior when external provider updated in tests/integration/external_providers_test.go

### Preflight Integration (Assumes lls-preflight-spec.md implemented)

- [ ] T077 Add preflight validation command invocation before server starts in controllers/llamastackdistribution_controller.go
- [ ] T078 Handle preflight validation failures with clear status updates in controllers/llamastackdistribution_controller.go
- [ ] T079 [P] Test architecture mismatch detection via preflight in tests/integration/external_providers_test.go
- [ ] T080 [P] Test import failure detection via preflight in tests/integration/external_providers_test.go
- [ ] T081 Test provider spec validation via preflight in tests/integration/external_providers_test.go

---

## Phase 7: Polish & Cross-Cutting Concerns

**Purpose**: Finalize implementation with examples, docs, and comprehensive testing

### Examples & Samples

- [ ] T082 [P] Create sample provider image Containerfile in docs/examples/
- [ ] T083 [P] Create sample lls-provider-spec.yaml in docs/examples/
- [ ] T084 [P] Create sample wheel packaging script in docs/examples/
- [ ] T085 [P] Create complete LLSD example with multiple external providers in config/samples/

### Documentation

- [ ] T086 [P] Write provider image creation guide in docs/provider-development.md
- [ ] T087 [P] Write CRD API reference for externalProviders fields in docs/api-reference.md
- [ ] T088 [P] Write migration guide from custom images to external providers in docs/migration.md
- [ ] T089 Update main README.md with external providers overview

### Final Testing & Validation

- [ ] T090 [P] Performance test: measure init container startup time per provider in tests/integration/
- [ ] T091 [P] Test multiple providers same API type in tests/integration/external_providers_test.go
- [ ] T092 [P] Test multiple providers different API types in tests/integration/external_providers_test.go
- [ ] T093 Test deterministic init container ordering in tests/integration/external_providers_test.go
- [ ] T094 Run full test suite and verify all tests pass
- [ ] T095 Verify CRD validation rules work correctly (invalid provider IDs, etc.)

### Code Quality

- [ ] T096 [P] Run golangci-lint and fix any issues
- [ ] T097 [P] Add godoc comments to all public functions in pkg/provider/ and pkg/deploy/
- [ ] T098 Review code for security issues (command injection, path traversal, etc.)
- [ ] T099 Ensure all error paths are tested and logged appropriately

---

## Dependencies & Execution Order

### Phase Dependencies

- **Setup (Phase 1)**: No dependencies - can start immediately
- **Foundational (Phase 2)**: Depends on Setup completion - BLOCKS all user stories
- **User Stories (Phase 3-5)**: All depend on Foundational phase completion
  - User Story 1 (P1): Core functionality - highest priority
  - User Story 2 (P2): Provider override - can start in parallel with US1 or after
  - User Story 3 (P2): Error handling - can start in parallel with US1/US2 or after
- **Edge Cases (Phase 6)**: Depends on User Stories 1-3 being complete
- **Polish (Phase 7)**: Depends on all core functionality being complete

### User Story Dependencies

- **User Story 1 (P1)**: Can start after Foundational (Phase 2) - No dependencies on other stories
- **User Story 2 (P2)**: Depends on User Story 1 (extends MergeRunYaml logic) - Some overlap acceptable
- **User Story 3 (P2)**: Can start after Foundational (Phase 2) - Independent error handling, but benefits from US1 being partially complete

### Within Each User Story

For User Story 1:
- Init container code before controller integration
- run.yaml merging before controller integration
- Both init container + run.yaml merging can proceed in parallel
- Status tracking can start early (parallel with other work)
- Controller integration depends on init container + run.yaml being done
- Testing can start as soon as implementation is done

### Parallel Opportunities

**Phase 1 (Setup)**: T001-T006 can all run in parallel (different files)

**Phase 2 (Foundational)**: T010-T011 can run in parallel (test files)

**Phase 3 (User Story 1)**:
- T012-T013 can run in parallel (init container helpers)
- T018-T019 can run in parallel (run.yaml structs)
- T031-T032 can run in parallel (status tracking helpers)
- T037-T042 all test files can run in parallel
- T046-T048 all docs can run in parallel

**Phase 4 (User Story 2)**:
- T052-T053 tests can run in parallel
- T056-T057 samples and docs can run in parallel

**Phase 5 (User Story 3)**:
- T058-T061 error formatting can run in parallel
- T063-T068 all integration tests can run in parallel
- T070-T072 all docs can run in parallel

**Phase 6 (Edge Cases)**:
- T073-T075 edge cases can run in parallel
- T079-T080 preflight tests can run in parallel

**Phase 7 (Polish)**:
- T082-T085 examples can run in parallel
- T086-T089 docs can run in parallel
- T090-T093 tests can run in parallel
- T096-T099 quality checks can run in parallel

---

## Parallel Example: User Story 1 Core Implementation

```bash
# Launch init container generation tasks in parallel:
Task: "Create GenerateInitContainers() function in pkg/deploy/initcontainer.go"
Task: "Create collectAllProviders() helper function in pkg/deploy/initcontainer.go"

# Launch run.yaml merging tasks in parallel:
Task: "Create RunYamlConfig struct in pkg/deploy/runyaml.go"
Task: "Create ProviderConfigEntry struct in pkg/deploy/runyaml.go"

# Launch status tracking tasks in parallel:
Task: "Create updateExternalProviderStatus() function in controllers/external_providers.go"
Task: "Implement getCurrentPod() helper in controllers/external_providers.go"

# Launch all unit tests in parallel after implementation:
Task: "Unit test init container generation in tests/unit/initcontainer_test.go"
Task: "Unit test init container ordering in tests/unit/initcontainer_test.go"
Task: "Unit test volume mount generation in tests/unit/initcontainer_test.go"
Task: "Unit test run.yaml merge scenarios in tests/unit/merge_test.go"
Task: "Unit test duplicate provider ID detection in tests/unit/merge_test.go"
Task: "Unit test API placement validation in tests/unit/merge_test.go"
```

---

## Implementation Strategy

### MVP First (User Story 1 Only)

1. Complete Phase 1: Setup (T001-T006)
2. Complete Phase 2: Foundational (T007-T011) - CRITICAL
3. Complete Phase 3: User Story 1 (T012-T048)
4. **STOP and VALIDATE**:
   - Deploy LLSD with external provider
   - Verify provider appears in /v1/providers
   - Test provider functionality
   - Check error messages for common failures
5. Demo/Deploy MVP if validation passes

### Incremental Delivery

1. **Foundation** (Phases 1-2) → Basic structure ready
2. **MVP** (Phase 3) → User Story 1 complete → Deploy custom providers ✅
3. **Enhancement** (Phase 4) → User Story 2 complete → Provider overrides work ✅
4. **Polish** (Phase 5) → User Story 3 complete → Excellent error messages ✅
5. **Hardening** (Phase 6) → Edge cases handled ✅
6. **Production Ready** (Phase 7) → Docs, examples, full test coverage ✅

### Parallel Team Strategy

With multiple developers:

1. **Together**: Complete Setup + Foundational (Phases 1-2)
2. **After Foundational complete**:
   - **Developer A**: User Story 1 - Init container generation (T012-T017)
   - **Developer B**: User Story 1 - run.yaml merging (T018-T024)
   - **Developer C**: User Story 1 - Status tracking (T031-T036)
3. **Integration**: Developer A completes controller integration (T025-T030) after B completes
4. **Testing**: All developers write tests in parallel (T037-T048)
5. **User Story 2 & 3**: Can proceed in parallel once US1 core is done

---

## Success Metrics

Per the plan, feature is successful when:

- ✅ Init container startup time < 30s per provider (T090)
- ✅ Zero false-positive conflict errors (T041, T066)
- ✅ 90%+ of external provider errors include actionable resolution (T069)
- ✅ All user scenarios have passing E2E tests (T043-T045, T054-T055, T063-T068)
- ✅ Documentation covers provider creation, API usage, troubleshooting (T046-T048, T056-T057, T070-T089)

---

## Notes

- [P] tasks = different files, no dependencies - can run in parallel
- [Story] label maps task to specific user story (US1, US2, US3) for traceability
- Each user story should be independently completable and testable
- Tests are integral to this feature (unit, integration, E2E all specified in plan)
- Commit after each task or logical group
- Stop at any checkpoint to validate story independently
- Many tasks can be parallelized within a phase - leverage concurrent development
- Provider validation (preflight) is a separate dependency - Phase 6 assumes it exists
