# Implementation Notes: Deploy-Time Modularity - Level 1

**Purpose**: This document captures technical insights, design decisions, and rationale from the brainstorming and specification process. It serves as context for implementation and future evolution.

**Created**: 2025-11-12
**Status**: Reference material for implementers

---

## Key Design Decisions

### 1. Provider Type Removed from CRD

**Decision**: Do NOT include `providerType` in the `ExternalProviderRef` CRD structure.

**Rationale**:
- **Provider type is intrinsic to the implementation**, not user configuration
- The provider image self-describes its type via `lls-provider-spec.yaml`
- Avoids duplication and potential mismatches between CRD and image
- Reduces user burden - they only specify configuration, not implementation details
- Aligns with module-based approach where type comes from `get_provider_spec()`

**Three Identifier Clarification**:
There are three distinct identifiers in the llama-stack provider system:

1. **API Type** (category): `inference`, `safety`, `eval` - which API surface the provider implements
2. **Provider Type** (implementation): `remote::vllm`, `inline::meta-reference` - which specific implementation
3. **Provider ID** (instance name): `vllm-inference`, `my-custom-vllm` - user-chosen unique identifier

The CRD structure (organizing by API type) + user-provided provider ID + image-declared provider type gives us all three.

**User Experience Impact**:
```yaml
# With providerType (rejected):
externalProviders:
  inference:
    - providerId: my-vllm
      providerType: remote::custom-vllm  # User must know this!
      image: quay.io/vendor/vllm:v1.0

# Without providerType (accepted):
externalProviders:
  inference:
    - providerId: my-vllm               # User chooses instance name
      image: quay.io/vendor/vllm:v1.0   # Image declares its type
```

**Validation Flow**:
1. Init container copies `lls-provider-spec.yaml` to shared volume
2. Operator reads metadata to get `providerType` and `module`
3. Preflight validates metadata matches `get_provider_spec()` output
4. Runtime value takes precedence if mismatch

---

### 2. Python Wheel Packaging Format

**Decision**: Use Python wheels (`.whl` files) as the standard provider package format.

**Rationale**:
- **Self-contained**: Wheels can bundle all dependencies or reference separate wheels
- **No network access needed**: `pip install --no-index` works with local wheels
- **Standard format**: Well-understood by Python ecosystem
- **Easy validation**: Can inspect wheel metadata without installation
- **Architecture support**: Wheels can include platform-specific compiled extensions

**Alternative Considered**: Plain source code with `requirements.txt`
- **Rejected because**: Would require pip to resolve dependencies (needs network access)
- **Rejected because**: Harder to validate before installation
- **Rejected because**: More error-prone (dependency resolution can fail)

**Provider Image Structure**:
```
/lls-provider/
  ├── lls-provider-spec.yaml
  └── packages/
      ├── llama_stack_provider_custom_vllm-1.0.0-py3-none-any.whl
      └── dependency1-2.3.4-py3-none-any.whl  (optional - see below)
```

---

### 3. Dependency Packaging: Separate vs. Bundled

**Decision**: Support BOTH approaches, document trade-offs

**Approach A: Separate Dependency Wheels**

Pros:
- Easier to audit dependencies (separate files)
- Shared dependencies across providers possible (same wheel used by multiple providers)
- Smaller individual wheels
- Clear visibility into dependency tree

Cons:
- More files to manage and track
- Potential for version conflicts between providers
- More complex packaging process

**Use Case**: Multiple providers with overlapping dependencies

**Approach B: Bundled Dependencies**

Pros:
- Single file deployment (simpler distribution)
- No external dependency conflicts
- Simpler packaging workflow

Cons:
- Larger wheel size
- Duplicate dependencies if multiple providers used
- Hidden dependency tree

**Use Case**: Standalone provider with unique dependencies, isolation critical

**Recommendation**: Default to **separate wheels** for transparency, but support bundled for special cases.

**Conflict Detection**:
With separate wheels, pip automatically detects conflicts during install:
```bash
pip install provider-a.whl provider-b.whl
# ERROR: Cannot install because:
#   provider-a depends on requests==2.28.0
#   provider-b depends on requests==2.31.0
```

This is actually GOOD - fail early, fail loudly.

---

### 4. Metadata Validation: Warning vs. Error

**Decision**: Metadata mismatch between `lls-provider-spec.yaml` and `get_provider_spec()` is a WARNING, not an error.

