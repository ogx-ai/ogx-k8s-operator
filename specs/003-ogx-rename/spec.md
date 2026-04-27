# Feature Specification: Rename to OGX (Open GenAI Stack) Operator

**Feature Branch**: `003-ogx-rename`
**Created**: 2026-04-16
**Updated**: 2026-04-24
**Status**: Ready
**Input**: Replace `LlamaStackDistribution` (`llamastack.io/v1alpha1`) with `OGXServer` (`ogx.io/v1beta1`). This is a breaking change — no conversion webhooks, no coexistence period. The new CRD incorporates both the rename and the expanded API surface from spec 002 (providers, resources, state storage, **`spec.network`** (port, TLS, expose, **`networkPolicy`** with native K8s ingress/egress types), workload, overrideConfig). The OGX controller handles the new CR. Provide annotation-driven migration for existing workloads to adopt legacy PVCs and networking.

**Scope note**: Spec 002 (operator-generated config) API types are folded into this CRD. The `api/v1alpha2/` package is deleted; there is no separate v1alpha2 (those types live under `ogx.io/v1beta1`). Config generation logic itself (provider expansion, resource registration, secret resolution) is deferred to a follow-up PR.

## User Scenarios & Testing

### User Story 1 - Upgrade from legacy operator with annotation-driven migration (Priority: P1)

An administrator is running the legacy operator with one or more existing workloads that have cached model data on persistent volumes. They want to switch to the renamed operator while preserving their PVC data and optionally maintaining their existing network endpoints. The administrator removes the old operator, creates a new OGXServer custom resource (manually migrating configuration from their old LLSD CR and ConfigMaps), and annotates it to adopt the legacy PVC. The new operator binds the new workload to the adopted PVC. There is a brief downtime window while the old pod terminates and the new pod mounts the PVC.

**Why this priority**: Every existing user with cached model data must have a path to preserve that data during the rename. Losing cached models (which may take hours to re-download) is unacceptable. The annotation-driven approach keeps state reproducible from the CR spec and avoids the complexity of automatic adoption with immutable Deployment selectors.

**Independent Test**: Install the legacy operator, create a workload with a populated persistent volume, remove the legacy operator (preserving the legacy CRD and CRs, or deleting the old CR with `--cascade=orphan` to preserve child resources), install the new operator, create an OGXServer CR with the `ogx.io/adopt-storage` annotation pointing to the old LLSD name, and verify (a) the persistent volume is preserved with its data intact and mounted by the new pod, and (b) the new workload comes up successfully.

**Acceptance Scenarios**:

1. **Given** the legacy operator has been removed and the old deployment is orphaned, **When** the administrator creates an OGXServer CR with `ogx.io/adopt-storage: "my-old-llsd"`, **Then** the new operator scales down or deletes the old deployment, adopts the legacy PVC by transferring its owner reference, and creates a new deployment that mounts the adopted PVC.
2. **Given** an OGXServer CR with the `ogx.io/adopt-storage` annotation, **When** the new pod starts, **Then** it has access to all previously cached model data on the adopted PVC.
3. **Given** an OGXServer CR with the same name as the old LLSD and the `ogx.io/adopt-networking: "my-old-llsd"` annotation, **When** the new operator reconciles, **Then** it adopts the old Service (updating its selectors to new pod labels) and old Ingress, transferring ownership to the new CR. Since the CR name is the same, the Service and Ingress names are unchanged and no client updates are needed.
4. **Given** an OGXServer CR with a **different** name from the old LLSD and the `ogx.io/adopt-networking: "my-old-llsd"` annotation, **When** the new operator reconciles, **Then** it adopts the legacy Service and Ingress (preserving existing endpoints) AND creates new Service and Ingress resources under the new CR name. Both sets coexist and route traffic to the new pods.
5. **Given** an adopted PVC, **When** the operator is restarted, **Then** reconciliation is idempotent and the adopted PVC remains bound to the new deployment without disruption.
6. **Given** the old deployment is still running with the PVC bound, **When** the adoption annotation is set, **Then** the operator scales the old deployment to zero to release the PVC before the new pod can mount it, resulting in a brief downtime window.

---

### User Story 2 - Clean install with no legacy state (Priority: P2)

A new user adopts the operator for the first time and has no legacy resources anywhere in their cluster. They want a clean install that does not carry migration overhead.

**Why this priority**: New users should not pay a cost for a migration they do not need. The operator must start up quickly and not create spurious warnings or perform unnecessary cluster-wide list operations.

