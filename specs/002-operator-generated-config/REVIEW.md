# Review Summary: Operator-Generated Server Configuration (v1alpha2)

**Feature**: 002-operator-generated-config
**Branch**: 002-reimpl
**Review Date**: 2026-03-10

## How to Review This Spec (30-Minute Recipe)

This is a spec-only PR. No implementation code. The goal is to validate the design before writing code, so we catch issues like the polymorphism problem from PR #253 early.

### Step 1: Understand the Goal (3 min)

Read the **Purpose** and **Configuration Tiers** table in `spec.md` (lines 9-11, 581-583). The core idea: users write 10-20 lines of YAML instead of a 200-line ConfigMap. Three tiers: simple inline, advanced settings, full override.

### Step 2: Review the CRD Example (5 min)

Read the **Complete v1alpha2 Spec Structure** YAML in `spec.md` (lines 379-473). This is what users will write. Ask yourself:

- Does this YAML feel natural for a Kubernetes user?
- Are the field names intuitive?
- Would you know what to write without reading docs?

Key design decision to validate: **providers are always lists** (e.g., `inference: [{provider: vllm}]` not `inference: {provider: vllm}`). We chose this to enable kubebuilder validation and CEL rules, at the cost of slightly more verbose YAML for single providers. If you disagree, this is the most impactful thing to flag.

### Step 3: Check the Three Critical Design Decisions (10 min)

These are the decisions that will be hardest to change after implementation starts:

**Decision 1: Typed structs instead of polymorphic JSON** (spec.md FR-004, FR-007, FR-011, research.md R1)

The original PR #253 used `apiextensionsv1.JSON` for three polymorphic fields. All three are replaced with typed alternatives:

- **Providers** (`ProviderConfigOrList`): Now `[]ProviderConfig`. Users always write list syntax (FR-004).
- **Models** (`[]apiextensionsv1.JSON`): Now `[]ModelConfig` with only `name` required (FR-007). Users write `- name: "llama3.2-8b"` instead of `- "llama3.2-8b"`.
- **Expose** (`*apiextensionsv1.JSON`): Now `ExposeConfig` struct with `enabled` bool + `hostname` string (FR-011). Users write `expose: {enabled: true}` instead of `expose: true`.

This eliminates: no kubebuilder validation, impossible CEL rules, ~500 lines of parsing code, false-positive secret detection bugs. Verify the tradeoffs are acceptable.

**Decision 2: Explicit `secretRefs` field instead of heuristic detection** (spec.md FR-005, contracts/config-generation.yaml)

The original PR #253 scanned the `settings` map for any `{name, key}` structure and treated it as a Secret reference. This caused false positives. The new design adds an explicit `secretRefs: map[string]SecretKeyRef` field on ProviderConfig. The `settings` map is passed through without any secret resolution. Check `contracts/config-generation.yaml` "Secret Resolution" section for examples.

**Decision 3: Provider merge = full API-type replacement** (contracts/config-generation.yaml merge_rules)

When a user specifies `providers.inference`, ALL base config inference providers are replaced. Base providers with unmatched IDs are dropped. The contract has before/after examples. Verify this matches your expectation. The alternative (merge-by-ID, preserving unmatched base providers) was the original PR #253 behavior but contradicted the contract.

### Step 4: Spot-Check CEL Validation Rules (5 min)

Read `contracts/crd-schema.yaml` CEL rules section (bottom of file). There are 11 rules. Focus on:

- **Rule 6** (provider ID required when list > 1): Can CEL express `self.providers.inference.size() <= 1 || self.providers.inference.all(p, has(p.id))`? This is the rule that was impossible with JSON types.
- **Rule 8** (disabled + provider conflict): Should this be an error or a warning? We chose error. If you prefer warning, flag it.
- **Rules 9-11** (conditional fields): TLS needs secretName when enabled, Redis needs endpoint, Postgres needs connectionString. These were missing in the original spec.

### Step 5: Scan Edge Cases (4 min)

Read the **Edge Cases** section in `spec.md` (lines 130-176). There are 13 edge cases. Focus on the ones that affect data integrity:

- "Secret references via settings vs secretRefs": confirms settings map is never inspected for secrets
- "Disabled APIs conflict with providers": now an error, not a warning
- "Config generation failure on update": preserves running Deployment, critical for production

### Step 6: Verify Conversion Strategy (3 min)

Read the **Field Mapping: v1alpha1 to v1alpha2** table in `spec.md` (lines 477-499). Check that existing v1alpha1 fields all have a v1alpha2 home. New v1alpha2-only fields (providers, resources, storage, disabled) are stored as JSON annotation for round-trip fidelity. This is the standard kubebuilder pattern.

### What NOT to Review

- `plan.md`: Implementation details, will change during coding
- `tasks.md`: Task breakdown, auto-generated, will evolve
- `data-model.md`: Derived from spec, no independent decisions
- `quickstart.md`: Examples only, validated against CRD schema

## Key Changes from PR #253

