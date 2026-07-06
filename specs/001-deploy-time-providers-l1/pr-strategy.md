# PR Strategy: External Providers Feature

**Purpose**: Break down the 110-task implementation into reviewable PRs that maintain code quality and reviewer sanity.

**Created**: 2025-11-13
**Last Updated**: 2025-11-14

**Architecture Notes**:
- Two-phase init containers: Install → Merge (not three phases)
- extra-providers.yaml schema provides forward compatibility for Phase 2 migration
- Merge tool binary included in operator image (cmd/merge-run-yaml)
- Init containers ordered by CRD order (not alphabetical)

---

## Strategy Overview

### Goals
- ✅ **Incremental**: Each PR delivers working, testable functionality
- ✅ **Reviewable**: PRs stay under 500 lines of diff where possible
- ✅ **Independent**: PRs can be reviewed in parallel where dependencies allow
- ✅ **Safe**: Each PR includes tests and doesn't break existing functionality

### Anti-Goals
- ❌ **No monster PRs**: Avoid 2000+ line diffs that take days to review
- ❌ **No incomplete features**: Each PR should be demonstrably correct
- ❌ **No breaking changes**: Maintain backward compatibility throughout

---

## PR Breakdown (8 PRs)

### PR #1: Foundation - CRD Schema & Types
**Size**: ~150 lines
**Review Time**: 30 minutes
**Merge Strategy**: Merge immediately after approval

**Tasks**: T001-T004, T007-T008 (8 tasks)
- T001: Add ExternalProvidersSpec struct to CRD
- T002: Add ExternalProviderRef struct with validation tags
- T003: Add ExternalProviderStatus struct to status
- T004: Generate CRD manifests with `make manifests`
- T007: Create ProviderMetadata struct in pkg/provider/metadata.go
- T008: Implement LoadProviderMetadata() function

**Files**:
```
api/v1alpha1/llamastackdistribution_types.go  (+80 lines)
config/crd/bases/                              (+60 lines, generated)
pkg/provider/metadata.go                       (+120 lines, new)
```

**Tests**:
- CRD validation (duplicate provider IDs, invalid names)
- Metadata parsing (valid YAML)
- Metadata parsing (invalid YAML, missing fields)

**Acceptance Criteria**:
- [ ] CRD compiles without errors
- [ ] `make manifests` succeeds
- [ ] Validation tags work (kubectl apply rejects invalid specs)
- [ ] All unit tests pass

**Review Focus**:
- API naming conventions (Kubernetes API style)
- Validation tags completeness
- Go struct field tags (json, yaml, kubebuilder)

**Dependencies**: None - can start immediately

**Branch**: `001-external-providers-pr1-foundation`

---

### PR #2: Provider Validation & Metadata
**Size**: ~200 lines
**Review Time**: 45 minutes
**Merge Strategy**: Merge after PR #1

**Tasks**: T009-T011 (3 tasks)
- T009: Implement ValidateMetadata() function
- T010: Create metadata parsing unit tests
- T011: Create validation unit tests

**Files**:
```
pkg/provider/validation.go      (+150 lines, new)
tests/unit/metadata_test.go     (+200 lines, new)
tests/unit/validation_test.go   (+180 lines, new)
```

**Tests**:
- All validation rules (apiVersion, kind, required fields)
- Invalid API types
- ProviderType pattern matching
- WheelPath validation

**Acceptance Criteria**:
- [ ] All validation error paths tested
- [ ] ValidateMetadata() rejects all invalid inputs
- [ ] ValidateMetadata() accepts all valid inputs
- [ ] Test coverage > 90%

**Review Focus**:
- Validation logic correctness
- Error messages are actionable
- Test coverage completeness

**Dependencies**: PR #1 (types)

**Branch**: `001-external-providers-pr2-validation`

---

### PR #3: Init Container Generation Logic
**Size**: ~400 lines
**Review Time**: 1 hour
**Merge Strategy**: Merge after PR #2

**Tasks**: T012-T017, T017a, T037-T039 (10 tasks)
- T012-T016: Install init container generation functions
- T017: UpdatePythonPath() implementation
- T017a: Merge config init container generation
- T037-T039: Unit tests for init container generation

**Files**:
```
pkg/deploy/initcontainer.go       (+350 lines, new)
tests/unit/initcontainer_test.go  (+250 lines, new)
```