**Independent Test**: On a cluster with no legacy custom resource definition and no legacy custom resources, install the new operator and verify startup completes cleanly with no migration activity logged and no errors related to missing legacy resources.

**Acceptance Scenarios**:

1. **Given** a cluster with no legacy custom resource definition, **When** the new operator starts, **Then** the startup log confirms no legacy resources were found and no errors are produced.
2. **Given** a fresh install, **When** the administrator creates a new-kind custom resource using the sample manifest, **Then** the workload comes up successfully with no reference to the legacy naming in operator-controlled artifacts (labels, annotations, CRD, RBAC, manifests). Upstream runtime contracts preserved per FR-002 (e.g., `LLAMA_STACK_CONFIG`, `/etc/llama-stack/`) are excluded from this check.

---

### User Story 3 - API consumers and tooling continue to function (Priority: P2)

Downstream consumers (dashboards, CI pipelines, custom scripts) that read from the custom resource's status field need to continue working after the migration.

**Why this priority**: External tooling is a real part of the operator's ecosystem. Breaking status-field consumers without warning erodes trust. A documented field rename with a clear deprecation path is acceptable; silent breakage is not.

**Independent Test**: Point an external tool that reads the server version status field at a new-kind custom resource after migration, and verify the tool can locate the renamed field using the documented migration guide.

**Acceptance Scenarios**:

1. **Given** a tool was reading the legacy status field path, **When** the migration documentation is consulted, **Then** the new field path is clearly stated and the tool can be updated.
2. **Given** a user is running read-only queries like listing resources by the legacy short name, **When** they are told to switch to the new short name, **Then** the documentation provides a one-to-one mapping for every naming change.

---

### User Story 4 - Administrator can audit and defer cleanup (Priority: P3)

A cautious administrator wants visibility into what the migration did and the ability to verify before committing to final cleanup of legacy resources.

**Why this priority**: Migrations that are opaque or irreversible feel risky. Giving administrators clear audit trails and a deferred cleanup step increases confidence in the process.

**Independent Test**: After an automated migration, inspect the cluster and verify every adopted resource carries an annotation pointing back to its legacy origin, and verify the legacy resources still exist (not auto-deleted) so the administrator can manually decide when to remove them.

**Acceptance Scenarios**:

1. **Given** adoption has completed, **When** the administrator lists new-kind resources, **Then** every migrated resource carries a machine-readable annotation identifying its legacy origin and the migration timestamp.
2. **Given** adoption has completed, **When** the administrator lists legacy resources, **Then** legacy resources still exist and are not auto-deleted.
3. **Given** an adopted workload, **When** the administrator chooses to remove the legacy resource, **Then** removing the legacy resource does not affect the running workload or its persistent volume.

---

### Edge Cases

- What happens when the legacy PVC referenced by the `ogx.io/adopt-storage` annotation does not exist? The operator logs a warning, skips adoption, and creates a new PVC normally. No error is raised to avoid blocking reconciliation.
- What happens when the old deployment is still running and holding the RWO PVC? The upgrade guide instructs users to delete orphaned stateless resources (including the old Deployment) before creating the OGXServer CR. If the user skips that step, the operator falls back to scaling the old Deployment to zero to release the PVC, then requeues to wait for pod termination before the new pod can mount it.
- What happens if the operator crashes mid-adoption (after deleting the old deployment but before the new pod starts)? Adoption is idempotent: on restart the operator detects the PVC is not yet adopted (ownerReference check), finds the old deployment already gone, and proceeds to transfer ownership and create the new deployment.
- What happens when the `ogx.io/adopt-storage` annotation is removed while the adopted PVC is in use? The PVC remains in use by the current deployment. The annotation must persist as long as the adopted PVC is bound, because the PVC name differs from the default naming convention. A status condition warns if the annotation is missing but the deployment references an adopted PVC name.
- What happens when the `ogx.io/adopt-networking` annotation is removed? When the OGXServer CR name matches the old LLSD name, the adopted Service and Ingress have the same names as what the reconciler would create, so the reconciler continues managing them normally — the annotation removal is a no-op. When the names differ, removing the annotation causes the operator to delete the adopted legacy resources (since they are no longer referenced) while retaining the new resources created under the current CR name. Administrators should ensure clients have migrated to the new endpoints before removing the annotation.
- What happens to orphaned stateless resources (NetworkPolicy, Deployment, ServiceAccount, RoleBinding, HPA, PDB) after `--cascade=orphan`? These resources lose their ownerReferences but retain the old `app: llama-stack` labels and configuration. The new operator's `patchResource` safety check skips resources it does not own, so it cannot update or replace them. Users must delete these orphaned resources before creating the OGXServer CR so the new operator can create fresh versions with correct `app: ogx` labels. This is especially critical for NetworkPolicy: the old policy's `podSelector` targets `app: llama-stack`, which no longer matches the new pods, leaving them unprotected.
- What happens when the old LLSD CR is deleted without `--cascade=orphan`? If the old CRD still exists and the CR is deleted normally, Kubernetes cascade-deletes all child resources (Deployment, PVC, Service, etc.), destroying the PVC data. The migration guide must instruct users to either delete with `--cascade=orphan` or remove the old operator first (which stops the cascade controller).
- What happens when both operators are running simultaneously? The new operator does not attempt to adopt resources owned by another controller. The `patchResource` ownership check (UID match) prevents the new operator from stealing resources still managed by the old operator.
- What happens on a cross-namespace adoption attempt? Not supported. The annotation value must reference an LLSD in the same namespace. Kubernetes ownerReferences are namespace-scoped, and the reconciler only looks up resources in the CR's own namespace.
- What happens when the adopted PVC has a different storage size than what the new CR specifies? The PVC spec is immutable after creation. The operator skips PVC patching (existing behavior) and the adopted PVC retains its original storage size. The new CR's storage spec is informational only for adopted PVCs.
- What happens when the user wants to roll back to the old operator after adoption? The user can remove the `ogx.io/adopt-storage` annotation, scale up the old deployment (if it was scaled to zero rather than deleted), and recreate the old LLSD CR. The PVC ownerReference would need to be manually restored or the old operator would create a new binding on reconcile.

