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
| Short Name | `llsd` | `ogxs` |
| Container Name | `llama-stack` | `ogx` |
| App Label | `app: llama-stack` | `app: ogx` |
| Managed-by | `llama-stack-operator` | `ogx-operator` |
| Operator Namespace | `llama-stack-k8s-operator-system` | `ogx-k8s-operator-system` |
| Watch Label | `llamastack.io/watch: "true"` | `ogx.io/watch: "true"` |
| Mount Path | `/.llama` | `/.ogx` |
| Leader Election ID | `81d5736e.llamastack.io` | `54e06e98.ogx.io` |

### Status Field Changes

| Old Path | New Path |
|----------|----------|
| `.status.version.llamaStackServerVersion` | `.status.version.serverVersion` |
| `.status.routeURL` | `.status.externalURL` |

### CLI Commands

| Old | New |
|-----|-----|
| `kubectl get llsd` | `kubectl get ogxs` |
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

The operand Deployments, CRD, and CRs remain after operator removal.

### Step 2: Scale Down and Clean Up Orphaned Stateless Resources

Scale the old deployment to zero (preserving it for rollback) and delete stateless resources that the new operator cannot adopt:

```bash
kubectl scale deployment <name> --replicas=0 -n <namespace>
kubectl delete networkpolicy,sa,rolebinding -l app.kubernetes.io/instance=<name>
kubectl delete hpa,pdb -l app.kubernetes.io/instance=<name>
```

**Keep for now:** The old Deployment (scaled to 0), PVC, Service, Ingress, and the old LlamaStackDistribution CR all remain in place. This makes rollback straightforward — see the [Rollback](#rollback) section.

**Why clean up stateless resources?** After operator removal, orphaned resources have no active controller. The new operator's `patchResource` safety check skips resources it does not own, so it cannot update or replace them. NetworkPolicy is especially critical: the old policy's `podSelector` targets `app: llama-stack` which no longer matches the new `app: ogx` pods.

### Step 3: Install the New OGX Operator

**Via meta-operator:**
```bash
dsc.spec.components.ogx = "Managed"
```

**Manual:**
```bash
kubectl apply -f release/operator.yaml
```

### Step 4: Define OGXServer CR

Translate fields from the old LLSD CR into the new OGXServer spec
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

The operator adopts the orphaned PVC (strips old ownerRef, labels it for discovery) and creates a new Deployment that mounts the adopted PVC. The PVC intentionally has **no** ownerReference to the OGXServer — it survives CR deletion and must be cleaned up manually. Expect ~30-60s until the new pod is ready.

> **Warning:** If multiple PVCs with the `ogx.io/adopted-from` label are found for the same instance, the controller sets an `AdoptionConfigInvalid` condition and stops reconciling (terminal error — no requeue). Remove the label from all but one PVC to resolve.

### Step 5 (Optional): Adopt Networking

Add the networking adoption annotation to preserve ClusterIP / external endpoint:

```yaml
metadata:
  annotations:
    ogx.io/adopt-storage: "<old-llsd-name>"
    ogx.io/adopt-networking: "<old-llsd-name>"
```

The operator adopts the orphaned Service + Ingress, replaces Service selectors with new pod labels (`app: ogx`, `app.kubernetes.io/instance: <name>`), and sets ownerReferences.

> **Note:** The OGXServer name **must differ** from the old LLSD name. Same-name adoption is rejected at admission time (webhook validation) to prevent resource naming conflicts.

### Step 6: Verify and Clean Up Legacy Resources

Once the new OGXServer is `Ready` and verified (see [Verification](#verification)), clean up legacy resources:

```bash
# Delete the old LlamaStackDistribution CR (orphan dependents since we already cleaned them up)
kubectl delete llamastackdistribution <old-llsd-name> -n <namespace> --cascade=orphan

# Delete the old deployment (was scaled to 0 in Step 2)
kubectl delete deployment <old-llsd-name> -n <namespace>

# Delete the legacy CRD (safe only after all LlamaStackDistribution CRs are removed)
kubectl delete crd llamastackdistributions.llamastack.io
```

If you chose **not** to adopt the old PVC (Step 4), delete it now:

```bash
kubectl delete pvc <old-llsd-name>-pvc -n <namespace>
```

If you chose **not** to adopt the old Service and Ingress/Route (Step 5), delete them now:

```bash
kubectl delete svc <old-llsd-name>-service -n <namespace>
kubectl delete ingress <old-llsd-name> -n <namespace> --ignore-not-found
kubectl delete route <old-llsd-name> -n <namespace> --ignore-not-found
```

## Verification

```bash
# Check the new CRD is registered
kubectl get crd ogxservers.ogx.io

# List OGXServer resources
kubectl get ogxs

# Check conditions for adoption status
kubectl get ogxs my-server -o jsonpath='{.status.conditions}'

# Verify the server is ready
kubectl get ogxs my-server -o jsonpath='{.status.phase}'
```

## Rollback

If you need to roll back before completing Step 6 (legacy cleanup):

1. Delete the OGXServer CR: `kubectl delete ogxserver my-server -n <namespace>`
2. Scale the old Deployment back up: `kubectl scale deployment <old-llsd-name> --replicas=1 -n <namespace>`
3. Reinstall the old LLS operator
4. The old LlamaStackDistribution CR is still present and will be reconciled by the reinstalled operator

If you already completed Step 6 (legacy resources deleted), you must recreate the old LlamaStackDistribution CR manually after reinstalling the old operator.

## Adoption Annotations

The following annotations are set by the user on the OGXServer CR to trigger adoption:

| Annotation | Purpose |
|------------|---------|
| `ogx.io/adopt-storage` | Triggers PVC adoption from the named legacy LLSD |
| `ogx.io/adopt-networking` | Triggers Service/Ingress adoption from the named legacy LLSD |

These annotations are transitional and will be removed in a future release once migration is complete.

**Constraints:**
- The annotation value **must not** equal the OGXServer CR name (same-name adoption is rejected by the validating webhook).
- The annotation value must be a valid DNS subdomain name (lowercase alphanumeric, `-`, max 253 chars).

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