**Tests**:
- Init container generation (single provider)
- Init container generation (multiple providers)
- CRD order verification (not alphabetical)
- Volume mount generation
- Script content validation
- PYTHONPATH logic
- Merge config init container generation

**Acceptance Criteria**:
- [ ] Init containers generated in deterministic order
- [ ] Volume mounts correct
- [ ] Script includes error handling
- [ ] All unit tests pass
- [ ] Test coverage > 85%

**Review Focus**:
- Shell script correctness (no injection vulnerabilities)
- Deterministic ordering logic
- Volume mount paths
- Error handling in scripts

**Dependencies**: PR #2 (metadata types)

**Branch**: `001-external-providers-pr3-init-containers`

---

### PR #4: run.yaml Merging Logic
**Size**: ~650 lines
**Review Time**: 2 hours
**Merge Strategy**: Merge after PR #2 (independent of PR #3)

**Tasks**: T018-T024, T024a-h, T040-T042 (18 tasks)
- T018-T024: run.yaml merging implementation
- T024a-c: extra-providers.yaml generation from provider metadata
- T024d-h: Merge tool binary implementation
- T040-T042: Unit tests for merging

**Files**:
```
pkg/deploy/runyaml.go              (+400 lines, new)
pkg/provider/extra_providers.go    (+150 lines, new)
cmd/merge-run-yaml/main.go         (+200 lines, new)
tests/unit/merge_test.go           (+350 lines, new)
Dockerfile                         (+5 lines, merge tool binary)
```

**Description**:
This PR implements the run.yaml merging logic and introduces the extra-providers.yaml schema for forward compatibility. The merge tool binary is included in the operator image to support Phase 2 migration where external tooling can read extra-providers.yaml.

**Tests**:
- Base + external merge
- ConfigMap + external merge (external wins)
- Duplicate provider ID detection
- API placement validation
- Multiple providers same API
- Multiple providers different APIs
- extra-providers.yaml generation and validation
- Merge tool binary functionality

**Acceptance Criteria**:
- [ ] Merge precedence correct (external > ConfigMap > base)
- [ ] Duplicate IDs rejected with clear error
- [ ] API placement validated
- [ ] All merge scenarios tested
- [ ] extra-providers.yaml generated correctly
- [ ] Merge tool binary builds and runs successfully
- [ ] Test coverage > 90%

**Review Focus**:
- Merge precedence logic
- Error message clarity
- YAML parsing/generation correctness
- extra-providers.yaml schema forward compatibility
- Merge tool binary implementation
- Edge cases handled

**Dependencies**: PR #2 (metadata types)

**Branch**: `001-external-providers-pr4-runyaml-merge`

---

### PR #5: Controller Integration (MVP)
**Size**: ~350 lines
**Review Time**: 1.5 hours
**Merge Strategy**: Merge after PR #3 AND PR #4

**Tasks**: T025-T030, T043 (7 tasks)
- T025-T030: Controller integration
- T043: Basic integration test

**Files Modified**:
```
controllers/llamastackdistribution_controller.go  (+50 lines)
controllers/resource_helper.go                    (+200 lines)
```

**Files Added**:
```
tests/integration/external_providers_test.go      (+150 lines, new)
```

**Changes**:
1. `buildManifestContext()`: Add init container generation
2. `configurePodStorage()`: Update signature, add volume config
3. `configureContainerEnvironment()`: Add PYTHONPATH
4. `configureContainerMounts()`: Add provider mounts
5. New function: `configureExternalProviders()`

**Tests**:
- Integration test: single external provider installation
- Deployment manifests generated correctly
- Init containers present in deployment

**Acceptance Criteria**:
- [ ] Deployment created with init containers
- [ ] Volumes configured correctly
- [ ] PYTHONPATH set in main container
- [ ] Integration test passes
- [ ] No regressions in existing tests

**Review Focus**:
- Integration points clean
- No breaking changes to existing logic
- Error handling complete
- Volume mount paths correct

**Dependencies**: PR #3 AND PR #4

**Branch**: `001-external-providers-pr5-controller-mvp`

**🎯 MILESTONE: MVP Complete - Users can deploy with external providers**

---

### PR #6: Status Tracking & Conditions
**Size**: ~300 lines
**Review Time**: 1 hour
**Merge Strategy**: Merge after PR #5

**Tasks**: T031-T036, T044 (7 tasks)
- T031-T036: Status tracking implementation
- T044: Integration test for status