## Requirements

### Functional Requirements

#### Renaming and identity

- **FR-001**: The operator's identity (name, repository, module path, container image, labels, API group, resource kind, and short name) MUST be migrated from all forms of "llama", "LlamaStack", and "llama-stack" to the corresponding "ogx" (Open GenAI Stack) naming, with the sole exceptions listed in FR-002.
- **FR-002**: Upstream runtime contracts (server startup module paths, `LLAMA_STACK_CONFIG`, `/etc/llama-stack/config.yaml`, `/.llama`, etc.) are currently being updated by the upstream project. Preserving or renaming these is **out of scope** for the initial PRs. Handle in a follow-up once upstream stabilizes.
- **FR-003**: The custom resource kind MUST be renamed to a name that reflects that the resource encompasses the full server deployment (not just a distribution selection).
- **FR-004**: The custom resource status field that reports the running server version MUST be renamed to match the new naming, and the rename MUST be documented as a breaking change for status consumers.
- **FR-005**: The operator's labels, annotations, leader-election identifier, operator-namespace name, and sample custom resources MUST all be updated to the new naming.
- **FR-006**: Container image references, registry locations, and Git/documentation URLs that still reference the legacy organization MAY remain temporarily, but the rename plan MUST enumerate them so they can be migrated on a later schedule.

#### Annotation-driven migration

- **FR-007**: The operator MUST validate that the values of the `ogx.io/adopt-storage` and `ogx.io/adopt-networking` annotations, when present, are non-empty and conform to valid Kubernetes resource names (RFC 1123 DNS label: lowercase alphanumeric or `-`, at most 63 characters, starting and ending with an alphanumeric character). If validation fails, the operator MUST set a status condition (e.g., `AdoptionConfigInvalid`) with a descriptive message and MUST skip all adoption steps for the invalid annotation. Normal reconciliation (creating fresh resources using default naming) MUST proceed so that a typo in an adoption annotation does not block the workload from deploying.

