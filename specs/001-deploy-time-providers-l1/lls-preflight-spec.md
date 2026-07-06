# Feature: LLama Stack Provider Preflight Validation

**Status**: Draft
**Created**: 2025-11-12
**Priority**: P1 (Dependency for Deploy-Time Providers)
**Depends on**: None
**Required by**: specs/001-deploy-time-providers-l1/spec.md

## Purpose

Provide a command-line tool for validating provider packages before the llama-stack server starts, enabling early detection of configuration errors, import failures, and metadata inconsistencies. This prevents runtime failures and provides clear feedback to operators during deployment.

## User Scenarios & Testing

### User Story 1 - Validate Provider Before Server Start (Priority: P1)

As a Kubernetes operator, I need to validate that installed provider packages are functional before starting the llama-stack server, so that deployment failures are caught early with clear error messages rather than failing silently at runtime.

**Why this priority**: Critical for deploy-time provider injection - prevents pod crash loops and provides fast feedback.

**Independent Test**: Run preflight command with valid and invalid provider configurations, verify exit codes and error messages.

**Acceptance Scenarios**:

1. **Given** all provider packages are correctly installed and importable, **When** preflight runs, **Then** it exits with code 0 and reports "All providers validated successfully"
2. **Given** a provider module cannot be imported, **When** preflight runs, **Then** it exits with code 1 and reports the specific import error with module name
3. **Given** a provider's get_provider_spec() returns invalid data, **When** preflight runs, **Then** it exits with code 1 and reports which provider and what validation failed

### Edge Cases

- What happens when provider has optional dependencies that fail import?
  - **Expected**: Optional dependencies in try/except blocks are allowed; main module import must succeed
- What happens when get_provider_spec() raises an exception?
  - **Expected**: Preflight catches exception, reports provider ID and error message, exits with code 1
- What happens when PYTHONPATH is not set correctly?
  - **Expected**: Import fails, preflight reports module not found error
- What happens when provider has native code compiled for wrong architecture?
  - **Expected**: Preflight detects architecture mismatch before import, fails with clear error identifying incompatible package and architectures

## Requirements

### Functional Requirements

#### Command Interface

- **FR-001**: Preflight validation MUST be invokable via command-line interface: `llama-stack preflight`
- **FR-002**: The command MUST accept a run.yaml configuration file path as input
- **FR-003**: The command MUST exit with code 0 on success, non-zero on failure
- **FR-004**: The command MUST validate ALL providers defined in run.yaml
- **FR-005**: The command MUST complete validation within 30 seconds for typical configurations (1-10 providers)

#### Import Validation

- **FR-006**: For each provider in run.yaml, preflight MUST attempt to import the provider module
- **FR-007**: Import failures MUST be reported with: provider ID, module name, import error message
- **FR-008**: Main provider module import MUST succeed; optional dependency import failures within the module are allowed (try/except blocks)

#### Architecture and Native Code Validation

- **FR-008a**: Preflight MUST detect if provider packages contain native extensions (compiled C/C++ code)
- **FR-008b**: Preflight MUST validate that native extensions are compatible with the runtime platform architecture
- **FR-008c**: Architecture mismatches MUST cause preflight to fail with error identifying the incompatible package and expected vs actual architecture
- **FR-008d**: Detection MUST happen before import attempt to fail fast (check wheel metadata or .so/.pyd file headers)

#### Provider Spec Validation

- **FR-009**: For each successfully imported provider, preflight MUST call `get_provider_spec()` function
- **FR-010**: The returned ProviderSpec MUST be validated for required fields: provider_type, api, config_schema
- **FR-011**: If get_provider_spec() raises an exception, preflight MUST fail with error details
- **FR-012**: If get_provider_spec() returns None or invalid data, preflight MUST fail with error details

#### Metadata Consistency (Warning Only)

- **FR-013**: Preflight MUST compare ALL fields that appear in both lls-provider-spec.yaml and get_provider_spec() output
- **FR-014**: Fields to compare include: provider_type, api, and any other overlapping fields in metadata and ProviderSpec
- **FR-015**: ALL metadata mismatches MUST log warnings but NOT fail validation
- **FR-016**: Warnings MUST clearly state which value is used (runtime ProviderSpec takes precedence)
- **FR-017**: Each mismatch warning MUST specify: provider ID, field name, metadata value, runtime value

#### Error Reporting

- **FR-018**: All errors MUST include: provider ID, module name, failure reason
- **FR-019**: Import errors MUST include the full Python traceback
- **FR-020**: Validation errors MUST specify which field failed validation and why
- **FR-021**: On failure, preflight MUST exit immediately after reporting first error (fail-fast)

### Non-Functional Requirements

- **NFR-001**: Preflight execution MUST NOT start the llama-stack server
- **NFR-002**: Preflight MUST NOT make network requests
- **NFR-003**: Preflight MUST NOT modify any files or state
- **NFR-004**: Error messages MUST be actionable (user can fix without llama-stack internals knowledge)

### Key Entities

- **PreflightResult**: Validation result containing status, validated providers, errors
- **ProviderValidationError**: Error details for single provider validation failure

## Behavioral Contracts

### Validation Process

For each provider in run.yaml:

1. **Architecture Validation Phase**:
   - Check provider packages for native extensions (.so, .pyd files)
   - Validate native extension architecture matches runtime platform
   - If architecture mismatch → Report error, exit with code 1
   - If no native code or compatible → Continue to next phase

