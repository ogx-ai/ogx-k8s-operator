# Extra Providers Schema

**Purpose**: Define the schema for `extra-providers.yaml` - a standardized format for external provider configuration that enables both current (merge-based) and future (native LlamaStack support) implementations.

**Created**: 2025-11-13

---

## Executive Summary

The `extra-providers.yaml` file is a **forward-compatible** configuration format for external providers:

**Current Implementation** (Phase 1):
- Operator generates `extra-providers.yaml` as ConfigMap
- Merge init container combines it with base run.yaml
- LlamaStack receives merged run.yaml

**Future Implementation** (Phase 2 - when LlamaStack adds support):
- Operator generates `extra-providers.yaml` as ConfigMap (same as Phase 1)
- LlamaStack started with `--extra-providers /etc/extra-providers.yaml`
- LlamaStack handles merge internally
- **No operator changes needed** - just remove merge init container

---

## Schema Definition

### File Format: `extra-providers.yaml`

```yaml
apiVersion: llamastack.io/v1alpha1
kind: ExternalProviders

# Providers organized by API type (matches run.yaml structure)
providers:
  inference:
    - provider_id: my-vllm-inference
      provider_type: remote::vllm
      module: my_org.vllm_provider
      config:
        url: http://vllm.default.svc.cluster.local:8000
        api_token: ${VLLM_API_TOKEN}

  safety:
    - provider_id: my-custom-safety
      provider_type: inline::custom-safety
      module: my_org.safety_provider
      config:
        safety_level: high

  agents:
    - provider_id: my-agent-provider
      provider_type: remote::custom-agents
      module: my_org.agents_provider
      config:
        endpoint: http://agents.default.svc.cluster.local:9000
```

### Schema Structure

```yaml
# Top-level metadata
apiVersion: string  # Always "llamastack.io/v1alpha1"
kind: string        # Always "ExternalProviders"

# Provider definitions (organized by API type)
providers:
  <api-type>:  # One of: inference, safety, agents, vector_io, datasetio, scoring, eval, tool_runtime, post_training
    - provider_id: string       # REQUIRED: Unique instance identifier
      provider_type: string     # REQUIRED: Provider type (from metadata)
      module: string            # REQUIRED: Python module path (from metadata)
      config: object            # OPTIONAL: Provider-specific configuration (from CRD)
```

### Field Mapping

| Field | Source | Description |
|-------|--------|-------------|
| `provider_id` | CRD `externalProviders.<api>.<n>.providerId` | User-assigned unique identifier |
| `provider_type` | Provider metadata `spec.providerType` | Provider type (e.g., "remote::vllm") |
| `module` | Provider metadata `spec.packageName` | Python module path (e.g., "my_org.custom_vllm") used by LlamaStack to import provider via `importlib.import_module()` |
| `config` | CRD `externalProviders.<api>.<n>.config` | Provider-specific configuration |

---

## Compatibility with run.yaml

The `extra-providers.yaml` schema **exactly matches** the `providers` section of `run.yaml`:

**run.yaml format**:
```yaml
version: 2
image_name: llamastack/distribution-remote-vllm
apis:
  - inference
  - safety

providers:
  inference:
    - provider_id: vllm-inference
      provider_type: remote::vllm
      config:
        url: http://localhost:8000

  safety:
    - provider_id: llama-guard
      provider_type: inline::llama-guard
      config:
        model: meta-llama/Llama-Guard-3-8B
```

**extra-providers.yaml format** (IDENTICAL provider structure):
```yaml
apiVersion: llamastack.io/v1alpha1
kind: ExternalProviders

providers:
  inference:
    - provider_id: my-custom-inference
      provider_type: remote::custom
      module: my_org.inference_provider
      config:
        url: http://custom:8000
```

**Merge result** (extra-providers appends to base providers):
```yaml
version: 2
image_name: llamastack/distribution-remote-vllm
apis:
  - inference
  - safety

providers:
  inference:
    - provider_id: vllm-inference        # From base
      provider_type: remote::vllm
      config:
        url: http://localhost:8000

    - provider_id: my-custom-inference   # From extra-providers
      provider_type: remote::custom
      module: my_org.inference_provider
      config:
        url: http://custom:8000

  safety:
    - provider_id: llama-guard            # From base
      provider_type: inline::llama-guard
      config:
        model: meta-llama/Llama-Guard-3-8B
```

---

## Generation Logic (Controller)

### Function: `generateExtraProvidersYaml()`

**File**: `controllers/extra_providers_config.go` (new file)