- **FR-010**: The new operator MUST support an `ogx.io/adopt-storage` annotation on the OGXServer custom resource whose value is the name of a legacy LlamaStackDistribution resource in the same namespace. When present, the operator MUST adopt the legacy PVC (`{value}-pvc`) instead of creating a new one.
- **FR-011**: When the `ogx.io/adopt-storage` annotation is present, the operator MUST replace the adopted PVC's controller owner reference: first removing the existing owner reference (which points to the legacy custom resource, and may be stale if the legacy CR was deleted), then setting a new controller owner reference pointing to the new OGXServer custom resource via `ctrl.SetControllerReference`. This ensures the PVC's lifecycle is tied to the new resource and avoids the `SetControllerReference` error that occurs when a resource already has a controller owner reference.
- **FR-012**: When the `ogx.io/adopt-storage` annotation is present and the old deployment (`{value}`) still exists and is running (i.e., the user skipped the orphan cleanup step), the operator MUST scale the old deployment to zero to release the ReadWriteOnce PVC before the new pod attempts to mount it. A brief downtime window while the PVC transfers between pods is acceptable and expected.
- **FR-013**: The operator MUST suppress creation of a new PVC from the kustomize manifest pipeline when the `ogx.io/adopt-storage` annotation is present, to avoid creating a duplicate empty PVC.
- **FR-014**: The adoption logic MUST be idempotent: if the operator restarts or re-reconciles, it MUST detect that the PVC is already adopted (via owner reference check) and not re-run destructive steps such as scaling down the old deployment.
- **FR-015**: The operator MUST support an `ogx.io/adopt-networking` annotation on the OGXServer custom resource whose value is the name of a legacy LlamaStackDistribution resource. When present, the operator MUST adopt the legacy Service (`{value}-service`) and Ingress (`{value}-ingress`), transfer their owner references to the new CR, and update the adopted Service's selectors to match the new pod labels. When the OGXServer CR name matches the old LLSD name, the adopted resource names match the reconciler's expected naming convention and no duplicate resources are created. When the names differ (e.g., OGXServer is named `my-server` but adopts networking from legacy LLSD `my-old-llsd`), the operator MUST adopt the legacy resources (`my-old-llsd-service`, `my-old-llsd-ingress`) AND create the new resources (`my-server-service`, `my-server-ingress`) from the kustomize pipeline. Both sets of resources coexist: the adopted resources preserve existing client endpoints, while the new resources provide the canonical endpoints for the new CR. The adopted resources remain owned by the new CR and are cleaned up when the administrator removes the `ogx.io/adopt-networking` annotation.
- **FR-016**: Adopted PVC, adopted Service, and adopted Ingress MUST all be reproducible from the annotation value plus the OGXServer CR spec. If an adopted resource is accidentally deleted, the operator MUST be able to recreate or re-adopt it using the same annotation.
- **FR-017**: The operator MUST report adoption status via conditions on the OGXServer custom resource (e.g., `StorageAdopted`, `NetworkingAdopted`) and emit Kubernetes events for audit purposes.
- **FR-017a**: After ownership transfer, the operator MUST annotate each adopted child resource (PVC, Service, Ingress) with `ogx.io/adopted-from: <legacy-name>` and `ogx.io/adopted-at: <RFC 3339 timestamp>` so that administrators can audit which resources were migrated and when, without inspecting operator logs.
- **FR-018**: The operator MUST NOT auto-delete legacy custom resources or the legacy custom resource definition. Administrators MUST retain manual control over cleanup of legacy resources.
- **FR-019**: The migration guide MUST instruct administrators to delete old LLSD custom resources using `--cascade=orphan` to preserve child resources, and then to delete orphaned stateless resources (Deployment, NetworkPolicy, ServiceAccount, RoleBinding, HPA, PDB) so the new operator can create fresh versions with correct labels. The guide MUST explain that orphaned NetworkPolicies with old `podSelector` labels leave new pods unprotected.
- **FR-020**: Users MUST manually create OGXServer custom resources and migrate configuration from their old LLSD CRs and ConfigMaps. The operator does not auto-create OGXServer resources from legacy CRs.

#### Documentation

- **FR-040**: A migration guide MUST be written that includes a complete old-to-new name mapping and lists every breaking change (including the status field rename) with specific instructions for external tool owners.
- **FR-041**: The migration guide MUST describe how administrators verify adoption succeeded, how they inspect the audit annotations, and how they perform the final cleanup of legacy resources and the legacy custom resource definition.
- **FR-042**: Sample custom resources, getting-started documentation, and the generated API reference MUST all reflect the new naming.

#### Deprecation

- **FR-050**: The adoption annotation support (`ogx.io/adopt-storage`, `ogx.io/adopt-networking`) and any associated RBAC permissions for managing legacy resources MUST be clearly marked as transitional and MUST have a documented removal target (a specific future release).

### Key Entities

