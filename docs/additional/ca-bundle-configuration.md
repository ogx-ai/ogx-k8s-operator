# CA Bundle Configuration for OGXServer

This document explains how to configure custom CA bundles for OGXServer to enable secure communication with external LLM providers using self-signed certificates.

## Overview

The CA bundle configuration allows you to:
- Use self-signed certificates for external LLM API connections
- Trust custom Certificate Authorities (CAs) for secure communication
- Mount CA certificates from ConfigMaps into the OGX server pods

Source ConfigMaps must live in the **same namespace** as the OGXServer and must include the label **`ogx.io/watch: "true"`** so the operator can watch them for changes.

## How It Works

When you configure a CA bundle:

1. **ConfigMap Storage**: CA certificates are stored in a Kubernetes ConfigMap (source ConfigMap) in the same namespace as the OGXServer, with `ogx.io/watch: "true"` on the ConfigMap metadata
2. **Controller Processing**: The operator controller reads and validates certificates from the source ConfigMap(s)
3. **Concatenation**: Valid certificates are concatenated into a single PEM file using Go's `encoding/pem` package
4. **Managed ConfigMap**: The operator creates a managed ConfigMap named `{instance-name}-ca-bundle` containing the concatenated bundle
5. **Volume Mounting**: The managed ConfigMap is mounted at `/etc/ssl/certs/ca-bundle/ca-bundle.crt` in the container
6. **Environment Variable**: The `SSL_CERT_FILE` environment variable is automatically set to point to the mounted certificate bundle
7. **Automatic Restarts**: Pods restart automatically when the source CA bundle ConfigMap changes

### Certificate Processing

The operator processes CA bundle certificates in the controller before deployment:

**Processing Steps:**
1. The controller reads CA certificate data from the source ConfigMap(s) specified in `spec.tls.trust.caCertificates`
2. Each certificate is validated using Go's `encoding/pem` package to ensure proper PEM format
3. Valid `CERTIFICATE` blocks are extracted and concatenated into a single PEM file
4. The concatenated bundle is stored in a managed ConfigMap named `{instance-name}-ca-bundle` with key `ca-bundle.crt`
5. The managed ConfigMap is mounted directly at `/etc/ssl/certs/ca-bundle/ca-bundle.crt` in the pod
6. The `SSL_CERT_FILE` environment variable is automatically set to point to the mounted bundle file

**Security Features:**
- Maximum bundle size limit (10MB) to prevent resource exhaustion attacks
- Maximum certificate count limit (1000 certificates)
- PEM format validation before processing
- Only valid X.509 CERTIFICATE blocks are extracted (other PEM types are ignored)
- Validation errors prevent deployment with clear error messages in the CR status
- No runtime dependencies (no openssl or other tools required in the container)

## Configuration Options

### Basic CA Bundle Configuration

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-ogx-server
spec:
  distribution:
    name: hf-serverless
  tls:
    trust:
      caCertificates:
        - name: my-ca-bundle
          key: ca-bundle.crt
```

### Multiple CA Sources (RHOAI Pattern)

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-ogx-server
spec:
  distribution:
    name: hf-serverless
  tls:
    trust:
      caCertificates:
        - name: odh-trusted-ca-bundle
          key: ca-bundle.crt        # CNO-injected cluster CAs
        - name: odh-trusted-ca-bundle
          key: odh-ca-bundle.crt    # User-specified custom CAs
```

### Configuration Fields

- `spec.tls.trust.caCertificates` (array of ConfigMapKeyRef): Each entry references a specific key in a ConfigMap containing PEM-encoded CA certificates. All certificates from all entries are concatenated into a single CA bundle file.
  - `name` (required): Name of the ConfigMap containing the CA certificate.
  - `key` (required): Key within the ConfigMap containing the PEM-encoded certificate data.

**ConfigMap requirements:** The referenced ConfigMap must include `metadata.labels["ogx.io/watch"]` set to `"true"`.

## Examples

### Example 1: Basic CA Bundle

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-ca-bundle
  labels:
    ogx.io/watch: "true"
data:
  ca-bundle.crt: |
    -----BEGIN CERTIFICATE-----
    # ... your CA certificate data here ...
    -----END CERTIFICATE-----
---
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: secure-ogx-server
spec:
  distribution:
    name: hf-serverless
  tls:
    trust:
      caCertificates:
        - name: my-ca-bundle
          key: ca-bundle.crt