```go
package controllers

import (
    "fmt"

    "gopkg.in/yaml.v3"
    llamav1alpha1 "github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
    "github.com/llamastack/llama-stack-k8s-operator/pkg/provider"
)

// ExtraProvidersYaml represents the extra-providers.yaml structure
type ExtraProvidersYaml struct {
    APIVersion string                                `yaml:"apiVersion"`
    Kind       string                                `yaml:"kind"`
    Providers  map[string][]ExtraProviderDefinition `yaml:"providers"`
}

type ExtraProviderDefinition struct {
    ProviderID   string                 `yaml:"provider_id"`
    ProviderType string                 `yaml:"provider_type"`
    Module       string                 `yaml:"module"`
    Config       map[string]interface{} `yaml:"config,omitempty"`
}

// generateExtraProvidersYaml creates extra-providers.yaml content from LLSD CR and provider metadata
func generateExtraProvidersYaml(
    instance *llamav1alpha1.LlamaStackDistribution,
    metadataDir string,
) (*ExtraProvidersYaml, error) {
    extraProviders := &ExtraProvidersYaml{
        APIVersion: "llamastack.io/v1alpha1",
        Kind:       "ExternalProviders",
        Providers:  make(map[string][]ExtraProviderDefinition),
    }

    // Collect all providers in CRD order
    allProviders := collectProvidersInCRDOrder(instance)

    for _, p := range allProviders {
        // Read provider metadata (copied by init container)
        metadataPath := fmt.Sprintf("%s/%s.yaml", metadataDir, p.ref.ProviderID)
        metadata, err := provider.LoadProviderMetadata(metadataPath)
        if err != nil {
            return nil, fmt.Errorf("failed to load metadata for provider %s: %w", p.ref.ProviderID, err)
        }

        // Create provider definition
        providerDef := ExtraProviderDefinition{
            ProviderID:   p.ref.ProviderID,
            ProviderType: metadata.Spec.ProviderType,
            Module:       metadata.Spec.PackageName,
            Config:       convertJSONToMap(p.ref.Config),
        }

        // Add to appropriate API type
        extraProviders.Providers[p.api] = append(extraProviders.Providers[p.api], providerDef)
    }

    return extraProviders, nil
}

// serializeExtraProvidersYaml converts struct to YAML bytes
func serializeExtraProvidersYaml(extraProviders *ExtraProvidersYaml) ([]byte, error) {
    return yaml.Marshal(extraProviders)
}
```

---

## ConfigMap Generation (Controller)

### Function: `reconcileExtraProvidersConfigMap()`

**File**: `controllers/llamastackdistribution_controller.go`

```go
func (r *LlamaStackDistributionReconciler) reconcileExtraProvidersConfigMap(
    ctx context.Context,
    instance *llamav1alpha1.LlamaStackDistribution,
) error {
    logger := log.FromContext(ctx)

    if instance.Spec.Server.ExternalProviders == nil {
        // No external providers - no ConfigMap needed
        return nil
    }

    // Note: We generate this WITHOUT reading metadata files
    // Metadata will be read by merge init container after provider init containers complete
    // For now, we create the structure based on CRD only

    extraProviders := &ExtraProvidersYaml{
        APIVersion: "llamastack.io/v1alpha1",
        Kind:       "ExternalProviders",
        Providers:  make(map[string][]ExtraProviderDefinition),
    }

    // Collect all providers - but we can't populate provider_type/module yet
    // Those come from metadata files that don't exist until provider init containers run
    // So we create a placeholder that merge init container will populate

    configMapName := fmt.Sprintf("%s-extra-providers", instance.Name)
    configMapData := fmt.Sprintf(`apiVersion: llamastack.io/v1alpha1
kind: ExternalProviders

# This file will be populated by the merge init container
# after provider metadata is available from provider init containers

providers: {}
`)

    configMap := &corev1.ConfigMap{
        ObjectMeta: metav1.ObjectMeta{
            Name:      configMapName,
            Namespace: instance.Namespace,
            Labels: map[string]string{
                "app.kubernetes.io/name":       "llama-stack",
                "app.kubernetes.io/instance":   instance.Name,
                "app.kubernetes.io/component":  "extra-providers",
                "app.kubernetes.io/managed-by": "llama-stack-operator",
            },
        },
        Data: map[string]string{
            "extra-providers.yaml": configMapData,
        },
    }

    // Set owner reference for auto-cleanup
    if err := ctrl.SetControllerReference(instance, configMap, r.Scheme); err != nil {
        return fmt.Errorf("failed to set controller reference: %w", err)
    }

    // Create or update ConfigMap
    if err := r.createOrUpdateConfigMap(ctx, configMap); err != nil {
        return fmt.Errorf("failed to create/update extra-providers ConfigMap: %w", err)
    }

    logger.Info("Created extra-providers ConfigMap", "configMap", configMapName)
    return nil
}
```

