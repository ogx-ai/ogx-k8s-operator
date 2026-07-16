# ogx-k8s-operator
This repo hosts a Kubernetes operator that creates and manages OGX (Open GenAI Stack) servers.


## Features

- Automated deployment of OGX servers
- Support for multiple distributions (includes Ollama, vLLM, and others)
- Declarative runtime config generation from OGXServer CR fields
- Customizable server configurations
- Volume management for model storage
- Kubernetes-native resource management

## Table of Contents

- [Quick Start](#quick-start)
    - [Installation](#installation)
    - [Deploying the OGX Server](#deploying-the-ogx-server)
    - [Runtime Config via CR](#runtime-config-via-cr)
- [Enabling Network Policies](#enabling-network-policies)
- [Monitoring](#monitoring)
- [Developer Guide](#developer-guide)
    - [Prerequisites](#prerequisites)
    - [Building the Operator](#building-the-operator)
    - [Deployment](#deployment)
- [Running E2E Tests](#running-e2e-tests)
- [API Overview](#api-overview)

## Quick Start

### Installation

You can install the operator directly from a released version or the latest main branch using `kubectl apply -f`.

To install the latest version from the main branch:

```bash
kubectl apply -f https://raw.githubusercontent.com/ogx-ai/ogx-k8s-operator/main/release/operator.yaml
```

To install a specific released version (e.g., v1.0.0), replace `main` with the desired tag:

```bash
kubectl apply -f https://raw.githubusercontent.com/ogx-ai/ogx-k8s-operator/v1.0.0/release/operator.yaml
```

### Deploying the OGX Server

1. Deploy the inference provider server (ollama, vllm)

**Ollama Examples:**

Deploy Ollama with default model llama3.2:1b
```bash
./hack/deploy-quickstart.sh
```

Deploy Ollama with other model:
```bash
./hack/deploy-quickstart.sh --provider ollama --model llama3.2:7b
```

**vLLM Examples:**

This would require a secret "hf-token-secret" in namespace "vllm-dist" for HuggingFace token (required for downloading models) to be created in advance.

Deploy vLLM with default model (meta-llama/Llama-3.2-1B):
```bash
./hack/deploy-quickstart.sh --provider vllm
```

Deploy vLLM with GPU support:
```bash
./hack/deploy-quickstart.sh --provider vllm --runtime-env "VLLM_TARGET_DEVICE=gpu,CUDA_VISIBLE_DEVICES=0"
```

2. Create an OGXServer CR to get the server running. Example:
```
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: ogxserver-sample
spec:
  distribution:
    name: starter
  workload:
    replicas: 1
    storage:
      size: "20Gi"
      mountPath: "/.ogx"
    overrides:
      env:
      - name: OLLAMA_INFERENCE_MODEL
        value: "llama3.2:1b"
      - name: OLLAMA_URL
        value: "http://ollama-server-service.ollama-dist.svc.cluster.local:11434"
```
3. Verify the server pod is running in the user defined namespace.

### Local Vector Storage (inline::milvus)

To enable the `inline::milvus` local vector storage provider, set `ENABLE_INLINE_MILVUS` in `spec.workload.overrides.env`. This is only supported in single-worker, single-replica deployments. Milvus-Lite uses SQLite internally and does not support concurrent access from multiple processes.

### Runtime Config via CR

The operator supports two ways to provide OGX `config.yaml`:

1. **Declarative generation from CR fields** (recommended) via:
   - `spec.baseConfig` (optional base config input)
   - `spec.providers`
   - `spec.resources`
   - `spec.storage`
   - `spec.disabledAPIs`
2. **Direct override** via `spec.overrideConfig` pointing to a user-managed ConfigMap.

When declarative fields are present and `spec.overrideConfig` is not set, the operator:

- Resolves base config from `spec.baseConfig` when set, otherwise from OCI labels `com.ogx.distribution.default-config` + `com.ogx.config.<filename>`
- Generates a final `config.yaml`
- Creates immutable ConfigMap `${name}-config-${hash}`
- Mounts that config to `/etc/ogx/config.yaml`
- Injects required secret-based env vars from provider/storage secret refs
- Rolls the Deployment when referenced config/secret inputs change

The mounted runtime config always comes from either `spec.overrideConfig` or the
generated ConfigMap. `spec.baseConfig` is only used as an input to generation
and is never mounted into the pod directly.

Example declarative OGXServer:

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: runtime-config-sample
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
  storage:
    sql:
      type: postgres
      connectionString:
        name: db-credentials
        key: connection-string
```

Ready-to-apply sample:

```bash
kubectl apply -f config/samples/example-with-generated-config.yaml
```

Required labels for referenced resources (same namespace as OGXServer):

```yaml
metadata:
  labels:
    ogx.io/watch: "true"
```

See [Runtime Config Generation Guide](docs/additional/runtime-config-generation.md) for detailed flow, examples, and troubleshooting.

### Using a ConfigMap for config.yaml override

A ConfigMap can be used to store config.yaml configuration for each OGXServer.
Updates to the ConfigMap will restart the Pod to load the new data.

Example to create a config.yaml ConfigMap, and an OGXServer that references it:
```
kubectl apply -f config/samples/example-with-configmap.yaml
```

`spec.overrideConfig` always takes precedence over declarative generation fields.

## Enabling Network Policies

Network policies are enabled by default per-CR. Configure via `spec.network.policy`:

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-ogxserver
spec:
  distribution:
    name: starter
  network:
    externalAccess:
      enabled: true
      hostname: my-ogx.example.com
    policy:
      enabled: true
      ingress:
        - from:
            - namespaceSelector:
                matchLabels:
                  kubernetes.io/metadata.name: my-app-namespace
          ports:
            - protocol: TCP
              port: 8321
```

| Field | Description |
|-------|-------------|
| `network.externalAccess.enabled` | When `true`, enables external access configuration for the server |
| `network.externalAccess.hostname` | Hostname used for external access (for example, Ingress host) |
| `network.policy.enabled` | When `true`, the operator creates a `NetworkPolicy` for the OGXServer workload |
| `network.policy.ingress` | Ingress rules for the policy (for example, allowed sources and ports) |

## Monitoring

The operator provides built-in Prometheus monitoring for OGXServer instances. Monitoring is **enabled by default** and requires no configuration when the prometheus-operator CRDs are installed on the cluster.

When enabled, the operator creates:
- A **ServiceMonitor** with label `monitoring.opendatahub.io/scrape: "true"` for ODH/RHOAI Prometheus scraping
- A **PrometheusRule** with telemetry recording rules for Red Hat Insights

Configure monitoring via `spec.monitoring`:

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-ogxserver
spec:
  distribution:
    name: starter
  monitoring:
    enabled: true        # default: true
    metricsPort: 9090    # default: 9464
```

| Field | Description |
|-------|-------------|
| `monitoring.enabled` | When `true` (default), the operator creates a ServiceMonitor and PrometheusRule |
| `monitoring.metricsPort` | Port for the `/metrics` endpoint (default: 9464) |

If the prometheus-operator CRDs are not installed on the cluster, monitoring resources are silently skipped.

Ready-to-apply sample:

```bash
kubectl apply -f config/samples/example-with-monitoring.yaml
```

See [Monitoring Integration Guide](docs/additional/monitoring-integration.md) for detailed architecture, pipelines, and troubleshooting.

## Image Mapping Overrides

The operator supports ConfigMap-driven image updates for OGX distribution images. This allows independent patching for security fixes or bug fixes without requiring a new operator version.

### Configuration

Create or update the operator ConfigMap with an `image-overrides` key:

```yaml
image-overrides: |
  starter-gpu: quay.io/custom/ogx:starter-gpu
  starter: quay.io/custom/ogx:starter
```

### Configuration Format

Use the distribution name directly as the key (e.g., `starter-gpu`, `starter`). The operator will apply these overrides automatically

### Example Usage

To update the OGX distribution image for all `starter` distributions:

```bash
kubectl patch configmap ogx-operator-config -n ogx-k8s-operator-system --type merge -p '{"data":{"image-overrides":"starter: quay.io/ogx-ai/ogx-server:latest"}}'
```

This will cause all OGXServer resources using the `starter` distribution to restart with the new image.

## Developer Guide

### Prerequisites

- Kubernetes cluster (v1.20 or later)
- Go version **go1.24**
- operator-sdk **v1.39.2** (v4 layout) or newer
- kubectl configured to access your cluster
- A running inference server:
  - For local development, you can use the provided script: `/hack/deploy-quickstart.sh`

### Building the Operator

- Prepare release files with specific versions

  ```commandline
  make release VERSION=0.2.1 LLAMASTACK_VERSION=0.2.12
  ```

  This command updates distribution configurations and generates release manifests with the specified versions.

- Custom operator image can be built using your local repository

  ```commandline
  make image IMG=quay.io/<username>/ogx-k8s-operator:<custom-tag>
  ```

  The default image used is `quay.io/ogx-ai/ogx-k8s-operator:latest` when not supply argument for `make image`
  To create a local file `local.mk` with env variables can overwrite the default values set in the `Makefile`.

- Building multi-architecture images (ARM64, AMD64, etc.)

  The operator supports building for multiple architectures including ARM64. To build and push multi-arch images:

  ```commandline
  make image-buildx IMG=quay.io/<username>/ogx-k8s-operator:<custom-tag>
  ```

  By default, this builds for `linux/amd64,linux/arm64`. You can customize the platforms by setting the `PLATFORMS` variable:

  ```commandline
  # Build for specific platforms
  make image-buildx IMG=quay.io/<username>/ogx-k8s-operator:<custom-tag> PLATFORMS=linux/amd64,linux/arm64

  # Add more architectures (e.g., for future support)
  make image-buildx IMG=quay.io/<username>/ogx-k8s-operator:<custom-tag> PLATFORMS=linux/amd64,linux/arm64,linux/s390x,linux/ppc64le
  ```

  **Note**:
  - The `image-buildx` target works with both Docker and Podman. It will automatically detect which tool is being used.
  - **Native builds in CI**: CI workflows use a matrix strategy with native runners for each architecture (AMD64 and ARM64). Each architecture is built on its own runner, avoiding QEMU emulation entirely. Per-architecture images are pushed separately, then combined into a single multi-arch manifest list. This ensures `CGO_ENABLED=1` with full OpenSSL FIPS support for all architectures.
  - **Local cross-compilation**: For local development, the Dockerfile uses `--platform=$BUILDPLATFORM` to run Go compilation natively on the build host. When cross-compiling (e.g., building ARM64 on an AMD64 host), `CGO_ENABLED=0` is used with pure Go FIPS (via `GOEXPERIMENT=strictfipsruntime`). Native local builds use `CGO_ENABLED=1` with full OpenSSL FIPS support.
  - **FIPS adherence**: All CI-produced images use `CGO_ENABLED=1` with full OpenSSL FIPS support via native builds on architecture-matched runners.
  - For Docker: Multi-arch builds require Docker Buildx. Ensure Docker Buildx is set up:

    ```commandline
    docker buildx create --name x-builder --use
    ```

  - For Podman: Podman 4.0+ supports `podman buildx` (experimental). If buildx is unavailable, the Makefile will automatically fall back to using podman's native manifest-based multi-arch build approach.
  - The resulting images are multi-arch manifest lists, which means Kubernetes will automatically select the correct architecture when pulling the image.

  **CI Build Targets**:

  The CI workflows use the following Makefile targets for the matrix-based build strategy:

  ```commandline
  # Build and push a single-arch image (used by each matrix job on its native runner)
  make image-build-push-single PLATFORM=linux/amd64 IMG=quay.io/<username>/ogx-k8s-operator:<tag>-amd64

  # Create a multi-arch manifest from per-arch images (used by the final manifest job)
  make image-create-manifest IMG=quay.io/<username>/ogx-k8s-operator:<tag> \
    ARCH_IMGS="quay.io/<username>/ogx-k8s-operator:<tag>-amd64 quay.io/<username>/ogx-k8s-operator:<tag>-arm64"
  ```

- Building ARM64-only images

  To build a single ARM64 image (useful for testing or ARM-native systems):

  ```commandline
  make image-build-arm IMG=quay.io/<username>/ogx-k8s-operator:<custom-tag>
  make image-push IMG=quay.io/<username>/ogx-k8s-operator:<custom-tag>
  ```

  This works with both Docker and Podman.

- Once the image is created, the operator can be deployed directly. For each deployment method a
  kubeconfig should be exported

  ```commandline
  export KUBECONFIG=<path to kubeconfig>
  ```

### Deployment

**Deploying on vanilla Kubernetes (cert-manager)**

- Deploy the created image in your cluster using following command:

  ```commandline
  make deploy IMG=quay.io/<username>/ogx-k8s-operator:<custom-tag>
  ```

- To remove resources created during installation use:

  ```commandline
  make undeploy
  ```

**Deploying on OpenShift**

OpenShift clusters use the built-in service-serving-cert-signer for webhook TLS
(no cert-manager required):

  ```commandline
  make deploy-openshift IMG=quay.io/<username>/ogx-k8s-operator:<custom-tag>
  ```

- To remove resources:

  ```commandline
  make undeploy-openshift
  ```

## Running E2E Tests

The operator includes end-to-end (E2E) tests to verify the complete functionality of the operator. To run the E2E tests:

1. Ensure you have a running Kubernetes cluster
2. Run the E2E tests using one of the following commands:
   - If you want to deploy the operator and run tests:
     ```commandline
     make deploy test-e2e
     ```
   - If the operator is already deployed:
     ```commandline
     make test-e2e
     ```

The make target will handle prerequisites including deploying ollama server.

## API Overview

Please refer to [api documentation](docs/api-overview.md)
