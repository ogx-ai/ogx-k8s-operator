# Migration Guide: LlamaStackDistribution to OGXServer

This guide covers migrating from the `LlamaStackDistribution` operator (`llamastack.io/v1alpha1`) to the `OGXServer` operator (`ogx.io/v1beta1`).

## Breaking Change

This is a **breaking change**. There is no coexistence period and no conversion webhooks. Users must manually create new OGXServer CRs and migrate configuration.

## Name Mapping

| Component | Old | New |
|-----------|-----|-----|
| API Group | `llamastack.io` | `ogx.io` |
| API Version | `v1alpha1` | `v1beta1` |
| Kind | `LlamaStackDistribution` | `OGXServer` |
| Plural | `llamastackdistributions` | `ogxservers` |
| Short Name | `llsd` | `ogxserver` |
| Container Name | `llama-stack` | `ogx` |
| App Label | `app: llama-stack` | `app: ogx` |
| Managed-by | `llama-stack-operator` | `ogx-operator` |
| Operator Namespace | `llama-stack-k8s-operator-system` | `ogx-k8s-operator-system` |
| Watch Label | `llamastack.io/watch: "true"` | `ogx.io/watch: "true"` |
| Mount Path | `/.llama` | `/.ogx` |
| Leader Election ID | `81d5736e.llamastack.io` | `54e06e98.ogx.io` |

### Environment Variables

| Old | New | Notes |
|-----|-----|-------|
| `LLS_PORT` | `OGX_PORT` | Container port for the server |
| `LLS_WORKERS` | `OGX_WORKERS` | Number of uvicorn worker processes |
| `LLAMA_STACK_CONFIG` | `OGX_CONFIG` | Path to the server config file |

### Status Field Changes

| Old Path | New Path |
|----------|----------|
| `.status.version.llamaStackServerVersion` | `.status.version.serverVersion` |
| `.status.routeURL` | `.status.externalURL` |

### CLI Commands

| Old | New |
|-----|-----|
| `kubectl get llsd` | `kubectl get ogxserver` |
| `kubectl get llamastackdistributions` | `kubectl get ogxservers` |

## Spec Changes

### Network Configuration

Old (`spec.network`):
```yaml
spec:
  network:
    exposeRoute: true
    allowedFrom:
      namespaces: ["my-app"]
      labels: ["team=frontend"]
```

New (`spec.network`):
```yaml
spec:
  network:
    externalAccess:
      enabled: true
    policy:
      enabled: true
      ingress:
        - from:
            - namespaceSelector:
                matchLabels:
                  kubernetes.io/metadata.name: my-app
            - namespaceSelector:
                matchLabels:
                  team: frontend
          ports:
            - protocol: TCP
              port: 8321
```

### TLS Configuration

Old (nested under `server.tlsConfig`):
```yaml
spec:
  server:
    tlsConfig:
      enabled: true
      secretName: my-tls-secret
      caBundle:
        configMapName: my-ca-bundle
```

New (server TLS under `network.tls`, outbound trust under `tls.trust`):
```yaml
spec:
  network:
    tls:
      secretName: my-tls-secret
  tls:
    trust:
      caCertificates:
        - name: my-ca-bundle
          key: ca-bundle.crt
```

### Workload Configuration

Old (flat on spec):
```yaml
spec:
  replicas: 2
  server:
    distribution:
      name: starter
    containerSpec:
      env:
        - name: MY_VAR
          value: "hello"
    storage:
      size: "20Gi"
```

New (grouped under `spec.workload`):
```yaml
spec:
  distribution:
    name: starter
  workload:
    replicas: 2
    storage:
      size: "20Gi"
    overrides:
      env:
        - name: MY_VAR
          value: "hello"
```

### Runtime Config in CR (New)

`OGXServer` adds declarative runtime config generation directly from CR fields:

- `spec.providers`
- `spec.resources`
- `spec.storage`
- `spec.disabledAPIs`