**Rationale**:
- **Runtime is source of truth**: The actual provider implementation knows what it is
- **Non-blocking**: Allows providers to work even if metadata slightly outdated
- **Evolutionary**: Providers can evolve without breaking due to metadata lag
- **User-friendly**: Doesn't fail deployment over non-critical mismatch

**Validation Flow**:
1. Read `lls-provider-spec.yaml` during init container
2. Install provider and import module
3. Call `get_provider_spec()` to get runtime values
4. Compare: if mismatch → log WARNING, use runtime values
5. Continue deployment

**Warning Message Format**:
```
WARNING: Provider metadata mismatch for 'custom-vllm'
  lls-provider-spec.yaml declares: providerType=remote::custom-vllm, api=inference
  get_provider_spec() returns:     providerType=remote::custom-vllm-v2, api=inference
  Using runtime values from get_provider_spec()

  Recommendation: Update lls-provider-spec.yaml to match get_provider_spec() or
  use lls-provider-spec.yaml as single source of truth in get_provider_spec() implementation.
```

**Best Practice**: Provider implementations should load `lls-provider-spec.yaml` directly to ensure single source of truth.

---

### 5. Provider Installation: Init Containers vs. Alternatives

**Decision**: Use Kubernetes init containers for provider installation (Level 1).

**Alternatives Considered**:

| Approach | Pros | Cons | Decision |
|----------|------|------|----------|
| **Init Containers** | Simple, native K8s pattern, no runtime overhead, shares pod context | Sequential startup, limited isolation, no independent scaling | **CHOSEN for Level 1** |
| **Sidecar Containers** | Better isolation, independent lifecycle, separate resource limits | Network overhead, more complex, higher resource usage | Defer to Level 2a |
| **External Pods** | Maximum isolation, independent scaling, separate deployments | Complex networking, high overhead, operational complexity | Defer to Level 2b |
| **Pre-built Images** | Fastest startup, smaller attack surface | Loses deploy-time flexibility, rebuild required | **REJECTED** - contradicts goal |
| **DaemonSet** | Install once per node, reuse across pods | Node-level coupling, complex cleanup, overkill | **REJECTED** - too complex |

**Init Container Benefits for Level 1**:
- ✅ Simplest implementation
- ✅ Native Kubernetes pattern
- ✅ No networking complexity
- ✅ Shared filesystem (emptyDir volume)
- ✅ No runtime overhead after startup
- ✅ Clear success/failure semantics

**Init Container Limitations** (addressed in future levels):
- ⚠️ Sequential startup (can be slow with many providers)
- ⚠️ Limited isolation (shared Python environment)
- ⚠️ No independent scaling
- ⚠️ No hot-reloading

---

### 6. Configuration Merge Order and Precedence

**Decision**: External providers override user ConfigMap and distribution defaults.

**Merge Order** (later overwrites earlier):
1. Base `run.yaml` from distribution image
2. User ConfigMap `run.yaml` (completely replaces base if specified)
3. External providers (merged into providers section)

**Rationale**:
- **User intent**: If user explicitly specifies external provider, they want that version
- **Deployment-time wins**: Deploy-time decision overrides build-time default
- **Flexibility**: Allows overriding distribution providers without forking image
- **Clear hierarchy**: Distribution < ConfigMap < ExternalProviders

**Conflict Resolution**:

**Scenario 1**: External provider same ID as ConfigMap provider
```
Result: External provider WINS
Action: Log WARNING, use external provider
Message: "Provider ID 'vllm-inference' in both ConfigMap and externalProviders. Using external provider."
```

**Scenario 2**: Two external providers same ID
```
Result: FAIL deployment
Action: Return error, set LLSD status to Failed
Message: "Duplicate provider_id 'my-provider' in externalProviders: found in inference[0] and inference[1]"
```

**Implementation Detail**: User ConfigMap completely replaces base run.yaml (not merged), then external providers are merged into whichever exists.

---

### 7. Deterministic Ordering of Init Containers

**Decision**: Init containers MUST be ordered alphabetically by provider ID.

**Rationale**:
- **Predictability**: Same inputs always produce same order
- **Debuggability**: Easier to trace which provider installed when
- **Testing**: Reproducible test results
- **Documentation**: Can reference "first provider" meaningfully

**Implementation**:
```go
// Collect all providers
allProviders := collectAllProviders(instance)

// Sort alphabetically by provider ID
sort.Slice(allProviders, func(i, j int) bool {
    return allProviders[i].ProviderID < allProviders[j].ProviderID
})

// Generate init containers in sorted order
for _, provider := range allProviders {
    initContainers = append(initContainers, generateInitContainer(provider))
}
```