**NOTE**: The controller creates a **placeholder** ConfigMap. The merge init container will populate it with actual provider definitions after reading metadata files.

---

## Current Implementation (Phase 1): Merge Init Container

### Updated Merge Init Container

**Purpose**: Generate `extra-providers.yaml` from metadata, then merge with base run.yaml

```yaml
initContainers:
- name: merge-config
  image: <operator-image>
  command: ["/usr/local/bin/merge-run-yaml"]
  args:
    - "--base=/etc/base-config/run.yaml"           # User ConfigMap (if exists) or empty
    - "--metadata-dir=/opt/external-providers/metadata"
    - "--extra-providers-output=/shared/extra-providers.yaml"  # Generate this first
    - "--output=/shared/final/run.yaml"            # Final merged output
  volumeMounts:
    - name: config-merge
      mountPath: /shared
    - name: external-providers
      mountPath: /opt/external-providers
      readOnly: true
    - name: user-config-source  # User ConfigMap (if exists)
      mountPath: /etc/base-config
      readOnly: true
```

**Merge Tool Logic**:
```go
func main() {
    // 1. Generate extra-providers.yaml from metadata files
    extraProviders := generateExtraProvidersFromMetadata(metadataDir)
    writeYaml(extraProvidersOutput, extraProviders)

    // 2. Merge base run.yaml + extra-providers.yaml
    baseConfig := readYaml(basePath)
    mergedConfig := mergeProviders(baseConfig, extraProviders)
    writeYaml(outputPath, mergedConfig)
}
```

**Result**: Both `/shared/extra-providers.yaml` AND `/shared/final/run.yaml` are available

---

## Future Implementation (Phase 2): Native LlamaStack Support

### Proposed LlamaStack Enhancement

**GitHub Issue** (to be filed with LlamaStack project):

```markdown
## Feature Request: Support for `--extra-providers` flag

### Problem
External provider integration currently requires:
1. Parsing base run.yaml
2. Merging provider definitions
3. Handling schema evolution across versions

This is brittle and doesn't scale as the run.yaml schema evolves.

### Proposed Solution
Add native support for external provider files:

```bash
llama stack run /etc/llama-stack/run.yaml \
  --extra-providers /etc/extra-providers.yaml
```

### Extra Providers File Format
```yaml
apiVersion: llamastack.io/v1alpha1
kind: ExternalProviders

providers:
  inference:
    - provider_id: custom-inference
      provider_type: remote::custom
      module: custom.inference
      config:
        url: http://custom:8000
```

### Merge Semantics
- Providers from `extra-providers.yaml` are **appended** to base providers
- Duplicate `provider_id` errors (fail fast)
- API validation (provider API type must match section)

### Benefits
- ✅ Clean separation of base vs external providers
- ✅ LlamaStack handles schema evolution internally
- ✅ No external parsing/merging logic needed
- ✅ Enables Kubernetes operators, Docker Compose, etc. to inject providers cleanly
```

### Migration Path (Operator Changes)

**When LlamaStack adds `--extra-providers` support**:

**Before** (Phase 1 - manual merge):
```yaml
initContainers:
- name: merge-config
  image: operator-image
  # Generates /shared/final/run.yaml

containers:
- name: llama-stack
  command: ["/bin/sh", "-c"]
  args:
    - llama stack run /etc/llama-stack/run.yaml
  volumeMounts:
    - name: config-merge
      mountPath: /etc/llama-stack/run.yaml
      subPath: final/run.yaml
```

**After** (Phase 2 - native support):
```yaml
# No merge init container needed!

containers:
- name: llama-stack
  command: ["/bin/sh", "-c"]
  args:
    - llama stack run /etc/llama-stack/run.yaml --extra-providers /etc/extra-providers/extra-providers.yaml
  volumeMounts:
    - name: user-config-source  # User ConfigMap
      mountPath: /etc/llama-stack
      readOnly: true
    - name: extra-providers     # Generated ConfigMap
      mountPath: /etc/extra-providers
      readOnly: true
```

**Operator Code Changes**:
- ✅ Remove merge init container generation
- ✅ Keep extra-providers ConfigMap generation (no change)
- ✅ Update main container args (add `--extra-providers` flag)
- ✅ Total changes: ~20 lines modified

---

## Validation

### Controller-Side Validation

**Before creating ConfigMap**, validate:

1. **No duplicate provider IDs** across all API types
2. **Provider API type matches CRD section** (after reading metadata)
3. **Required fields present** (providerId, image)

### Merge Init Container Validation

**After reading metadata**, validate:

1. **Metadata API matches CRD API section**
2. **All required metadata fields present** (providerType, packageName)
3. **No conflicts with base providers** (same provider_id)