This spec addresses all critical issues raised in the PR #253 review:

| PR #253 Issue | Resolution | Spec Location |
|--------------|------------|---------------|
| Polymorphic JSON types lose kubebuilder validation | Replaced with typed `[]ProviderConfig` slices | FR-004, research.md R1 |
| Polymorphic models (`[]apiextensionsv1.JSON`) | Replaced with typed `[]ModelConfig` (only `name` required) | FR-007, data-model.md ModelConfig |
| Polymorphic expose (`*apiextensionsv1.JSON`) | Replaced with typed `ExposeConfig` struct (`enabled` bool + `hostname` string) | FR-011, data-model.md ExposeConfig |
| CEL rules impossible on `apiextensionsv1.JSON` | CEL now works because providers are typed | FR-071, FR-072 |
| `extractDirectSecretRef` false positives | Explicit `secretRefs` field, no heuristic matching | FR-005 |
| `sortedMapKeys` doesn't sort | Determinism addressed in NFR-001, merge.go | NFR-001 |
| Missing CEL for TLS/storage conditionals | Added FR-079, FR-079a-c | Validation section |
| Disabled + provider should be error not warning | Changed to validation error | OQ-002, edge case |
| Status conditions defined but unwired | Tasks T036, T055, T090 wire all 4 conditions | tasks.md |
| Missing test coverage for FR-097, FR-096, FR-100 | Dedicated tasks T053-T058 | tasks.md Phase 7 |
| Contract says replace but code does merge-by-ID | Contract updated with explicit examples | config-generation.yaml |

## Coverage Matrix

| Spec Requirement | Plan Section | Task(s) | Status |
|-----------------|-------------|---------|--------|
| FR-001 v1alpha2 API version | 1.1 CRD Schema | T001-T002 | Covered |
| FR-002 Distribution name/image | 1.1 CRD Schema | T003 | Covered |
| FR-003 Provider types | 1.1 CRD Schema | T004 | Covered |
| FR-004 Typed provider slices | 1.1 CRD Schema | T004 | Covered |
| FR-005 ProviderConfig fields + secretRefs | 1.1 CRD Schema | T004-T005 | Covered |
| FR-006-007 Resources (ModelConfig) | 1.1 CRD Schema | T006 | Covered |
| FR-008 Storage subsections | 1.1 CRD Schema | T007 | Covered |
| FR-010-011 Networking + ExposeConfig | 1.1 CRD Schema | T008 | Covered |
| FR-012 WorkloadSpec | 1.1 CRD Schema | T009 | Covered |
| FR-013 OverrideConfig mutual exclusivity | 1.1 CEL rules | T010, T012 | Covered |
| FR-020 Distribution resolution | 1.4 Controller | T033 | Covered |
| FR-021 Config generation | 1.2 Pipeline | T023 | Covered |
| FR-022 SecretKeyRef resolution | 1.2 Pipeline | T020 | Covered |
| FR-023-025 ConfigMap creation + owner ref | 1.4 Controller | T031-T032 | Covered |
| FR-025a ConfigMap cleanup (retain 2) | 1.4 Controller | T032 | Covered |
| FR-027a1 ConfigResolver interface | 1.3 ConfigResolver | T014 | Covered |
| FR-027a-e Embedded configs | 1.3 ConfigResolver | T014-T016 | Covered |
| FR-030-035 Provider field mapping | 1.2 Pipeline | T019 | Covered |
| FR-040-044 Resource registration | 1.2 Pipeline | T021, T045-T048 | Covered |
| FR-050-053 Storage configuration | 1.2 Pipeline | T022, T049-T052 | Covered |
| FR-060-066 Networking | 1.4 Controller | T059-T063 | Covered |
| FR-070-072 CEL validation | 1.1 CEL rules | T012 | Covered |
| FR-073-078 Webhook validation | 1.5 Webhook | T074-T080 | Covered |
| FR-079-079d Conditional CEL + webhook | 1.1/1.5 | T012, T074 | Covered |
| FR-080-083 Conversion webhook | 1.6 Conversion | T068-T073 | Covered |
| FR-095-098 Runtime updates | 1.4 Controller | T053-T058 | Covered |
| FR-099 Status conditions | 1.4 Controller | T036, T055, T090 | Covered |
| FR-100 Atomic deployment update | 1.4 Controller | T035 | Covered |
| NFR-001 Deterministic output | 1.2 Pipeline | T030 | Covered |
| NFR-005 Immutable ConfigMaps | 1.4 Controller | T031 | Covered |
| NFR-007 Kubernetes Events | 1.4 Controller | T037 | Covered |

## Summary Statistics

- **Total tasks**: 91 across 12 phases
- **Tasks per user story**: US1: 11, US2: 3, US3: 4, US4: 4, US5: 5, US6: 4, US7: 6, US8: 6
- **Parallel execution streams**: 4 independent streams after foundational phase
- **MVP checkpoint**: Phase 3 (US1: minimal inference config)
- **Estimated implementation PRs**: 5 focused PRs
