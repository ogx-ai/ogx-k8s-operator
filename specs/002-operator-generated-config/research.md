# Research: Operator-Generated Server Configuration (v1alpha2)

**Spec**: 002-operator-generated-config
**Created**: 2026-02-10
**Status**: Complete

## Technical Context

**Language/Version**: Go 1.25 (from go.mod)
**Primary Dependencies**: controller-runtime v0.22.4, kubebuilder, kustomize/api v0.21.0, client-go v0.34.3, go-containerregistry v0.20.7
**Storage**: Kubernetes ConfigMaps (generated), Secrets (referenced via secretKeyRef)
**Testing**: Go test, envtest (controller-runtime), testify v1.11.1
**Target Platform**: Kubernetes 1.30+ (controller-runtime v0.22.x baseline)
**Project Type**: Kubernetes operator (single binary)
**Performance Goals**: Config generation < 5 seconds (NFR-002)
**Constraints**: Namespace-scoped RBAC, air-gapped registry support, deterministic output

---

## Research Areas

### R1: Provider and Resource Type Design

**Decision**: Use typed `[]ProviderConfig` slices for all provider fields. A single provider is expressed as a one-element list. Use `[]ModelConfig` (with only `name` required) for models. No polymorphic types.

**Rationale** (revised after PR #253 review):
The original design used `json.RawMessage` / `apiextensionsv1.JSON` for polymorphic fields (single object OR list). PR #253 review revealed this creates several problems:
1. Kubebuilder validation markers don't apply to opaque JSON fields
2. CEL rules (FR-071, FR-072) cannot inspect opaque JSON, making provider ID validation impossible at admission time
3. ~500 lines of parsing code needed (`ParsePolymorphicProvider`, `collectSecretRefsFromProviderField`, etc.)
4. Heuristic secret detection (`extractDirectSecretRef` matching any map with `name`+`key`) produces false positives
5. Contradicts constitution §2.1 ("MUST use kubebuilder validation tags")

**Trade-off**: Users write `- provider: vllm` (list syntax) instead of `provider: vllm` (object syntax). This is a one-character YAML difference per provider, versus eliminating all parsing complexity and enabling full CRD validation.

**Alternatives considered**:
- `json.RawMessage` with custom unmarshaling (original design): Rejected due to above issues
- `apiextensionsv1.JSON`: Same problems, plus no IDE autocompletion
- Separate fields (`InferenceProvider` + `InferenceProviders`): Verbose, users must choose correct field

**Implementation pattern**:
```go
type ProvidersSpec struct {
    Inference   []ProviderConfig `json:"inference,omitempty"`
    Safety      []ProviderConfig `json:"safety,omitempty"`
    VectorIo    []ProviderConfig `json:"vectorIo,omitempty"`
    ToolRuntime []ProviderConfig `json:"toolRuntime,omitempty"`
    Telemetry   []ProviderConfig `json:"telemetry,omitempty"`
}

type ProviderConfig struct {
    ID         string                       `json:"id,omitempty"`
    Provider   string                       `json:"provider"`
    Endpoint   string                       `json:"endpoint,omitempty"`
    APIKey     *SecretKeyRef                `json:"apiKey,omitempty"`
    SecretRefs map[string]SecretKeyRef      `json:"secretRefs,omitempty"`
    Settings   apiextensionsv1.JSON         `json:"settings,omitempty"`
}
```

**Risk**: Low. The only UX cost is requiring list syntax for single providers.

---

### R2: Base Config Extraction Strategy (Phased)

**Decision**: Phase 1 embeds default configs in the operator binary via `go:embed`. Phase 2 adds OCI label extraction using `go-containerregistry`.

**Rationale**: Embedding avoids runtime dependencies on image registries, works in air-gapped environments, and requires no changes to upstream distribution image builds. OCI labels add flexibility for custom images in Phase 2.

**Alternatives considered**:
- **Init container extraction**: Run a short-lived container from the distribution image to extract config. Rejected because it requires the image to have a shell/cat binary and adds startup latency. Also problematic for air-gapped registries where init container images may not be available.
- **Runtime image pull + tar extract**: Pull the full image at reconcile time. Rejected due to image size (hundreds of MB), slow network dependencies, and complexity of registry authentication at runtime.
- **ConfigMap-only approach**: Require users to always provide a ConfigMap. Rejected because it undermines the core value proposition (minimal YAML).

**Phase 1 file layout**:
```
configs/
├── starter/config.yaml
├── remote-vllm/config.yaml
├── meta-reference-gpu/config.yaml
└── postgres-demo/config.yaml
```

**Build-time validation**: Makefile target `validate-configs` ensures every entry in `distributions.json` has a corresponding embedded config file.

**Phase 2 OCI labels**:
- `io.llamastack.config.base64`: Inline config for small configs (< 50KB)
- `io.llamastack.config.layer` + `io.llamastack.config.path`: Layer reference for large configs
- Uses `k8schain` from `go-containerregistry` for same auth as kubelet
- Caches by image digest to avoid repeated registry fetches

---

### R3: Config Merging Strategy

**Decision**: Deep merge user configuration over base config using a recursive map merge, with provider replacement semantics (user providers replace base providers by API type, not merge into them).

**Rationale**: Users expect that specifying `providers.inference` replaces the entire inference provider section from the base config, not merges individual fields. This matches Kubernetes strategic merge patch behavior for typed fields.

**Alternatives considered**:
- **JSON Merge Patch (RFC 7386)**: Simple but cannot handle arrays properly (replaces entire arrays). Models and tools are arrays, so this is insufficient.
- **JSON Patch (RFC 6902)**: Too low-level for user-facing configuration. Users would need to specify operations (add, replace, remove).
- **Strategic Merge Patch**: Kubernetes-native but requires CRD schema annotations for merge keys. Adds complexity to the internal config model.

**Merge rules**:
1. Provider sections: Replace entire API type block (e.g., `inference` section is fully replaced if user specifies it)
2. Storage sections: Merge at subsection level (`kv`, `sql` each independently replaced if specified)
3. Resources: Additive (user resources added to base registered_resources)
4. Disabled APIs: Subtractive (remove matching APIs from generated config)
5. Scalar fields: User value overrides base value

---

### R4: Environment Variable Naming for Secrets

**Decision**: `LLSD_<PROVIDER_ID>_<FIELD>` where provider ID is uppercased with hyphens replaced by underscores.

**Rationale**: Provider IDs are unique across all provider types (enforced by FR-072 CEL validation), making them a collision-free namespace for env var names. Using provider type alone would collide when multiple providers share the same type.

**Alternatives considered**:
- `LLSD_<API_TYPE>_<PROVIDER_TYPE>_<FIELD>`: Verbose. Still collides when multiple providers of the same type serve the same API (unlikely but possible with external providers from spec 001).
- `LLSD_<INDEX>_<FIELD>`: Fragile. Index-based naming breaks when providers are reordered.
- Hash-based names: Not human-readable, hard to debug.

**Normalization rules**:
- Hyphens (`-`) become underscores (`_`)
- All uppercase
- Example: provider ID `vllm-primary`, field `apiKey` produces `LLSD_VLLM_PRIMARY_API_KEY`

---

### R5: Conversion Webhook Architecture

**Decision**: v1alpha2 is the hub (storage version). v1alpha1 is the spoke with `ConvertTo` (v1alpha1 to v1alpha2) and `ConvertFrom` (v1alpha2 to v1alpha1) methods.

**Rationale**: Standard kubebuilder conversion pattern. The newest API version is the hub because it has the richest schema. All storage and reconciliation operates on v1alpha2 internally.

**Alternatives considered**:
- **Internal version as hub**: Common in core Kubernetes but adds a third representation layer. Overkill for an operator with two API versions.
- **No conversion (breaking change)**: Forces users to migrate immediately. Rejected because backward compatibility is a stated priority (User Story 7, P2).

**Data loss on down-conversion**: New v1alpha2 fields (`providers`, `resources`, `storage`, `disabled`) cannot be represented in v1alpha1. These are silently dropped during v1alpha2-to-v1alpha1 conversion with a warning logged. Round-trip v1alpha2 -> v1alpha1 -> v1alpha2 loses these fields.

**Annotation preservation**: To mitigate round-trip data loss, the spoke conversion stores v1alpha2-only fields as a JSON annotation (`llamastack.io/v1alpha2-fields`) on the v1alpha1 object. The v1alpha1-to-v1alpha2 conversion restores from this annotation if present. This is the standard pattern used by Kubernetes built-in resources.

---

### R6: Validating Webhook vs Controller Validation

**Decision**: Use both. CEL rules (CRD-level) for structural validation, validating webhook for cross-resource existence checks, and controller-level validation for runtime state verification.

**Rationale**: Each validation layer serves a different purpose:
- **CEL (CRD)**: Cheapest, runs at admission without operator involvement. Handles structural constraints like mutual exclusivity.
- **Webhook**: Runs at admission with access to the API server. Can check Secret/ConfigMap existence for fast-fail feedback.
- **Controller**: Runs during reconciliation. Handles drift (Secret deleted after CR creation) and complex cross-resource validation.

**Alternatives considered**:
- **Controller-only validation**: Slower feedback loop. Users apply a CR, wait for reconciliation, then discover errors in status. Poor UX.
- **Webhook-only validation**: Cannot handle drift. A Secret existing at admission may be deleted before reconciliation.

**Webhook scope**: Limited to checks that benefit from admission-time feedback:
- Secret existence (fast-fail on typos in secret names)
- ConfigMap existence for `overrideConfig` and `caBundle`
- Provider ID cross-references in `resources.models[].provider`

---

### R7: Immutable ConfigMap Pattern

**Decision**: Create a new ConfigMap with a content-hash suffix on every config change. Keep the last 2 ConfigMaps for rollback visibility. Set owner references for automatic garbage collection.

**Rationale**: Immutable ConfigMaps ensure that running Pods always see a consistent config. Updating a ConfigMap in-place risks a running Pod seeing a partially-updated config if the kubelet hasn't refreshed its cache.

**Alternatives considered**:
- **Mutable ConfigMap with hash annotation on Deployment**: Simpler but kubelet ConfigMap cache can serve stale data for up to the sync period (default 1 minute). Risk of config/Pod mismatch during transition.
- **ConfigMap with `immutable: true` field**: Kubernetes native immutable ConfigMaps cannot be updated (by design), so this requires creating new ConfigMaps anyway. Compatible with our approach.

**Naming convention**: `{cr-name}-config-{hash[:8]}` where hash is SHA256 of the config.yaml content.

**Cleanup**: Retain last 2 ConfigMaps (current + previous for debugging). Older ones are garbage collected via owner references when the CR is deleted, or explicitly cleaned up by the controller.

---

### R8: Atomic Deployment Updates

**Decision**: Apply image, ConfigMap volume, env vars, and hash annotation in a single `client.Update()` call to the Deployment resource.

**Rationale**: Prevents intermediate states where the running image and config are mismatched. A single API server update means the Deployment controller sees a consistent desired state.

**Alternatives considered**:
- **Sequential updates (image first, then config)**: Risk of a Pod starting with new image but old config, or vice versa.
- **Delete and recreate Deployment**: Causes downtime. Not acceptable for production workloads.
- **Server-side apply**: More complex, but could work. Standard `client.Update()` is sufficient since the operator is the sole writer of the Deployment.

**Failure handling**: If the `client.Update()` call fails, the Deployment retains its previous state (API server is transactional for single resources). The controller sets `ConfigGenerated=False` in status and does not retry until the next reconciliation.

---

## Integration Patterns

### Spec 001 Integration (External Providers)

External providers from spec 001 are merged after the generated config is produced. The merge is additive: external providers are appended to the providers list. On ID conflict, the external provider overrides the inline provider with a warning logged.

The integration point is after config generation and before ConfigMap creation (step 9 in the Controller Flow).

### Distribution Image Resolution

The resolution chain is:
1. Check `image-overrides` in the operator ConfigMap (allows downstream builds like RHOAI to override images)
2. Look up `distribution.name` in `distributions.json`
3. Record resolved image in `status.resolvedDistribution.image`
4. On subsequent reconciliation, compare with stored image to detect operator upgrade changes

---

## Summary of Decisions

| Area | Decision | Risk Level |
|------|----------|------------|
| Provider/resource types | Typed `[]ProviderConfig` slices, `[]ModelConfig` | Low (full schema validation) |
| Base config source | Embedded `go:embed` (Phase 1) + OCI labels (Phase 2) | Low |
| Config merging | Deep merge with provider replacement semantics | Low |
| Env var naming | `LLSD_<PROVIDER_ID>_<FIELD>` | Low |
| Conversion | v1alpha2 hub, v1alpha1 spoke with annotation preservation | Low |
| Validation layers | CEL + webhook + controller | Low |
| ConfigMap pattern | Immutable with content-hash suffix | Low |
| Deployment updates | Single atomic `client.Update()` | Low |