**Error Format**:
```
ERROR: Provider API type mismatch
Provider 'my-provider' (image: ghcr.io/org/provider:v1)
declares api=inference in lls-provider-spec.yaml
but is placed under externalProviders.safety

Resolution: Move the provider to externalProviders.inference section in the LLSD spec.
```

---

## Schema Evolution Strategy

### Version 1 (Current)
```yaml
apiVersion: llamastack.io/v1alpha1
kind: ExternalProviders

providers:
  <api-type>:
    - provider_id: string
      provider_type: string
      module: string
      config: object
```

### Future Version (Example)
```yaml
apiVersion: llamastack.io/v1alpha2  # Version bump
kind: ExternalProviders

providers:
  <api-type>:
    - provider_id: string
      provider_type: string
      module: string
      config: object
      # New fields added by LlamaStack
      health_check_endpoint: string
      retry_policy: object
```

**Handling**:
- LlamaStack owns the schema
- Operator generates what it knows (v1alpha1 fields)
- LlamaStack handles forward/backward compatibility
- **No operator changes needed** for schema evolution

---

## File Locations

### In Operator Codebase

```
api/v1alpha1/
├── llamastackdistribution_types.go  # ExternalProvidersSpec (CRD schema)

pkg/provider/
├── metadata.go                      # Provider metadata parsing
├── extra_providers.go               # extra-providers.yaml generation (NEW)

controllers/
├── extra_providers_config.go        # ConfigMap reconciliation (NEW)

cmd/merge-run-yaml/
├── main.go                          # Merge tool binary
```

### In Kubernetes Cluster

```
ConfigMaps (per LLSD instance):
  <llsd-name>-extra-providers         # Generated by operator
    └── extra-providers.yaml

Pod Volumes:
  /etc/extra-providers/
    └── extra-providers.yaml          # Mounted from ConfigMap

  /shared/
    ├── extra-providers.yaml          # Copy for merge process
    └── final/
        └── run.yaml                  # Merged result (Phase 1 only)

  /opt/external-providers/
    ├── python-packages/              # pip installed packages
    └── metadata/
        ├── provider-1.yaml           # Metadata from provider images
        └── provider-2.yaml
```

---

## Benefits of This Approach

### Phase 1 (Current - Manual Merge)
- ✅ **Clean schema** - Defined, versioned, documented
- ✅ **Testable** - Generate extra-providers.yaml independently
- ✅ **Debuggable** - Can inspect extra-providers.yaml in pod
- ✅ **No run.yaml extraction** - Don't need to find/parse distribution run.yaml

### Phase 2 (Future - Native Support)
- ✅ **Minimal migration** - Just remove merge init container, add flag
- ✅ **Schema evolution handled by LlamaStack** - No operator changes
- ✅ **Faster pod startup** - One less init container
- ✅ **Less complexity** - LlamaStack handles merge logic

### General
- ✅ **Forward compatible** - Current implementation doesn't block future enhancement
- ✅ **Clear separation** - Base config vs external providers
- ✅ **Reusable** - Other tools can use same schema (Docker Compose, Helm, etc.)

---

## Implementation Checklist

**Phase 1 (Current)**:
- [ ] Define ExtraProvidersYaml struct in pkg/provider/extra_providers.go
- [ ] Implement generateExtraProvidersYaml() function
- [ ] Create reconcileExtraProvidersConfigMap() in controller
- [ ] Update merge tool to generate extra-providers.yaml from metadata
- [ ] Update merge tool to merge extra-providers into run.yaml
- [ ] Mount extra-providers ConfigMap in merge init container
- [ ] Add validation for provider definitions
- [ ] Unit tests for generation logic
- [ ] Integration tests for merge process

**Phase 2 (Future - when LlamaStack supports it)**:
- [ ] File GitHub issue with LlamaStack project
- [ ] Wait for LlamaStack to implement `--extra-providers` flag
- [ ] Remove merge init container from operator
- [ ] Update main container args to include `--extra-providers /etc/extra-providers/extra-providers.yaml`
- [ ] Update tests to verify native integration
- [ ] Document migration in release notes

---

## Summary

The `extra-providers.yaml` schema is a **strategic design choice** that:

1. **Solves current need** - Enable external providers without LlamaStack changes
2. **Prepares for future** - Clean migration path when LlamaStack adds native support
3. **Reduces complexity** - No run.yaml extraction/parsing needed
4. **Enables evolution** - LlamaStack owns schema, handles versioning
5. **Doesn't block progress** - Can implement and deploy today

**Key Insight**: By defining a clean schema now, we make it easy for LlamaStack to adopt it later, turning our "workaround" into a standard.
