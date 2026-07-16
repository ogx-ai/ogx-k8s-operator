# Monitoring Integration for OGXServer

This document explains how the OGX operator integrates with Prometheus monitoring to provide observability for OGXServer instances.

## Overview

The operator provides built-in Prometheus monitoring through two pipelines:

- **Pipeline 1 — ODH local observability**: A ServiceMonitor enables Prometheus to scrape raw metrics from OGXServer pods.
- **Pipeline 2 — CMO/Telemeter (Red Hat Insights)**: A PrometheusRule creates recording rules that produce binary adoption signals for fleet-wide telemetry.

Monitoring is **enabled by default**. The operator creates both resources automatically when the prometheus-operator CRDs (`monitoring.coreos.com`) are installed on the cluster. No explicit configuration is required.

## How It Works

### Pipeline 1: ODH Local Observability (ServiceMonitor)

When monitoring is enabled, the operator creates a ServiceMonitor resource for each OGXServer instance.

1. The operator creates a ServiceMonitor named `<instance-name>-service-monitor` in the same namespace as the OGXServer.
2. The ServiceMonitor carries the label `monitoring.opendatahub.io/scrape: "true"`, which causes ODH/RHOAI's Prometheus stack to automatically discover and scrape it.
3. The ServiceMonitor targets the `metrics` port on the Service, scraping the `/metrics` endpoint every 60 seconds.
4. The Service's `metrics` port maps to the container's metrics port (default 9464, or the value of `spec.monitoring.metricsPort`).

This pipeline provides per-namespace observability of raw OGX server metrics such as `ogx_requests_total`, `ogx_vector_io_documents_retrieved_total`, and `ogx_responses_agentic_calls_total`.

### Pipeline 2: CMO/Telemeter Recording Rules (PrometheusRule)

The operator also creates a PrometheusRule resource containing recording rules that feed into Red Hat's Telemeter pipeline for Red Hat Insights.

1. The operator creates a PrometheusRule named `<instance-name>-prometheus-rules` in the same namespace as the OGXServer.
2. The PrometheusRule contains a `telemetry.rules` group evaluated every 60 seconds.
3. Recording rules create binary (0 or 1) `ogx:api_info:max` metrics for each API surface:

| Recording Rule | Label | Source Metric | Meaning |
|---|---|---|---|
| `ogx:api_info:max` | `api=inference` | `ogx_requests_total{api="inference"}` | Inference API is in use |
| `ogx:api_info:max` | `api=vector_io` | `ogx_requests_total{api="vector_io"}` | Vector IO API is in use |
| `ogx:api_info:max` | `api=responses` | `ogx_requests_total{api="responses"}` | Responses API is in use |
| `ogx:api_info:max` | `api=rag` | `ogx_vector_io_documents_retrieved_total` | RAG (document retrieval) is in use |
| `ogx:api_info:max` | `api=agentic` | `ogx_responses_agentic_calls_total` | Agentic calls are in use |

Each rule uses `clamp_max(sum(...) or vector(0), 1)` to produce a binary "feature in use" signal. These lightweight signals are consumed by the Cluster Monitoring Operator (CMO) and federated to Red Hat's Telemeter service for fleet-wide adoption tracking.

### CRD Availability Detection

The operator gracefully handles clusters without the prometheus-operator CRDs:

1. Before creating monitoring resources, the operator checks whether both `servicemonitors.monitoring.coreos.com` and `prometheusrules.monitoring.coreos.com` CRDs are registered on the cluster.
2. If either CRD is missing, both ServiceMonitor and PrometheusRule are silently excluded from rendering. The operator logs this and continues without error.
3. This check runs at reconciliation time (not startup), so the operator adapts if CRDs are installed or removed after the operator starts.

### Metrics Port Logic

The metrics endpoint is configured automatically when monitoring is enabled:

- **Default metrics port**: 9464
- **Custom port**: Set via `spec.monitoring.metricsPort`
- **NetworkPolicy**: When NetworkPolicy is enabled, an additional ingress rule allows Prometheus scraping from OpenShift monitoring namespaces (`network.openshift.io/policy-group: monitoring`).

When monitoring is disabled (`spec.monitoring.enabled: false`), no metrics port, environment variables, ServiceMonitor, or PrometheusRule are created.

## Configuration