**Files Modified**:
```
controllers/llamastackdistribution_controller.go  (+20 lines)
controllers/status.go                             (+40 lines)
```

**Files Added**:
```
controllers/external_providers.go                 (+280 lines, new)
```

**Tests**:
- Unit test: init container status mapping
- Integration test: LLSD status reflects init container progress
- Status transitions (Pending → Installing → Ready/Failed)

**Acceptance Criteria**:
- [ ] ExternalProviderStatus updated correctly
- [ ] Init container failures reflected in status
- [ ] Conditions set appropriately
- [ ] Status messages are actionable
- [ ] Integration test passes

**Review Focus**:
- Status mapping correctness
- Error message quality
- Condition logic
- Status update timing

**Dependencies**: PR #5

**Branch**: `001-external-providers-pr6-status`

---

### PR #7: Documentation & Examples
**Size**: ~500 lines (mostly docs)
**Review Time**: 1 hour
**Merge Strategy**: Merge after PR #6

**Tasks**: T046-T048, T048a-b, T082-T089 (13 tasks)
- T046-T048: Basic examples and docs
- T048a: Document extra-providers.yaml schema
- T048b: Document forward compatibility plan (Phase 2 migration)
- T082-T089: Comprehensive documentation

**Files Added**:
```
config/samples/llamastackdistribution_external_providers.yaml  (+50 lines, new)
docs/external-providers.md                                     (+350 lines, new - includes extra-providers.yaml)
docs/examples/provider-image/Containerfile                     (+30 lines, new)
docs/examples/provider-image/lls-provider-spec.yaml            (+20 lines, new)
docs/provider-development.md                                   (+250 lines, new)
docs/troubleshooting.md                                        (+200 lines, new)
docs/forward-compatibility.md                                  (+100 lines, new - Phase 2 migration)
```

**Files Modified**:
```
README.md  (+30 lines - external providers overview)
```

**Acceptance Criteria**:
- [ ] Sample LLSD YAML works end-to-end
- [ ] Provider image creation guide complete
- [ ] Troubleshooting covers all error scenarios
- [ ] extra-providers.yaml schema documented
- [ ] Forward compatibility plan documented
- [ ] Examples tested and validated

**Review Focus**:
- Documentation clarity
- Examples correctness
- Troubleshooting completeness
- extra-providers.yaml schema documentation
- Forward compatibility approach

**Dependencies**: PR #6

**Branch**: `001-external-providers-pr7-docs`

---

### PR #8: Enhanced Error Handling & Edge Cases
**Size**: ~350 lines
**Review Time**: 1 hour
**Merge Strategy**: Merge after PR #7

**Tasks**: T049-T072 (24 tasks - User Story 2 & 3)
- T049-T057: Provider override logic and tests
- T058-T072: Error message formatting and edge case testing

**Files Modified**:
```
pkg/deploy/runyaml.go                            (+80 lines - override logic)
pkg/deploy/initcontainer.go                      (+40 lines - error formatting)
controllers/external_providers.go                (+60 lines - enhanced error extraction)
```

**Files Added**:
```
tests/integration/override_test.go               (+150 lines, new)
tests/integration/error_scenarios_test.go        (+200 lines, new)
```

**Tests**:
- Provider override scenarios
- All error message formats
- Missing metadata file
- Invalid YAML in metadata
- Missing packages directory
- Duplicate provider IDs
- API placement mismatches

**Acceptance Criteria**:
- [ ] Provider override works with warnings logged
- [ ] All error scenarios tested
- [ ] Error messages match spec format
- [ ] 90%+ of errors include resolution steps
- [ ] All integration tests pass

**Review Focus**:
- Override logic correctness
- Error message quality
- Warning logging
- Edge case coverage

**Dependencies**: PR #7

**Branch**: `001-external-providers-pr8-error-handling`

**🎯 MILESTONE: Feature Complete - All user stories implemented**

---

## Optional Follow-up PRs (Phase 6 & 7)

### PR #9: Preflight Validation Integration (OPTIONAL)
**Size**: ~200 lines
**Tasks**: T077-T081 (5 tasks)

**Note**: Only if lls-preflight-spec.md is implemented

**Dependencies**: Separate preflight feature

---

### PR #10: Performance & Polish (OPTIONAL)
**Size**: ~300 lines
**Tasks**: T073-T076, T090-T099 (14 tasks)

**Focus**:
- Performance testing
- Code quality improvements
- Final edge cases
- Security review

