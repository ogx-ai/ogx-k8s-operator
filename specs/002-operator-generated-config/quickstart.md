# Quickstart: LlamaStackDistribution v1alpha2

This guide walks through common v1alpha2 configuration patterns, from minimal to production-grade.

## Prerequisites

- Kubernetes cluster (1.30+)
- llama-stack-k8s-operator installed (version supporting v1alpha2)
- `kubectl` configured to access your cluster

## Example 1: Minimal Inference Setup

Deploy a LlamaStack instance with a single vLLM provider using just a few lines:

```yaml
apiVersion: llamastack.io/v1alpha2
kind: LlamaStackDistribution
metadata:
  name: my-stack
spec:
  distribution:
    name: starter
  providers:
    inference:
      - provider: vllm
        endpoint: "http://vllm-service:8000"
```

Apply and verify:

```bash
kubectl apply -f simple.yaml
kubectl get llsd my-stack
kubectl wait --for=condition=Available llsd/my-stack --timeout=120s
```

The operator:
1. Resolves `starter` to the distribution image
2. Loads the embedded base config for `starter`
3. Merges your inference provider over the base config
4. Generates a ConfigMap with the final config.yaml
5. Creates a Deployment with the resolved image

## Example 2: Inference with API Key Authentication

Use a Kubernetes Secret for provider authentication:

```bash
# Create the secret first
kubectl create secret generic vllm-creds --from-literal=token=sk-your-api-key
```

```yaml
apiVersion: llamastack.io/v1alpha2
kind: LlamaStackDistribution
metadata:
  name: my-stack
spec:
  distribution:
    name: remote-vllm
  providers:
    inference:
      - provider: vllm
        endpoint: "https://vllm.example.com"
        apiKey:
          secretKeyRef:
            name: vllm-creds
            key: token
```

The operator resolves the secret reference to an environment variable (`LLSD_VLLM_API_KEY`) and injects it into the Deployment. The secret value never appears in the ConfigMap.

## Example 3: Multiple Providers

Configure primary and fallback inference providers:

```yaml
apiVersion: llamastack.io/v1alpha2
kind: LlamaStackDistribution
metadata:
  name: ha-stack
spec:
  distribution:
    name: starter
  providers:
    inference:
      - id: vllm-primary
        provider: vllm
        endpoint: "http://vllm-primary:8000"
      - id: vllm-fallback
        provider: vllm
        endpoint: "http://vllm-fallback:8000"
```

When using list form, each provider MUST have an explicit `id`. The operator validates uniqueness across all provider types.

## Example 4: Models and Resources

Register models and tools declaratively:

```yaml
apiVersion: llamastack.io/v1alpha2
kind: LlamaStackDistribution
metadata:
  name: my-stack
spec:
  distribution:
    name: starter
  providers:
    inference:
      - id: vllm-primary
        provider: vllm
        endpoint: "http://vllm-primary:8000"
      - id: vllm-secondary
        provider: vllm
        endpoint: "http://vllm-secondary:8000"
  resources:
    models:
      - name: "llama3.2-8b"                  # Uses first inference provider
      - name: "llama3.2-70b"                 # Uses specified provider
        provider: vllm-secondary
        contextLength: 128000
    tools:
      - websearch
      - rag
    shields:
      - llama-guard
```

When `provider` is omitted, models are registered with the first inference provider. Specify `provider` to assign models to specific providers.

## Example 5: PostgreSQL State Storage

Configure persistent state storage with PostgreSQL:

```bash
# Create the connection string secret
kubectl create secret generic pg-creds \
  --from-literal=url="postgresql://user:pass@postgres:5432/llamastack"
```

```yaml
apiVersion: llamastack.io/v1alpha2
kind: LlamaStackDistribution
metadata:
  name: prod-stack
spec:
  distribution:
    name: starter
  providers:
    inference:
      - provider: vllm
        endpoint: "http://vllm:8000"
  storage:
    sql:
      type: postgres
      connectionString:
        secretKeyRef:
          name: pg-creds
          key: url
```

When `storage` is not specified, the distribution's defaults (typically SQLite) are preserved.

## Example 6: Production Configuration

A full production setup with networking, TLS, autoscaling, and resource limits:

```yaml
apiVersion: llamastack.io/v1alpha2
kind: LlamaStackDistribution
metadata:
  name: prod-stack
spec:
  distribution:
    name: starter

  providers:
    inference:
      - id: vllm-primary
        provider: vllm
        endpoint: "http://vllm-primary:8000"
        apiKey:
          secretKeyRef: {name: vllm-creds, key: token}
    safety:
      - provider: llama-guard

  resources:
    models:
      - name: "llama3.2-8b"
    shields:
      - llama-guard

  storage:
    sql:
      type: postgres
      connectionString:
        secretKeyRef: {name: pg-creds, key: url}

  disabled:
    - postTraining
    - eval

  networking:
    port: 8321
    tls:
      enabled: true
      secretName: llama-tls
    expose:
      enabled: true
      hostname: "llama.example.com"
    allowedFrom:
      namespaces: ["app-ns"]

  workload:
    replicas: 2
    workers: 4
    resources:
      requests: {cpu: "1", memory: "2Gi"}
      limits: {cpu: "4", memory: "8Gi"}
    autoscaling:
      minReplicas: 2
      maxReplicas: 10
      targetCPUUtilizationPercentage: 75
    storage:
      size: "20Gi"
      mountPath: "/.llama"
    podDisruptionBudget:
      maxUnavailable: 1
```

## Example 7: Full ConfigMap Override

For advanced use cases where the CRD schema doesn't cover your needs:

```yaml
apiVersion: llamastack.io/v1alpha2
kind: LlamaStackDistribution
metadata:
  name: custom-stack
spec:
  distribution:
    name: starter
  overrideConfig:
    configMapName: my-full-config
```

`overrideConfig` is mutually exclusive with `providers`, `resources`, `storage`, and `disabled`. The operator uses the ConfigMap contents as-is.

## Verifying Configuration

After applying a CR, check the generated configuration:

```bash
# Check CR status
kubectl get llsd my-stack -o yaml

# View generated ConfigMap
kubectl get configmap -l app=llama-stack

# Check status conditions
kubectl get llsd my-stack -o jsonpath='{.status.conditions}' | jq .

# View the generated config.yaml
kubectl get configmap my-stack-config-<hash> -o jsonpath='{.data.config\.yaml}'
```

## Updating Configuration

Modify the CR and the operator handles the rest:

```bash
# Add a new provider
kubectl patch llsd my-stack --type merge -p '
spec:
  providers:
    safety:
      - provider: llama-guard
'

# Watch the rollout
kubectl rollout status deployment/my-stack-deployment
```

The operator:
1. Regenerates config.yaml with the new provider
2. Creates a new ConfigMap (with updated hash)
3. Updates the Deployment atomically (no intermediate mismatch states)
4. Skips the update entirely if the generated config is identical to the current one

## Migration from v1alpha1

Existing v1alpha1 CRs continue to work. The conversion webhook translates between versions automatically. See `docs/migration-v1alpha1-to-v1alpha2.md` for the full migration guide.

```bash
# Your v1alpha1 CR still works
kubectl get llsd my-old-stack -o yaml

# Retrieve it as v1alpha2 to see the converted fields
kubectl get llsd.v1alpha2.llamastack.io my-old-stack -o yaml
```