```

### Example 2: Custom Key Name

Reference a non-default key name from the ConfigMap:

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: my-ca-bundle
  labels:
    ogx.io/watch: "true"
data:
  custom-ca.pem: |
    -----BEGIN CERTIFICATE-----
    # ... your CA certificate data here ...
    -----END CERTIFICATE-----
---
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: secure-ogx-server
spec:
  distribution:
    name: hf-serverless
  tls:
    trust:
      caCertificates:
        - name: my-ca-bundle
          key: custom-ca.pem
```

### Example 3: Same Namespace Only

CA bundle ConfigMaps must be in the **same namespace** as the OGXServer. There is no cross-namespace reference support; create or copy the bundle ConfigMap into the OGXServer namespace before referencing it.

### Example 4: RHOAI Pattern with Multiple CA Sources

```yaml
apiVersion: v1
kind: ConfigMap
metadata:
  name: odh-trusted-ca-bundle
  labels:
    ogx.io/watch: "true"
    config.openshift.io/inject-trusted-cabundle: "true"
data:
  ca-bundle.crt: |
    # Populated by Cluster Network Operator (CNO)
    -----BEGIN CERTIFICATE-----
    # ... cluster-wide CA certificates ...
    -----END CERTIFICATE-----
  odh-ca-bundle.crt: |
    # User-specified custom CAs from DSCInitialization
    -----BEGIN CERTIFICATE-----
    # ... custom CA certificate 1 ...
    -----END CERTIFICATE-----
    -----BEGIN CERTIFICATE-----
    # ... custom CA certificate 2 ...
    -----END CERTIFICATE-----
---
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: rhoai-ogx-server
spec:
  distribution:
    name: hf-serverless
  tls:
    trust:
      caCertificates:
        - name: odh-trusted-ca-bundle
          key: ca-bundle.crt
        - name: odh-trusted-ca-bundle
          key: odh-ca-bundle.crt
```

## Creating CA Bundle ConfigMaps

Every CA bundle ConfigMap must be labeled so the operator watches it:

```yaml
metadata:
  labels:
    ogx.io/watch: "true"
```

### From Certificate Files

```bash
# Create a ConfigMap from a certificate file
kubectl create configmap my-ca-bundle --from-file=ca-bundle.crt=/path/to/your/ca.crt

# Or create from multiple certificate files
kubectl create configmap my-ca-bundle \
  --from-file=ca-bundle.crt=/path/to/your/ca1.crt \
  --from-file=additional-ca.crt=/path/to/your/ca2.crt

# Label for OGX (required before the operator will track updates)
kubectl label configmap my-ca-bundle ogx.io/watch=true
```

### From Certificate Content

```bash
# Create a ConfigMap with certificate content
kubectl create configmap my-ca-bundle --from-literal=ca-bundle.crt="$(cat /path/to/your/ca.crt)"
kubectl label configmap my-ca-bundle ogx.io/watch=true
```

## Use Cases

### 1. Private Cloud Providers

When using private cloud LLM providers with self-signed certificates:

```yaml
spec:
  distribution:
    name: hf-serverless
  workload:
    overrides:
      env:
        - name: HF_API_KEY
          valueFrom:
            secretKeyRef:
              name: hf-api-key
              key: token
  overrideConfig:
    name: ogx-config
    key: config.yaml
  tls:
    trust:
      caCertificates:
        - name: private-cloud-ca-bundle
          key: ca-bundle.crt
```

### 2. Internal Enterprise APIs

For enterprise environments with internal CAs, place the CA bundle ConfigMap in the same namespace as the OGXServer and reference it by name:

```yaml
spec:
  distribution:
    name: hf-endpoint
  tls:
    trust:
      caCertificates:
        - name: enterprise-ca-bundle
          key: ca-bundle.crt
```

### 3. Development/Testing

For development environments with self-signed certificates:

```yaml
spec:
  distribution:
    name: ollama
  tls:
    trust:
      caCertificates:
        - name: dev-ca-bundle
          key: development-ca.pem
```

## Troubleshooting

### Common Issues

1. **Certificate Not Found**: Ensure the ConfigMap exists and contains the specified key
2. **Permission Denied**: Check that the operator has permissions to read the ConfigMap
3. **Invalid Certificate**: Verify the certificate format is correct (PEM format)
4. **Pod Not Restarting**: ConfigMap changes trigger automatic pod restarts via annotations
5. **ConfigMap Not Watched**: Ensure the source ConfigMap has label `ogx.io/watch: "true"`