### Default (Monitoring Enabled, Default Port)

Monitoring is enabled by default with no configuration required:

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-ogxserver
spec:
  distribution:
    name: starter
```

### Custom Metrics Port

To serve metrics on a specific port:

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-ogxserver
spec:
  distribution:
    name: starter
  monitoring:
    enabled: true
    metricsPort: 9090
```

### Monitoring Disabled

To disable monitoring entirely:

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-ogxserver
spec:
  distribution:
    name: starter
  monitoring:
    enabled: false
```

When monitoring is disabled after having been enabled, the operator cleans up the existing ServiceMonitor and PrometheusRule resources.

### Configuration Fields

| Field | Type | Default | Description |
|---|---|---|---|
| `spec.monitoring.enabled` | boolean | `true` | Controls whether the operator creates monitoring resources (ServiceMonitor, PrometheusRule) for this server. Set to `false` to disable. |
| `spec.monitoring.metricsPort` | integer (1–65535) | `9464` | The port serving the `/metrics` endpoint. When omitted, the default port 9464 is used. |

Ready-to-apply samples:

```bash
kubectl apply -f config/samples/example-with-monitoring.yaml
```

## Troubleshooting

### prometheus-operator CRDs Not Installed

**Symptom**: No ServiceMonitor or PrometheusRule resources are created, even though monitoring is enabled (or not explicitly disabled).

**Cause**: The prometheus-operator CRDs are not registered on the cluster.

**Solution**:

```bash
# Verify CRDs are installed
kubectl get crd servicemonitors.monitoring.coreos.com
kubectl get crd prometheusrules.monitoring.coreos.com
```

If the CRDs are missing, install the prometheus-operator or ensure the cluster's monitoring stack includes it. On OpenShift, the Cluster Monitoring Operator (CMO) provides these CRDs by default.

### Verifying the ServiceMonitor Is Being Scraped

```bash
# Check the ServiceMonitor exists
kubectl get servicemonitor -n <namespace> <instance-name>-service-monitor

# Verify the ODH scrape label is present
kubectl get servicemonitor -n <namespace> <instance-name>-service-monitor \
  -o jsonpath='{.metadata.labels.monitoring\.opendatahub\.io/scrape}'
# Expected output: true

# Verify the metrics endpoint is reachable
kubectl port-forward svc/<instance-name>-service -n <namespace> 9464:9464
curl http://localhost:9464/metrics
```

### Verifying Recording Rule Output

```bash
# Check the PrometheusRule exists
kubectl get prometheusrule -n <namespace> <instance-name>-prometheus-rules

# View the recording rules
kubectl get prometheusrule -n <namespace> <instance-name>-prometheus-rules \
  -o jsonpath='{.spec.groups[0].rules[*].record}'

# Query Prometheus for the recording rule output (requires access to the Prometheus API)
curl -g 'http://<prometheus-host>:9090/api/v1/query?query=ogx:api_info:max'
```

### Monitoring Was Enabled but Resources Are Missing

1. **Check the operator logs** for messages about CRD availability:
   ```bash
   kubectl logs -n ogx-k8s-operator-system deployment/ogx-k8s-operator-controller-manager | grep -i monitoring
   ```
2. **Verify CRDs are installed** (see above).
3. **Check the OGXServer CR** does not have `spec.monitoring.enabled: false`:
   ```bash
   kubectl get ogxserver <name> -n <namespace> -o jsonpath='{.spec.monitoring}'
   ```

### Metrics Endpoint Not Responding

```bash
# Check the container's environment variables
kubectl exec -n <namespace> <pod-name> -- env | grep OGX_METRICS

# Check the container ports
kubectl get pod -n <namespace> <pod-name> \
  -o jsonpath='{.spec.containers[0].ports[*].name}'
# Expected: should include "metrics"

# Check the Service ports
kubectl get svc -n <namespace> <instance-name>-service \
  -o jsonpath='{.spec.ports}'
# Expected: should include a port named "metrics"
```

### Cleanup Behavior When Disabling Monitoring

When monitoring is disabled (setting `spec.monitoring.enabled: false`), the operator actively deletes existing ServiceMonitor and PrometheusRule resources owned by the OGXServer instance. Only resources with an ownerReference pointing to the current OGXServer are removed — resources belonging to other instances are not affected.