**Dependencies**: PR #8

---

## PR Sequencing Diagram

```
PR #1 (Foundation)
   ├─→ PR #2 (Validation)
   │      ├─→ PR #3 (Init Containers)
   │      │      └─→ PR #5 (Controller Integration) ──→ PR #6 (Status) ──→ PR #7 (Docs) ──→ PR #8 (Errors)
   │      └─→ PR #4 (run.yaml Merge) ──┘
   │
   └─→ (PR #3 and PR #4 can be developed in parallel after PR #2)
```

**Critical Path**: PR1 → PR2 → PR3 → PR5 → PR6 → PR7 → PR8

**Parallel Work**:
- After PR #2: PR #3 and PR #4 can be developed simultaneously
- While PR #5 is in review: Start PR #6
- While PR #6 is in review: Work on PR #7

---

## Review Guidelines

### For Reviewers

**PR #1-2 (Foundation)**:
- Focus: API design, validation completeness
- Time: 30-45 min each
- Checklist:
  - [ ] Kubernetes API conventions followed
  - [ ] Validation rules comprehensive
  - [ ] Error messages actionable

**PR #3-4 (Core Logic)**:
- Focus: Business logic correctness, test coverage
- Time: 1-1.5 hours each
- Checklist:
  - [ ] Logic matches spec requirements
  - [ ] Edge cases handled
  - [ ] Test coverage > 85%
  - [ ] No security vulnerabilities (shell injection, path traversal)

**PR #5-6 (Integration)**:
- Focus: Integration correctness, no regressions
- Time: 1-1.5 hours each
- Checklist:
  - [ ] Existing functionality unaffected
  - [ ] Integration points clean
  - [ ] Status updates accurate
  - [ ] All existing tests still pass

**PR #7 (Documentation)**:
- Focus: Clarity, completeness, examples work
- Time: 45 min
- Checklist:
  - [ ] Examples tested and work
  - [ ] Troubleshooting covers common errors
  - [ ] Writing is clear and accurate

**PR #8 (Polish)**:
- Focus: Error handling, edge cases
- Time: 1 hour
- Checklist:
  - [ ] All error paths tested
  - [ ] Error messages match spec
  - [ ] Edge cases handled gracefully

---

## Merge Cadence

**Target**: ~1 PR per day (development) + 1-2 days review time

### Week 1: Foundation
- **Mon-Tue**: Develop PR #1, submit for review
- **Wed**: Develop PR #2 while PR #1 is reviewed
- **Thu**: Merge PR #1, submit PR #2 for review
- **Fri**: Start PR #3 and PR #4 (parallel)

### Week 2: Core Implementation
- **Mon**: Complete PR #3 and PR #4, submit both for review
- **Tue-Wed**: Develop PR #5 while PR #3/#4 reviewed
- **Thu**: Merge PR #3 and PR #4, submit PR #5 for review
- **Fri**: Start PR #6

### Week 3: Integration & Polish
- **Mon**: Merge PR #5, submit PR #6 for review
- **Tue**: Develop PR #7 (docs)
- **Wed**: Merge PR #6, submit PR #7 for review
- **Thu-Fri**: Develop PR #8

### Week 4: Completion
- **Mon**: Merge PR #7, submit PR #8 for review
- **Tue-Wed**: Address PR #8 feedback
- **Thu**: Merge PR #8
- **Fri**: Feature complete, end-to-end validation

**Total Timeline**: ~4 weeks from start to feature complete

---

## Risk Mitigation

### Large PR Risk
**Mitigation**: Strict line limits (400-500 max), split if needed

### Review Bottleneck Risk
**Mitigation**:
- Flag PRs as high-priority in sequence
- Assign dedicated reviewers early
- Use GitHub draft PRs for early feedback

### Merge Conflict Risk
**Mitigation**:
- Rebase frequently on main
- Keep PRs small and merge quickly
- Communicate parallel work to team

### Test Coverage Risk
**Mitigation**:
- Minimum 85% coverage required for merge
- Integration tests in every non-docs PR
- E2E test in PR #5 (MVP)

### Breaking Change Risk
**Mitigation**:
- No changes to existing CRD fields
- Only additive changes
- Feature flag if needed (disabled by default)

---

## Success Metrics Per PR

