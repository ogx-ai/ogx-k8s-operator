# Alternative: Init Container Config Extraction

**Status**: Deferred
**Reason**: OCI label-based approach chosen for simplicity and single-phase reconciliation
**Revisit When**: If OCI label approach proves problematic in specific registry environments

## Overview

This document describes an alternative approach for extracting the distribution's base `config.yaml` using an init container instead of OCI registry API calls.

## Architecture

```
┌─────────────────────────────────────────────────────────────┐
│  Pod                                                        │
│  ┌─────────────────────┐    ┌─────────────────────────────┐ │
│  │ Init: config-extract│───►│ Main: llama-stack           │ │
│  │ image: distribution │    │ mounts: /config/config.yaml │ │
│  │ copies config.yaml  │    │                             │ │
│  │ to shared volume    │    │                             │ │
│  └─────────────────────┘    └─────────────────────────────┘ │
│           │                            ▲                    │
│           ▼                            │                    │
│  ┌─────────────────────────────────────┴───────────────────┐│
│  │  EmptyDir Volume: config-extract                        ││
│  │  /extract/config.yaml                                   ││
│  └─────────────────────────────────────────────────────────┘│
└─────────────────────────────────────────────────────────────┘
```

## Two-Phase Reconciliation

Since the operator needs the base config to generate the final ConfigMap, but the config lives inside the image, this approach uses two reconciliation phases:

### Phase 1 (First Reconcile)
Deploy with init container that extracts and stores base config:
- Init container copies `/app/config.yaml` to a ConfigMap (`{name}-base-config`)
- Main container starts with distribution defaults
- Operator watches for the base config ConfigMap

### Phase 2 (Subsequent Reconciles)
Generate merged config using cached base:
- Operator reads base config from `{name}-base-config` ConfigMap
- Merges user overrides from CR spec
- Creates `{name}-config-{hash}` ConfigMap with final config
- Updates Deployment to use generated config

## Init Container Spec

```yaml
initContainers:
  - name: config-extract
    image: {{ .Distribution.Image }}
    command:
      - /bin/sh
      - -c
      - |
        if [ -f /app/config.yaml ]; then
          cp /app/config.yaml /extract/config.yaml
        else
          echo "version: 2" > /extract/config.yaml
          echo "# No base config found" >> /extract/config.yaml
        fi
    volumeMounts:
      - name: config-extract
        mountPath: /extract
```

## Base Config Caching

```go
// BaseConfigCache stores extracted configs in ConfigMaps for reuse
type BaseConfigCache struct {
    client    client.Client
    namespace string
}

// GetOrCreate returns cached base config or triggers extraction
func (c *BaseConfigCache) GetOrCreate(ctx context.Context, name, image string) (*BaseConfig, error) {
    cmName := fmt.Sprintf("%s-base-config", name)

    var cm corev1.ConfigMap
    err := c.client.Get(ctx, types.NamespacedName{Name: cmName, Namespace: c.namespace}, &cm)
    if err != nil {
        if apierrors.IsNotFound(err) {
            // Base config not yet extracted, return nil to trigger Phase 1
            return nil, nil
        }
        return nil, err
    }

    // Parse cached config
    var config BaseConfig
    if err := yaml.Unmarshal([]byte(cm.Data["config.yaml"]), &config); err != nil {
        return nil, fmt.Errorf("failed to parse cached base config: %w", err)
    }

    // Verify image matches (invalidate cache if distribution changed)
    if cm.Annotations["llamastack.io/source-image"] != image {
        // Image changed, delete cached config to trigger re-extraction
        if err := c.client.Delete(ctx, &cm); err != nil {
            return nil, err
        }
        return nil, nil
    }

    return &config, nil
}
```

## Sidecar Alternative

An alternative using a sidecar that uploads the base config on first start:

```yaml
initContainers:
  - name: config-upload
    image: {{ .Distribution.Image }}
    command:
      - /bin/sh
      - -c
      - |
        # Extract and upload to a known ConfigMap via kubectl/API
        cat /app/config.yaml | base64 > /extract/config.b64
    volumeMounts:
      - name: config-extract
        mountPath: /extract
```

## Key Functions

```go
func (c *BaseConfigCache) GetOrCreate(ctx context.Context, name, image string) (*BaseConfig, error)
func (c *BaseConfigCache) Invalidate(ctx context.Context, name string) error
func NeedsBaseConfigExtraction(instance *v1alpha2.LlamaStackDistribution) bool
```

## Reconciliation Flow

```go
func (r *Reconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
    // ... fetch instance ...

    // Check if base config is available
    baseConfig, err := r.baseConfigCache.GetOrCreate(ctx, instance.Name, image)
    if err != nil {
        return ctrl.Result{}, err
    }

    if baseConfig == nil {
        // Phase 1: Deploy with init container to extract base config
        return r.reconcileWithInitContainer(ctx, instance)
    }

    // Phase 2: Generate merged config
    return r.reconcileWithGeneratedConfig(ctx, instance, baseConfig)
}
```

## Pros

- Leverages Kubernetes native image pulling (imagePullSecrets, private registries)
- No external registry API calls from operator
- Works in air-gapped environments without registry API access
- Base config cached in ConfigMap (survives operator restarts)
- No changes required to distribution image build process

## Cons

- First deployment requires two reconciliation cycles
- Slight startup latency for init container execution
- Requires cleanup of base-config ConfigMap on CR deletion
- More complex reconciliation state machine
- Race conditions possible during initial extraction

## When to Consider This Approach

1. Registry does not support OCI manifest/config blob fetching
2. Distribution images cannot be modified to include OCI labels
3. Very strict network policies prevent operator from accessing registry
4. Need to support legacy distribution images without labels

## Migration Path

If switching from OCI label approach to init container:

1. Add fallback logic in extractor to detect missing labels
2. Deploy init container when labels not present
3. Cache extracted config in ConfigMap
4. Subsequent reconciles use cached config
