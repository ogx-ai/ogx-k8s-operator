# Deploying the `ogx-k8s-operator` as an OpenShift Catalog Item

This document outlines the steps required to package the `ogx-k8s-operator` into an Operator Bundle and make it available as a catalog item in OpenShift's OperatorHub.

## Overview

To expose the `ogx-k8s-operator` through the OpenShift OperatorHub, we need to package it according to the Operator Lifecycle Manager (OLM) specifications. This involves creating an Operator Bundle, building a bundle image, compiling a custom catalog image, and finally, registering this catalog with OpenShift using a `CatalogSource`.

## 2 Prerequisites

Before you begin, ensure you have the following:

  * **OpenShift Cluster:** Access to an OpenShift cluster with `cluster-admin` privileges.
  * **Operator SDK:** The Operator SDK CLI tool installed on your local machine.
      * [Operator SDK Installation Guide](https://sdk.operatorframework.io/docs/building-operators/golang/installation/)
  * **Go and Podman:**
  * **`oc` CLI:** The OpenShift command-line interface, configured to connect to your cluster.

## Steps

Follow these steps to prepare and deploy your operator.

### Clone the Operator Repository

Start by cloning the `ogx-k8s-operator` repository to your local machine:

```bash
git clone https://github.com/ogx-ai/ogx-k8s-operator.git
cd ogx-k8s-operator
```

### Set Env Variables

```bash
export REGISTRY=quay.io
export REGISTRY_NAMESPACE=<your_namespace>
export TAG="1.0"
```

### Build the Operator Image

The operator's container image needs to be built and pushed to a container registry.

**Note:** Replace `<your-namespace>` with your actual registry and namespace.

```bash
make image-build IMG=$REGISTRY/$REGISTRY_NAMESPACE/ogx-k8s-operator:$TAG
podman push $REGISTRY/$REGISTRY_NAMESPACE/ogx-k8s-operator:$TAG
```

### 3.3. Generate the Operator Bundle

An Operator Bundle is a directory containing the necessary manifests that describe your Operator, including its Custom Resource Definitions (CRDs), ClusterServiceVersion (CSV), and other related files. The Operator SDK helps automate this.

```bash
# This will generate the bundle manifests in the 'bundle' directory.
# Ensure you are in the root directory of the operator project.
make bundle
```

After running `make bundle`, inspect the contents of the newly created `bundle/manifests/bases` directory. Pay close attention to the `ogx-k8s-operator.clusterserviceversion.yaml` file. You may need to edit this file to:

  * **`spec.installModes`**: Should be se to `AllNamespaces`
  * **`spec.replaces`**: If you have previous versions of the operator, this field is crucial for OLM to manage upgrades.
  * **`spec.version`**: The current version of your operator (e.g., `0.0.1`).
  * **`spec.customresourcedefinitions`**: Ensure all CRDs managed by your operator are correctly listed.
  * **`spec.install.spec.deployments`**: Verify that the `image` field for your operator's deployment points to the image you built and pushed in Step 3.2.

### 3.4. Build the Bundle Image

The generated bundle needs to be packaged into its own container image.

```bash
make bundle-build BUNDLE_IMG=$REGISTRY/$REGISTRY_NAMESPACE/ogx-k8s-operator-bundle:$TAG
podman push $REGISTRY/$REGISTRY_NAMESPACE/ogx-k8s-operator-bundle:$TAG
```

### 3.5. Create a Custom Catalog (Index) Image

To make your operator discoverable in the OpenShift OperatorHub, you must create a custom catalog that includes your operator's bundle. This catalog is also a container image.

We'll use the `opm` (Operator Package Manager) CLI tool, which is part of the Operator SDK.

```bash
# 1. Initialize a new catalog (if you don't have one).
#    This creates a Dockerfile for the catalog and an empty index.yaml.
#    You can create a new directory for your catalog, e.g., 'catalog_dir'.
mkdir -p catalog_dir
opm init ogx-catalog --output yaml > catalog_dir/index.yaml

# 2. Add your operator bundle to the catalog.
#    Replace with your actual bundle image.
opm index add \
  --bundles $REGISTRY/$REGISTRY_NAMESPACE/ogx-k8s-operator-bundle:$TAG \
  --tag $REGISTRY/$REGISTRY_NAMESPACE/ogx-catalog:$TAG \
  --build-tool podman

# 3. Push the catalog image to your registry.
podman push $REGISTRY/$REGISTRY_NAMESPACE/ogx-catalog:$TAG
```

### 3.6. Create a `CatalogSource` in OpenShift

Finally, you inform OpenShift about your new custom catalog by creating a `CatalogSource` object. This makes your operator visible in the OperatorHub.

Create a YAML file (e.g., `.openshift/catalog-item.yaml`):

```yaml
apiVersion: operators.coreos.com/v1alpha1
kind: CatalogSource
metadata:
  name: ogx-catalog
  namespace: openshift-marketplace # Standard namespace for CatalogSources
  labels:
    environment: dev
spec:
  sourceType: grpc
  image: $REGISTRY/$REGISTRY_NAMESPACE/ogx-catalog:$TAG # Your catalog image from Step 3.5
  displayName: "Llama Stack Operator Catalog (dev)"
  publisher: "Red Hat Community"
  updateStrategy:
    registryPoll:
      interval: 30m # How often OpenShift should check for updates to your catalog
```

Apply this `CatalogSource` to your OpenShift cluster:

```bash
oc apply -f .openshift/catalog-item.yaml
```

### 3.7. Verify in OperatorHub

After applying the `CatalogSource`, allow a few moments for OpenShift to process it. You should then be able to:

1.  Log in to the OpenShift Web Console.
2.  Navigate to **Operators \> OperatorHub**.
3.  In the "Catalog Sources" filter, you should see "Llama Stack Operator Catalog" listed.
4.  Filter by this catalog, and your OGX K8s operator should appear, ready for installation.