**Alternative Considered**: Random order or API-type grouping
- **Rejected**: Non-deterministic behavior is hard to debug
- **Rejected**: API-type grouping still needs secondary sort

---

### 8. Error Message Design Philosophy

**Principle**: Every error message MUST be actionable - user can resolve without deep operator knowledge.

**Required Elements**:
1. **What failed**: Clear identification of the failure
2. **Context**: Provider ID, image, init container name
3. **Root cause**: Why it failed (missing file, conflict, mismatch)
4. **Resolution**: Specific steps to fix

**Example: Dependency Conflict**
```
ERROR: Cannot install provider packages due to dependency conflict:

provider-a 1.0.0 depends on requests==2.28.0
provider-b 1.0.0 depends on requests==2.31.0

Resolution: Update provider images to use compatible dependency versions.

Conflicting providers:
  - custom-vllm (quay.io/myvendor/llama-stack-provider-vllm:v1.0.0)
  - another-provider (quay.io/partner/provider:v1.0.0)
```

**Key Design Choices**:
- Include BOTH provider IDs and images (user needs both to fix)
- Show conflicting package names and versions (pinpoint the issue)
- Provide concrete resolution (not just "fix dependencies")
- Format for readability (whitespace, bullets, clear sections)

---

### 9. PYTHONPATH Precedence

**Decision**: External providers PREPEND to PYTHONPATH (take precedence).

**Rationale**:
- **User intent**: If user adds external provider, they want it used
- **Override capability**: Allows replacing base provider implementations
- **Debugging**: External version easier to test/validate
- **Consistency**: Matches config merge precedence (external > base)

**Implementation**:
```bash
PYTHONPATH=/opt/llama-stack/external-providers/python-packages:$EXISTING_PYTHONPATH
```

**Edge Case**: External provider overrides base Python package
- **Behavior**: External version used
- **Warning**: None (this is expected behavior)
- **Risk Mitigation**: Preflight validation checks import works

---

### 10. API Validation: CRD Section vs. Image Declaration

**Decision**: Operator MUST validate that provider's declared API matches CRD section placement.

**Rationale**:
- **Catch user errors**: Easy to put provider in wrong section
- **Fail early**: Better than runtime error when provider doesn't work
- **Clear error**: Can tell user exactly how to fix (move to correct section)

**Validation Logic**:
```go
// Provider placed in: externalProviders.inference
// Provider declares in lls-provider-spec.yaml: api: safety

if metadata.Spec.API != crdAPISection {
    return APIPlacementError{
        ProviderID:  "custom-provider",
        DeclaredAPI: "safety",
        PlacedInAPI: "inference",
        Suggestion:  "Move to externalProviders.safety",
    }
}
```

**Error Message**:
```
ERROR: Provider API type mismatch

Provider 'custom-shield' (image: quay.io/vendor/shield:v1.0)
declares api=safety in lls-provider-spec.yaml
but is placed under externalProviders.inference

Resolution: Move the provider to externalProviders.safety section in the LLSD spec.
```

---

## Technical Insights

### Python Package Import Mechanics

**How llama-stack discovers providers**:
1. Looks for module specified in `run.yaml` `module:` field
2. Imports module: `importlib.import_module(module_name)`
3. Calls `get_provider_spec()` to get provider metadata
4. Validates provider implements required API interface

**Why PYTHONPATH works**:
- Adding to PYTHONPATH makes packages importable
- No special llama-stack configuration needed
- Standard Python import mechanism

**Init container installation**:
```bash
pip install *.whl --target /shared/python-packages --no-index
```

**Main container import**:
```bash
export PYTHONPATH=/shared/python-packages:$PYTHONPATH
python -c "import llama_stack_provider_custom_vllm"  # Works!
```

### Kubernetes Volume Sharing Mechanics

**emptyDir volume lifecycle**:
- Created when pod is scheduled to node
- Persists through container restarts
- Deleted when pod is removed from node
- Shared among all containers in pod (init + main)

**Why emptyDir works for us**:
- ✅ Writable by init containers
- ✅ Readable by main container
- ✅ No size limits (unlike ConfigMap)
- ✅ Automatic cleanup on pod deletion
- ✅ No persistent storage needed

**Volume mount strategy**:
```yaml
# Init containers write to /shared
volumeMounts:
  - name: external-providers
    mountPath: /shared

# Main container reads from /opt/llama-stack/external-providers
volumeMounts:
  - name: external-providers
    mountPath: /opt/llama-stack/external-providers
    readOnly: true  # Prevent accidental writes
```

---