### Common Error Messages and Solutions

#### "CA bundle key not found in ConfigMap"
- **Cause**: The specified key doesn't exist in the ConfigMap data
- **Solution**: Check the `key` field in your `spec.tls.trust.caCertificates` entry matches an existing key in the ConfigMap
- **Example**: Verify `kubectl get configmap my-ca-bundle -o yaml` shows your expected key

#### "Invalid CA bundle format"
- **Cause**: The certificate data is not in valid PEM format or contains invalid certificates
- **Solution**: Ensure certificates are properly formatted with BEGIN/END CERTIFICATE blocks
- **Example**: Valid format starts with `-----BEGIN CERTIFICATE-----`

#### "Referenced CA bundle ConfigMap not found"
- **Cause**: The ConfigMap specified in `spec.tls.trust.caCertificates[].name` doesn't exist or is not in the same namespace as the OGXServer
- **Solution**: Create the ConfigMap in the OGX namespace first, label it with `ogx.io/watch=true`, then apply the OGXServer
- **Example**: `kubectl create configmap my-ca-bundle --from-file=ca-bundle.crt=my-ca.crt && kubectl label configmap my-ca-bundle ogx.io/watch=true`

#### "No valid certificates found in CA bundle"
- **Cause**: The ConfigMap contains data but no parseable certificates
- **Solution**: Verify certificate content and format
- **Example**: Use `openssl x509 -text -noout -in your-cert.crt` to validate certificates

#### "Failed to parse certificate"
- **Cause**: Certificate data is corrupted or not a valid X.509 certificate
- **Solution**: Regenerate the certificate or verify the source
- **Example**: Check if the certificate was properly base64 encoded

### Debugging

```bash
# Check if ConfigMap exists
kubectl get configmap my-ca-bundle -o yaml

# Check pod environment variables
kubectl describe pod <ogx-pod-name>

# Check mounted certificates
kubectl exec <ogx-pod-name> -- ls -la /etc/ssl/certs/ca-bundle/

# Check SSL_CERT_FILE environment variable
kubectl exec <ogx-pod-name> -- env | grep SSL_CERT_FILE

# Validate certificate format locally
openssl x509 -text -noout -in ca-bundle.crt

# Check certificate expiration
openssl x509 -enddate -noout -in ca-bundle.crt

# Test certificate chain
openssl verify -CAfile ca-bundle.crt server.crt
```

### Validation Checklist

Before deploying an OGXServer with CA bundle:

- [ ] ConfigMap exists in the **same namespace** as the OGXServer
- [ ] ConfigMap has label `ogx.io/watch: "true"`
- [ ] ConfigMap contains the key referenced in `spec.tls.trust.caCertificates[].key`
- [ ] Certificate data is in PEM format
- [ ] Certificate data contains valid X.509 certificates
- [ ] Operator has read permissions on the ConfigMap
- [ ] Certificate is not expired
- [ ] Certificate contains the expected CA for your external service

## Security Considerations

1. **ConfigMap Security**: ConfigMaps are stored in plain text in etcd. Consider using appropriate RBAC policies
2. **Certificate Rotation**: Update ConfigMaps when certificates expire or are rotated
3. **Namespace Isolation**: CA bundle ConfigMaps must reside in the same namespace as the OGXServer; use namespaces to isolate workloads and secrets
4. **Audit Trail**: Monitor ConfigMap changes in production environments
5. **Principle of Least Privilege**: Only grant necessary permissions to access CA bundle ConfigMaps
6. **Resource Limits**: The operator enforces limits during certificate concatenation (10MB max bundle size, 1000 max certificates) to prevent resource exhaustion
7. **Input Validation**: Certificates are validated in the controller using Go's `encoding/pem` package before deployment

## Limitations

- Only supports PEM format certificates
- ConfigMap size limits apply (1MB by default for source ConfigMaps)
- Maximum bundle size is 10MB and maximum 1000 certificates (enforced by controller)
- Certificate validation is handled by Go's `encoding/pem` and `crypto/x509` packages in the controller
- The CA bundle ConfigMap must be in the same namespace as the OGXServer (cross-namespace references are not supported)