If `spec.overrideConfig` is not set, the operator generates `config.yaml` and mounts it automatically.
If `spec.overrideConfig` is set, the user-provided ConfigMap override takes precedence.

See [Runtime Config Generation Guide](additional/runtime-config-generation.md).

### NetworkPolicy

The legacy `AllowedFromSpec` and ConfigMap-based `enableNetworkPolicy` feature flag are replaced by `spec.network.policy` with native Kubernetes NetworkPolicy types:

```yaml
spec:
  network:
    policy:
      enabled: true            # Per-CR toggle (replaces ConfigMap feature flag)
      policyTypes:
        - Ingress
        - Egress
      ingress:                 # Native K8s NetworkPolicyIngressRule
        - from:
            - namespaceSelector:
                matchLabels:
                  kubernetes.io/metadata.name: my-app
          ports:
            - protocol: TCP
              port: 8321
      egress:                  # Native K8s NetworkPolicyEgressRule
        - to:
            - ipBlock:
                cidr: 10.0.0.0/8
          ports:
            - protocol: TCP
              port: 443
```

To translate `enableNetworkPolicy: false` from the old ConfigMap, set `spec.network.policy.enabled: false` on the CR.

### ConfigMap/Secret Watch Labels and Namespace Scope

All referenced ConfigMaps and Secrets must have the `ogx.io/watch: "true"` label, and must be in the same namespace as the OGXServer CR:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-config
  namespace: <ogx_server_cr_namespace>
  labels:
    ogx.io/watch: "true"
data:
  config.yaml: |
    ...
```

## Upgrade Steps

### Step 1: Remove the Old LLS Operator

**Via meta-operator:**
```bash
# Set component to Removed
dsc.spec.components.lls = "Removed"
```

**Manual:**
```bash
# Delete the operator Deployment, ServiceAccount, and RBAC — but NOT the CRD.
# Deleting the CRD cascade-deletes all CRs and their owned resources (data loss).
kubectl -n llama-stack-k8s-operator-system delete deployment llama-stack-k8s-operator-controller-manager
kubectl -n llama-stack-k8s-operator-system delete serviceaccount llama-stack-k8s-operator-controller-manager
kubectl delete clusterrolebinding llama-stack-k8s-operator-manager-rolebinding
kubectl delete clusterrole llama-stack-k8s-operator-manager-role
```

> **Warning:** Do NOT run `kubectl delete -f release/operator.yaml` — that file includes the CRD,
> and deleting a CRD cascade-deletes all its CRs and their owned resources (PVCs, Deployments, etc.).

The operand Deployments, CRD, and CRs remain after operator removal. The old workload keeps running — no downtime yet.

### Step 2: Install the New OGX Operator

**Via meta-operator:**
```bash
dsc.spec.components.ogx = "Managed"
```

**Manual:**
```bash
kubectl apply -f release/operator.yaml
```

### Step 3: Define the OGXServer CR

Translate fields from the old LLSD CR into the new OGXServer spec. See [Spec Changes](#spec-changes) above for the field mapping.

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-server
spec:
  distribution:
    name: starter
  workload:
    replicas: 1
    storage:
      size: "20Gi"
    overrides:
      env:
        - name: OLLAMA_INFERENCE_MODEL
          value: "llama3.2:1b"
        - name: OLLAMA_URL
          value: "http://ollama-server-service.ollama-dist.svc.cluster.local:11434"
```

> **Note:** The OGXServer name **must differ** from the old LLSD name. Same-name adoption is rejected by the validating webhook to prevent resource naming conflicts.

#### Adopting Existing PVC (Optional)

To preserve existing data by adopting the PVC from the old LlamaStackDistribution:

```yaml
metadata:
  annotations:
    ogx.io/adopt-storage: "<old-llsd-name>"
```