2. **Import Phase**:
   - Attempt `import {module}`
   - If fails → Report error, exit with code 1
   - If succeeds (optional dependencies in try/except can fail) → Continue to next phase

3. **Spec Retrieval Phase**:
   - Call `get_provider_spec()`
   - If raises exception → Report error, exit with code 1
   - If returns None/invalid → Report error, exit with code 1
   - If succeeds → Continue to next phase

4. **Spec Validation Phase**:
   - Validate required fields: provider_type, api, config_schema
   - If any field missing/invalid → Report error, exit with code 1
   - If valid → Continue to next phase

5. **Metadata Consistency Phase** (Optional):
   - Load lls-provider-spec.yaml from metadata directory (if exists)
   - Compare ALL overlapping fields (provider_type, api, etc.)
   - If mismatch → Log WARNING for each mismatched field (don't fail)
   - Runtime values take precedence

### Exit Codes

- **0**: All providers validated successfully
- **1**: Validation failed (import error, spec error, validation error)
- **2**: Invalid command-line arguments (e.g., missing run.yaml path)

### Error Message Format

```
ERROR: Provider validation failed

Provider: {providerId}
Module: {moduleName}
Reason: {errorReason}

{traceback or details}

Resolution: {actionable fix suggestion}
```

**Import Error Example**:
```
ERROR: Provider validation failed

Provider: custom-vllm
Module: custom_vllm_provider
Reason: Module not found

Traceback:
  ModuleNotFoundError: No module named 'custom_vllm_provider'

Resolution: Ensure provider packages are installed and PYTHONPATH is set correctly.
```

**Spec Validation Error Example**:
```
ERROR: Provider spec validation failed

Provider: custom-vllm
Module: custom_vllm_provider
Reason: get_provider_spec() returned invalid data

Missing required field: provider_type

Resolution: Provider implementation must return ProviderSpec with provider_type field.
```

**Architecture Mismatch Error Example**:
```
ERROR: Provider validation failed

Provider: custom-vllm
Module: custom_vllm_provider
Reason: Native extension architecture mismatch

Package: torch-2.1.0-cp311-cp311-linux_x86_64.whl
Expected architecture: linux/arm64
Found architecture: linux/x86_64

Resolution: Rebuild provider package with native extensions compiled for linux/arm64, or use pure Python implementation.
```

**Metadata Mismatch Warning Example**:
```
WARNING: Provider metadata mismatch (non-blocking)

Provider: custom-vllm

Field: provider_type
  Metadata (lls-provider-spec.yaml): remote::custom-vllm
  Runtime (get_provider_spec()): remote::vllm-v2
  Using: remote::vllm-v2 (runtime value)

Field: api
  Metadata (lls-provider-spec.yaml): inference
  Runtime (get_provider_spec()): inference
  Match ✓

Note: Update lls-provider-spec.yaml to match runtime implementation for mismatched fields.
```

## Success Criteria

### Measurable Outcomes

- **SC-001**: Preflight command exists and is executable: `llama-stack preflight --run-yaml=<path>`
- **SC-002**: Preflight validates all providers in run.yaml and exits with code 0 on success
- **SC-003**: Architecture mismatches are detected before import and fail with clear error identifying package and architectures
- **SC-004**: Import failures result in exit code 1 with clear error message and traceback
- **SC-005**: Invalid ProviderSpec results in exit code 1 with field-specific error message
- **SC-006**: ALL overlapping fields between metadata and runtime are compared with warnings for mismatches
- **SC-007**: Metadata mismatches generate warnings but don't fail validation
- **SC-008**: Optional dependency import failures within providers are allowed (try/except blocks)
- **SC-009**: Preflight completes within 30 seconds for 10 providers

## Dependencies

### Internal
- llama-stack must expose `get_provider_spec()` function for all providers
- llama-stack must define ProviderSpec structure with required fields

### External
- Python 3.11+ runtime
- Access to installed provider packages (via PYTHONPATH)

## Constraints

- Preflight runs in same Python environment as llama-stack server
- All provider packages must already be installed (preflight doesn't install packages)
- Preflight validates configuration but doesn't test actual provider functionality (no API calls)

## Out of Scope

The following are explicitly NOT included:

- **Provider functionality testing** - Actually calling provider APIs or methods
- **Performance benchmarking** - Measuring provider latency or throughput
- **Dependency conflict detection** - Checking for package version conflicts (handled during installation)
- **Provider health monitoring** - Ongoing runtime health checks
- **Configuration schema validation** - Deep validation of provider config values
- **Caching validation results** - Storing preflight results for future runs
- **Parallel validation** - Running preflight checks concurrently

## Open Questions

None - this is a straightforward validation tool.

## Acceptance

Feature is complete when:

- [ ] `llama-stack preflight --run-yaml=<path>` command exists
- [ ] All functional requirements implemented and tested
- [ ] Architecture validation detects native code mismatches before import
- [ ] Import validation catches module not found errors
- [ ] Optional dependency imports within modules are allowed (try/except)
- [ ] ProviderSpec validation catches missing required fields
- [ ] ALL overlapping fields between metadata and runtime are compared
- [ ] Metadata mismatch warnings are logged correctly for each mismatched field
- [ ] Error messages match specified format with traceback and resolution
- [ ] Exit codes correct (0 = success, 1 = validation failed, 2 = invalid args)
- [ ] Unit tests cover all error paths (architecture, import, spec, metadata)
- [ ] Integration tests validate with real provider packages
- [ ] Documentation covers usage and error troubleshooting