## Brainstorming Questions and Answers

### Q: Should provider metadata be in CRD or image?

**Answer**: Image only.

**Reasoning**: Provider type, module name, and API are intrinsic to the provider implementation, not user configuration. The image is the source of truth. Putting it in CRD would:
- Create potential for mismatch
- Increase user burden (have to know provider internals)
- Duplicate information
- Make provider distribution harder (need to update CRD whenever provider changes)

### Q: How to handle dependency conflicts?

**Answer**: Fail early with detailed error message.

**Reasoning**: Dependency conflicts are serious - they prevent correct functionality. Better to fail deployment and force user to resolve than silently use wrong version. The error message must identify:
- Which packages conflict
- Which provider images are involved
- How to fix (update images to compatible versions)

### Q: Should we download dependencies from PyPI during installation?

**Answer**: No - all dependencies must be in provider image.

**Reasoning**:
- **Security**: No network access during pod startup
- **Reliability**: Don't depend on external service availability
- **Reproducibility**: Same image always installs same dependencies
- **Speed**: No download time, faster startup
- **Compliance**: Some environments prohibit outbound network from pods

### Q: What if provider has native code for wrong architecture?

**Answer**: Preflight check detects and fails with clear error (separate spec).

**Reasoning**: Native code architecture mismatch causes cryptic failures. Better to validate upfront and tell user "provider built for x86_64 but running on arm64, rebuild provider for correct architecture."

### Q: How to handle provider updates (new image version)?

**Answer**: Deployment rolling update automatically handles it.

**Reasoning**: Changing provider image reference in LLSD triggers Deployment update, which creates new pods with new init containers, downloads new image, installs new provider. Old pods terminate gracefully. Standard Kubernetes behavior - no special handling needed.

---

## Future Evolution Considerations

### Level 2a: Sidecar Providers

**When to implement**: When users need:
- Independent resource limits per provider
- Process isolation between providers
- Ability to restart provider without restarting llama-stack server

**Changes required**:
- Provider container spec in CRD
- Service discovery between containers
- Liveness/readiness probes per provider
- Network policy for pod-local communication

### Level 2b: External Pod Providers

**When to implement**: When users need:
- Independent scaling of providers
- Provider running in different namespace
- Provider shared across multiple LLSD instances

**Changes required**:
- Provider deployment CRD (or reference)
- Service discovery across pods
- Network policy for cross-pod communication
- Provider health monitoring

### Provider Catalog/Marketplace

**When to implement**: When ecosystem of providers grows

**Features**:
- Registry of verified providers
- Version compatibility matrix
- Automated discovery
- Security scanning results

---

## Lessons for Future Features

1. **Contracts vs. Implementation**: Keep spec focused on WHAT (requirements, behaviors, API), defer HOW to plan
2. **User Experience First**: Design error messages before implementing error handling
3. **Single Source of Truth**: Avoid duplicating information (providerType learned this lesson)
4. **Fail Early, Fail Clearly**: Validation errors better than runtime failures
5. **Standard Patterns**: Use Kubernetes native patterns (init containers, emptyDir) over custom solutions
6. **Evolutionary Design**: Support both approaches (bundled/separate wheels) when trade-offs exist

---

## Questions for Implementation

These arose during design but need to be decided during implementation:

1. **Should we cache provider metadata in operator memory or re-read each reconciliation?**
   - Trade-off: Performance vs. memory usage vs. freshness

2. **What retry policy for provider image pull failures?**
   - Kubernetes default backoff? Custom retry logic?

3. **Should init containers have resource limits?**
   - Probably yes - but what defaults? Configurable per provider?

4. **How to test provider functionality without running full llama-stack server?**
   - Mock llama-stack dependencies? Integration test harness?

5. **Should we validate provider interface compliance in preflight or let llama-stack handle it?**
   - Preflight advantage: fail before server starts
   - llama-stack advantage: validation always matches llama-stack version

---

## References

- [Kubernetes Init Containers](https://kubernetes.io/docs/concepts/workloads/pods/init-containers/)
- [Python Wheel Format](https://packaging.python.org/en/latest/specifications/binary-distribution-format/)
- [llama-stack External Providers Documentation](https://llamastack.github.io/docs/providers/external)
- [Operator Pattern Best Practices](https://kubernetes.io/docs/concepts/extend-kubernetes/operator/)

---

**End of Implementation Notes**

These notes should be referenced during implementation to understand the "why" behind design decisions. They capture context that isn't appropriate for the spec (which is about WHAT) or the plan (which is about HOW).
