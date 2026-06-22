# Runtime Config Generation from OGXServer CR

This guide explains how to use OGX runtime config generation directly from the `OGXServer` custom resource, without hand-writing a full `config.yaml`.

## Overview

The operator can generate server `config.yaml` from declarative CR fields:

- `spec.providers`
- `spec.resources`
- `spec.storage`
- `spec.disabledAPIs`

When this mode is active, the operator resolves a base config from `spec.baseConfig` when provided, otherwise from OCI image labels, merges your CR values, writes an immutable generated ConfigMap, and mounts it into the server pod.

## Precedence Rules

The operator supports two config modes:

1. **Override mode**: `spec.overrideConfig` points to a user-managed ConfigMap key.
2. **Generated mode**: declarative CR fields generate `config.yaml`.

If `spec.overrideConfig` is set, it takes precedence over generated mode.
The mounted runtime config always comes from `spec.overrideConfig` or the
generated ConfigMap; `spec.baseConfig` is only an input to generation.

## Required Resource Labels and Namespace Scope

Any ConfigMap/Secret referenced by the CR must:

- Be in the **same namespace** as the `OGXServer`
- Have label `ogx.io/watch: "true"`

Example:

```yaml
metadata:
  labels:
    ogx.io/watch: "true"
```

This label allows the operator to watch changes and trigger reconciliation.

## How It Works

1. If `spec.baseConfig` is set, read that ConfigMap key as the base config.
2. Otherwise, resolve base config from OCI labels `com.ogx.distribution.default-config` and `com.ogx.config.<default-config-filename>` on the resolved distribution image.
3. Expand providers/resources/storage from CR spec.
4. Merge with base config:
   - providers: user values replace base per API type
   - models/resources: user values replace base models
   - storage: user value replaces base storage
   - APIs: base filtered by `disabledAPIs`
5. Create immutable ConfigMap: `${ogxserver-name}-config-${contentHash}`.
6. Mount generated `config.yaml` to `/etc/ogx/config.yaml`.
7. Inject secret-backed environment variables for provider/storage credentials.
8. Roll deployment when referenced ConfigMaps/Secrets change.

## Minimal Declarative Example

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: openai-creds
  labels:
    ogx.io/watch: "true"
stringData:
  api-key: "<token>"
---
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: generated-config-sample
spec:
  distribution:
    name: starter
  providers:
    inference:
      remote:
        openai:
          - id: openai-primary
            apiKey:
              name: openai-creds
              key: api-key
  resources:
    models:
      - name: gpt-4o-mini
        provider: openai-primary
```

Apply a ready-to-use sample from this repository:

```bash
kubectl apply -f config/samples/example-with-generated-config.yaml
```

## `configgen` CLI

The `configgen` CLI runs the same generation pipeline outside Kubernetes.

```bash
configgen <ogxserver.yaml> -base <config.yaml> [-distributions-path distributions.json] [-output-config] [-validate]
```

Notes:

- If the CR uses `spec.baseConfig`, pass that file with `-base`.
- If the CR only sets `spec.distribution.name`, pass `-base` as well; image
  resolution for named distributions happens in the operator.
- `-validate` uses `distributions.json` to validate `spec.distribution.name`.

## Storage Example (Postgres)

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: pg-conn
  labels:
    ogx.io/watch: "true"
stringData:
  connection-string: "postgresql://user:pass@postgres:5432/ogx"
---
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: generated-config-with-storage
spec:
  distribution:
    name: starter
  storage:
    sql:
      type: postgres
      connectionString:
        name: pg-conn
        key: connection-string
```

## Status and Conditions

When generation is active, status includes:

- `.status.configGeneration.configMapName`
- `.status.configGeneration.generatedAt`
- `.status.configGeneration.providerCount`
- `.status.configGeneration.resourceCount`
- `.status.configGeneration.configVersion`

Condition `ConfigGenerated` indicates current generation state:

- `True` with reason `ConfigGenerationSucceeded` when generation succeeds
- `False` with reasons like `ConfigGenerationFailed` or `ConfigGenerationInactive`

## Rollout Behavior

Pod template annotations include hashes for generated config and referenced inputs.
Changing any of these triggers rollout:

- Generated config content
- Managed CA bundle (if configured)
- Referenced Secret resource versions

## Troubleshooting

- **Config not generated**
  - Check `kubectl get ogxserver <name> -o yaml` for `ConfigGenerated` condition and messages.
- **No restart after editing Secret/ConfigMap**
  - Ensure referenced object has `ogx.io/watch: "true"` and same namespace as `OGXServer`.
- **Validation errors for generated env var names**
  - Check provider IDs and custom secretRef keys for collisions after normalization.
- **Hash collision/content mismatch error**
  - Operator detected existing generated ConfigMap name with different content; update CR and re-reconcile.