- **Operator**: The controller running in the cluster. After migration, the operator is identified by the new naming throughout (namespace, image labels, leader-election identifier, managed-by label).
- **Custom Resource (new kind)**: The renamed custom resource that represents a running instance of the server, including server configuration, **`spec.network`** (expose, TLS, **`networkPolicy`** with per-CR enable/disable and native K8s `NetworkPolicyIngressRule`/`NetworkPolicyEgressRule` types), storage, autoscaling, and workload settings. Always created manually by administrators, both for fresh installs and upgrades. The legacy ConfigMap-based `enableNetworkPolicy` feature flag is replaced by `spec.network.networkPolicy.enabled` on each CR.
- **Custom Resource (legacy kind)**: The old-kind custom resource from the legacy operator. After the legacy operator is removed, these resources may still exist on the cluster. Administrators clean them up manually (using `--cascade=orphan` to preserve child resources).
- **Adoption annotations**: Annotations on the OGXServer CR (`ogx.io/adopt-storage`, `ogx.io/adopt-networking`) that instruct the operator to adopt legacy child resources (PVC, Ingress) by name. These annotations persist on the CR and make the adopted state reproducible.
- **Persistent volume claim**: The most important piece of stateful data in the system (holds cached models and user files). Preservation of persistent volume claims and their contents is the primary migration requirement. Adopted via the `ogx.io/adopt-storage` annotation.
- **Adopted networking resources**: The legacy Service (preserving ClusterIP for cluster-internal clients) and legacy Ingress (preserving external endpoint), both adopted via the `ogx.io/adopt-networking` annotation. The adopted Service's selectors are updated to route to the new pods. When the OGXServer CR name matches the old LLSD name, these resources have the same names as what the reconciler expects and are managed normally after ownership transfer. When the names differ, the adopted resources coexist alongside newly created resources for the new CR name.

## Assumptions

- The administrator removes the legacy operator before installing the new operator. This can be done by deleting the operator manifests directly, or via a meta-operator setting (e.g., setting the component to "Removed"). The legacy CRD and CRs may or may not remain on the cluster depending on the uninstall method.
- If the administrator wants to preserve legacy child resources (PVC, Service, Ingress) while deleting the old LLSD CR, they use `kubectl delete llsd <name> --cascade=orphan`. After that, they delete orphaned stateless resources (Deployment, NetworkPolicy, ServiceAccount, RoleBinding, HPA, PDB) so the new operator can create fresh versions. Only PVC and optionally Service + Ingress are kept for adoption.
- This is a breaking change with no conversion webhook. Users manually create new OGXServer CRs and migrate configuration from their old LLSD CRs and ConfigMaps. The new CRD incorporates the expanded API surface from spec 002.
- A brief downtime window (typically 30-60 seconds) is acceptable during PVC adoption, as the old pod must terminate before the new pod can mount a ReadWriteOnce PVC.
- Container image registry names and Git organization URLs can remain on the legacy names temporarily; their migration will be scheduled separately and does not block this rename.
- Upstream server image contracts (module paths, environment variable names, filesystem paths) are being updated upstream. Handling these is out of scope for initial PRs.
- Administrators are responsible for cleanup of legacy resources (old CRs, old CRD) after migration is complete.

## Success Criteria

### Measurable Outcomes

- **SC-001**: Persistent volume contents (cached model files, user configuration files) survive migration without loss or corruption. The verification bar is a byte-for-byte check of representative files before and after migration on at least one end-to-end test scenario.
- **SC-002**: On a cluster with no legacy state, the new operator completes startup within the same time bounds as the legacy operator did, confirming that no migration-related overhead is introduced for clean installs.
- **SC-003**: The published migration guide allows an administrator unfamiliar with the internal migration mechanics to successfully migrate a test workload using only the documentation, measured by a documentation walkthrough on a test cluster.
- **SC-004**: Status-field consumers (dashboards, external scripts) can update their queries using only the old-to-new field mapping table in the migration guide, with no additional investigation needed.
- **SC-005**: The rename leaves no residual legacy naming in any operator-owned artifact (custom resource definition, role-based access, deployment, labels, annotations, leader-election identifier, sample manifests, documentation) except the explicitly-listed exceptions (upstream runtime contracts) and the deferred items (container registry, Git organization URLs).
- **SC-006**: A user running a read-only query for the new resource kind (by the new plural or short name) gets the expected results, proving the naming migration is complete from an end-user command-line perspective.
- **SC-007**: An OGXServer CR with `ogx.io/adopt-storage` annotation successfully mounts the legacy PVC and the new pod starts serving with cached model data available.
- **SC-008**: The adopted PVC and Ingress state is reproducible from the OGXServer CR spec plus annotations. If either resource is accidentally deleted, the operator can recreate or re-adopt it on the next reconcile.
- **SC-009**: The migration downtime window for PVC adoption (old pod termination to new pod ready) is under 90 seconds for a typical deployment with default graceful termination settings.