The operator strips the old ownerRef from the PVC and labels it for discovery. The adopted PVC intentionally has **no** ownerReference to the OGXServer — it survives CR deletion and must be cleaned up manually.

> **Warning:** If multiple PVCs with matching adoption labels are found for the same instance, the controller sets an `AdoptionConfigInvalid` condition and stops reconciling. Remove the conflicting label to resolve.

#### Adopting Existing Service and Ingress (Optional)

To preserve ClusterIP / external endpoints:

```yaml
metadata:
  annotations:
    ogx.io/adopt-storage: "<old-llsd-name>"
    ogx.io/adopt-networking: "<old-llsd-name>"
```

The operator adopts the orphaned Service + Ingress, replaces Service selectors with new pod labels (`app: ogx`, `app.kubernetes.io/instance: <name>`), and sets ownerReferences.

### Step 4: Apply the OGXServer CR

```bash
kubectl apply -f ogxserver.yaml
```

Expect ~30–60s until the new pod is ready.

### Step 5: Verify

```bash
# Check the new CRD is registered
kubectl get crd ogxservers.ogx.io

# List OGXServer resources
kubectl get ogxserver

# Check conditions for adoption status
kubectl get ogxserver my-server -o jsonpath='{.status.conditions}'

# Verify the server is ready
kubectl get ogxserver my-server -o jsonpath='{.status.phase}'
```

Wait until the OGXServer phase is `Ready` before proceeding to cleanup.

### Step 6: Clean Up Legacy Resources

Once the new OGXServer is verified healthy, remove legacy resources.

Delete the old LlamaStackDistribution CR. Kubernetes will cascade-delete its owned resources (Deployment, NetworkPolicy, ServiceAccount, RoleBinding, HPA, PDB):

```bash
kubectl delete llamastackdistribution <old-llsd-name> -n <namespace>
```

If you adopted the PVC or Service/Ingress (Step 3), those resources now have new ownerRefs and are **not** affected by this deletion.

If you chose **not** to adopt the old PVC, delete it manually:

```bash
kubectl delete pvc <old-llsd-name>-pvc -n <namespace>
```

If you chose **not** to adopt the old Service and Ingress/Route, delete them manually:

```bash
kubectl delete svc <old-llsd-name>-service -n <namespace>
kubectl delete ingress <old-llsd-name> -n <namespace> --ignore-not-found
kubectl delete route <old-llsd-name> -n <namespace> --ignore-not-found
```

Finally, once all LlamaStackDistribution CRs have been removed, delete the legacy CRD:

```bash
kubectl delete crd llamastackdistributions.llamastack.io
```

## Rollback

If something goes wrong before completing Step 6 (legacy cleanup), rollback is straightforward because the old resources are still in place:

1. Delete the OGXServer CR: `kubectl delete ogxserver my-server -n <namespace>`
2. Reinstall the old LLS operator
3. The old LlamaStackDistribution CR is still present and will be reconciled by the reinstalled operator

If you already completed Step 6 (legacy resources deleted), you must recreate the old LlamaStackDistribution CR manually after reinstalling the old operator.

## NetworkPolicy Impact

The label change from `app: llama-stack` to `app: ogx` affects all NetworkPolicy `podSelector` fields:

**Old operator NetworkPolicy:**
```yaml
spec:
  podSelector:
    matchLabels:
      app: llama-stack
      app.kubernetes.io/instance: my-server
```

**New operator NetworkPolicy:**
```yaml
spec:
  podSelector:
    matchLabels:
      app: ogx
      app.kubernetes.io/instance: my-server
```

Any external NetworkPolicies targeting `app: llama-stack` must be updated to `app: ogx`.

### Distribution Name Removals

The following built-in distribution names have been removed from `distributions.json`:

- `remote-vllm`
- `meta-reference-gpu`

If your `OGXServer` spec references either of these by name, you must switch to specifying the image directly via `spec.distribution.image` or use one of the remaining distributions (`starter`, `postgres-demo`).