| PR | Metric | Target |
|----|--------|--------|
| #1 | Lines changed | < 200 |
| #1 | Review time | < 1 hour |
| #2 | Test coverage | > 90% |
| #2 | Review time | < 1 hour |
| #3 | Test coverage | > 85% |
| #3 | Review time | < 1.5 hours |
| #4 | Test coverage | > 90% |
| #4 | Review time | < 2 hours |
| #5 | Existing tests pass | 100% |
| #5 | Review time | < 2 hours |
| #6 | Status accuracy | 100% |
| #6 | Review time | < 1.5 hours |
| #7 | Examples work | 100% |
| #7 | Review time | < 1 hour |
| #8 | Error coverage | > 90% |
| #8 | Review time | < 1.5 hours |

**Overall Target**: < 13 hours total review time across all PRs

---

## Alternative Strategy: Feature Flag

If reviewer bandwidth is extremely limited, consider:

**Option B: Single Large PR with Feature Flag**
- Merge all code in one PR (~1500 lines)
- Disabled by default via feature flag
- Enable incrementally via flag
- Requires very experienced reviewer with 4+ hours availability

**Pros**: Fewer context switches, one approval process
**Cons**: Large diff, longer review time, harder to spot issues

**Recommendation**: Stick with incremental approach unless team specifically requests consolidation

---

## Post-Merge Validation

After each PR:
- [ ] Run full test suite
- [ ] Verify CRD generation
- [ ] Check for lint errors
- [ ] Validate API compatibility
- [ ] Update changelog/release notes

After PR #5 (MVP):
- [ ] Deploy sample LLSD with external provider
- [ ] Verify provider appears in API
- [ ] Test basic provider functionality
- [ ] Document any issues found

After PR #8 (Feature Complete):
- [ ] Full E2E test suite
- [ ] Performance benchmarking
- [ ] Security review
- [ ] Documentation review
- [ ] Prepare release announcement

---

## Communication Plan

### To Team
- **Before starting**: Share PR strategy, get buy-in
- **Each PR**: Link to spec, integration points doc
- **After MVP**: Demo working feature
- **After completion**: Feature walkthrough, Q&A

### To Reviewers
- **PR Description Template**:
  ```markdown
  ## PR #X: [Title]

  **Part of**: External Providers Feature (#001)
  **Spec**: specs/001-deploy-time-providers-l1/spec.md
  **Integration Points**: specs/001-deploy-time-providers-l1/integration-points.md
  **PR Strategy**: specs/001-deploy-time-providers-l1/pr-strategy.md

  ### Summary
  [What this PR does]

  ### Tasks Completed
  - [ ] Task ID: Description

  ### Changes
  - File 1: [description]
  - File 2: [description]

  ### Testing
  - Unit tests: [coverage %]
  - Integration tests: [scenarios]

  ### Review Focus
  - [Area 1]
  - [Area 2]

  ### Depends On
  - PR #X (merged/in review)

  ### Screenshots/Examples
  [If applicable]
  ```

### In Code
- Link to spec in comments
- Reference task IDs in commits
- Clear commit messages

---

## Rollback Plan

If a merged PR causes issues:

**Option 1: Quick Fix**
- If fix < 50 lines → Hotfix PR
- Reference original PR
- Fast-track review

**Option 2: Revert**
- Revert the problematic PR
- Fix in separate PR
- Re-merge when ready

**Option 3: Feature Flag Disable**
- If implemented with feature flag
- Disable via config
- Fix offline, re-enable

**Prevention**:
- Comprehensive tests in each PR
- Integration tests catch regressions
- E2E test in PR #5 validates end-to-end

---

## Summary

- ✅ **8 PRs** covering **110 tasks** instead of 1 monster PR
- ✅ **~4 weeks** timeline with parallel work
- ✅ **< 650 lines** per PR on average
- ✅ **2 checkpoints**: MVP (PR #5), Feature Complete (PR #8)
- ✅ **Independent review**: PRs can be reviewed in parallel where possible

**Architecture Highlights**:
- Two-phase init containers (Install → Merge) for simplicity
- extra-providers.yaml schema enables forward compatibility
- Merge tool binary in operator image supports Phase 2 migration
- CRD-ordered init containers for deterministic behavior

**Key Success Factors**:
1. Strict adherence to PR size limits
2. Comprehensive tests in each PR
3. Clear communication and documentation
4. Regular rebasing to avoid conflicts
5. Dedicated reviewer assignment

This strategy balances velocity with quality, ensuring reviewers can thoroughly review each piece while maintaining forward momentum.
