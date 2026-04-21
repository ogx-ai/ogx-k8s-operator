# llama-stack-operator
This repo hosts a kubernetes operator that is responsible for creating and managing [llama-stack](https://github.com/meta-llama/llama-stack) server.


## Features

- Automated deployment of Llama Stack servers
- Support for multiple [distributions](https://github.com/meta-llama/llama-stack?tab=readme-ov-file#distributions) (includes Ollama, vLLM, and others)
- Customizable server configurations
- Volume management for model storage
- Kubernetes-native resource management

## Table of Contents

- [Quick Start](#quick-start)
    - [Installation](#installation)
    - [Deploying Llama Stack Server](#deploying-the-llama-stack-server)
- [Enabling Network Policies](#enabling-network-policies)
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
kubectl apply -f https://raw.githubusercontent.com/llamastack/llama-stack-k8s-operator/main/release/operator.yaml
```

To install a specific released version (e.g., v1.0.0), replace `main` with the desired tag:

```bash
kubectl apply -f https://raw.githubusercontent.com/llamastack/llama-stack-k8s-operator/v1.0.0/release/operator.yaml
```

### Deploying the Llama Stack Server

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

2. Create LlamaStackDistribution CR to get the server running. Example:
```
apiVersion: llamastack.io/v1alpha1
kind: LlamaStackDistribution
metadata:
  name: llamastackdistribution-sample
spec:
  replicas: 1
  server:
    distribution:
      name: starter
    containerSpec:
      env:
      - name: OLLAMA_INFERENCE_MODEL
        value: "llama3.2:1b"
      - name: OLLAMA_URL
        value: "http://ollama-server-service.ollama-dist.svc.cluster.local:11434"
    storage:
      size: "20Gi"
      mountPath: "/home/lls/.lls"
```
3. Verify the server pod is running in the user defined namespace.

### Local Vector Storage (inline::milvus)

To enable the `inline::milvus` local vector storage provider, set `ENABLE_INLINE_MILVUS` in `spec.server.containerSpec.env`. This is only supported in single-worker, single-replica deployments. Milvus-Lite uses SQLite internally and does not support concurrent access from multiple processes.

### Using a ConfigMap for config.yaml configuration

A ConfigMap can be used to store config.yaml configuration for each LlamaStackDistribution.
Updates to the ConfigMap will restart the Pod to load the new data.

Example to create a config.yaml ConfigMap, and a LlamaStackDistribution that references it:
```
kubectl apply -f config/samples/example-with-configmap.yaml
```

## Enabling Network Policies

The operator can create an ingress-only `NetworkPolicy` for each `LlamaStackDistribution`. By default, traffic is limited to:
- All pods within the same namespace
- The operator namespace (`llama-stack-k8s-operator-system`)

### Enable the Feature Flag

Network policies are disabled by default. Enable via ConfigMap:

```bash
kubectl apply -f - <<EOF
apiVersion: v1
kind: ConfigMap
metadata:
  name: llama-stack-operator-config
  namespace: llama-stack-k8s-operator-system
data:
  featureFlags: |
    enableNetworkPolicy:
      enabled: true
EOF
```

### Configure Per-Instance Access

Use `spec.network` to customize access controls:

```yaml
apiVersion: llamastack.io/v1alpha1
kind: LlamaStackDistribution
metadata:
  name: my-llsd
spec:
  server:
    distribution:
      name: starter
  network:
    exposeRoute: false          # Set true to create an Ingress for external access
    allowedFrom:
      namespaces:               # Explicit namespace names
        - my-app-namespace
        - monitoring
      labels:                   # Namespaces matching these label keys
        - team=frontend
```

| Field | Description |
|-------|-------------|
| `network.exposeRoute` | When `true`, creates an Ingress for external access (default: `false`) |
| `network.allowedFrom.namespaces` | List of namespace names allowed to access the service. Use `"*"` to allow all namespaces |
| `network.allowedFrom.labels` | List of namespace label keys. Namespaces with these labels are allowed |

Set `enabled: false` in the ConfigMap to disable; the operator will delete the managed policies.

## Image Mapping Overrides

The operator supports ConfigMap-driven image updates for LLS Distribution images. This allows independent patching for security fixes or bug fixes without requiring a new operator version.

### Configuration

Create or update the operator ConfigMap with an `image-overrides` key:

```yaml

  image-overrides: |
    starter-gpu: quay.io/custom/llama-stack:starter-gpu
    starter: quay.io/custom/llama-stack:starter
```

### Configuration Format

Use the distribution name directly as the key (e.g., `starter-gpu`, `starter`). The operator will apply these overrides automatically

### Example Usage

To update the LLS Distribution image for all `starter` distributions:

```bash
kubectl patch configmap llama-stack-operator-config -n llama-stack-k8s-operator-system --type merge -p '{"data":{"image-overrides":"starter: quay.io/opendatahub/llama-stack:latest"}}'
```

This will cause all LlamaStackDistribution resources using the `starter` distribution to restart with the new image.

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
  make image IMG=quay.io/<username>/llama-stack-k8s-operator:<custom-tag>
  ```

  The default image used is `quay.io/llamastack/llama-stack-k8s-operator:latest` when not supply argument for `make image`
  To create a local file `local.mk` with env variables can overwrite the default values set in the `Makefile`.

- Building multi-architecture images (ARM64, AMD64, etc.)

  The operator supports building for multiple architectures including ARM64. To build and push multi-arch images:

  ```commandline
  make image-buildx IMG=quay.io/<username>/llama-stack-k8s-operator:<custom-tag>
  ```

  By default, this builds for `linux/amd64,linux/arm64`. You can customize the platforms by setting the `PLATFORMS` variable:

  ```commandline
  # Build for specific platforms
  make image-buildx IMG=quay.io/<username>/llama-stack-k8s-operator:<custom-tag> PLATFORMS=linux/amd64,linux/arm64

  # Add more architectures (e.g., for future support)
  make image-buildx IMG=quay.io/<username>/llama-stack-k8s-operator:<custom-tag> PLATFORMS=linux/amd64,linux/arm64,linux/s390x,linux/ppc64le
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
  make image-build-push-single PLATFORM=linux/amd64 IMG=quay.io/<username>/llama-stack-k8s-operator:<tag>-amd64

  # Create a multi-arch manifest from per-arch images (used by the final manifest job)
  make image-create-manifest IMG=quay.io/<username>/llama-stack-k8s-operator:<tag> \
    ARCH_IMGS="quay.io/<username>/llama-stack-k8s-operator:<tag>-amd64 quay.io/<username>/llama-stack-k8s-operator:<tag>-arm64"
  ```

- Building ARM64-only images

  To build a single ARM64 image (useful for testing or ARM-native systems):

  ```commandline
  make image-build-arm IMG=quay.io/<username>/llama-stack-k8s-operator:<custom-tag>
  make image-push IMG=quay.io/<username>/llama-stack-k8s-operator:<custom-tag>
  ```

  This works with both Docker and Podman.

- Once the image is created, the operator can be deployed directly. For each deployment method a
  kubeconfig should be exported

  ```commandline
  export KUBECONFIG=<path to kubeconfig>
  ```

### Deployment

**Deploying operator locally**

- Deploy the created image in your cluster using following command:

  ```commandline
  make deploy IMG=quay.io/<username>/llama-stack-k8s-operator:<custom-tag>
  ```

- To remove resources created during installation use:

  ```commandline
  make undeploy
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
