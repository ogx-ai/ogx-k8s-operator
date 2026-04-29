# Feature: Operator-Generated Server Configuration (v1alpha2)

**Status**: Draft
**Created**: 2026-02-02
**Priority**: P1
**Depends on**: 001-deploy-time-providers-l1 (external providers merge into generated config)
**Design Doc**: [LlamaStackDistribution CRD v1alpha2 Schema Design](https://docs.google.com/document/d/10VhoQPb8bLGUo9yka4MXuEGIZClGf1oBr31TpK4NLD0/edit)

## Purpose

Enable the llama-stack Kubernetes operator to generate the server configuration (config.yaml) from a high-level, abstracted specification in the LlamaStackDistribution CR v1alpha2, rather than requiring users to provide a complete ConfigMap. This empowers users to configure LlamaStack with minimal YAML while preserving the flexibility to override any setting when needed.

## User Scenarios & Testing

### User Story 1 - Simple Inference Configuration (Priority: P1)

As a developer, I want to deploy a llama-stack instance with a vLLM backend using just a few lines of YAML, so that I can get started quickly without understanding the full config.yaml schema.

**Why this priority**: Core value proposition. Most users need simple inference configuration.

**Independent Test**: Deploy a LLSD CR with minimal `providers.inference` configuration and verify the server starts with the provider accessible via the `/v1/providers` API.

**Acceptance Scenarios**:

1. **Given** a LLSD CR with `providers.inference: [{provider: vllm, endpoint: "http://vllm:8000"}]`, **When** I apply the CR, **Then** the operator generates a valid config.yaml with the vLLM provider configured
2. **Given** a LLSD CR with `providers.inference.apiKey.secretKeyRef`, **When** I apply the CR, **Then** the secret value is injected via environment variable and the provider can authenticate
3. **Given** a LLSD CR with only `distribution.name: starter`, **When** I apply the CR, **Then** the distribution's default config.yaml is used unchanged

### User Story 2 - Multiple Providers Configuration (Priority: P1)

As a platform engineer, I want to configure multiple inference providers (primary and fallback) in a single LLSD, so that I can provide high availability without managing complex ConfigMaps.

**Why this priority**: Production deployments commonly require multiple providers.

**Independent Test**: Deploy a LLSD CR with multiple inference providers using list form, verify all providers appear in the `/v1/providers` API.

**Acceptance Scenarios**:

1. **Given** a LLSD CR with `providers.inference` containing two `ProviderConfig` entries with explicit IDs, **When** I apply the CR, **Then** both providers are configured and accessible, and status condition `ConfigGenerated` is `True`
2. **Given** a LLSD CR with multiple providers in a list without explicit IDs, **When** I apply the CR, **Then** CEL validation rejects the CR at admission with a clear error message requiring explicit IDs
3. **Given** a LLSD CR with duplicate provider IDs across any provider types, **When** I apply the CR, **Then** CEL validation rejects the CR at admission with a clear error message listing the duplicate IDs

### User Story 3 - Resource Registration (Priority: P1)

As a developer, I want to register models and tools declaratively in the CR, so that they are available immediately when the server starts.

**Why this priority**: Models and tools are essential for using llama-stack.

**Independent Test**: Deploy a LLSD CR with `resources.models` and `resources.tools`, verify resources appear in the respective API endpoints.

**Acceptance Scenarios**:

1. **Given** a LLSD CR with `resources.models: [{name: "llama3.2-8b"}]`, **When** I apply the CR, **Then** the model is registered with the first configured inference provider
2. **Given** a LLSD CR with a model specifying explicit provider assignment, **When** I apply the CR, **Then** the model is registered with the specified provider
3. **Given** a LLSD CR with `resources.tools: [websearch, rag]`, **When** I apply the CR, **Then** the tool groups are registered and available

### User Story 4 - State Storage Configuration (Priority: P1)

As a platform operator, I want to configure PostgreSQL for state storage, so that llama-stack data persists across restarts and scales horizontally.

**Why this priority**: Production deployments require persistent storage.

**Independent Test**: Deploy a LLSD CR with `storage.sql` configuration, verify the server uses PostgreSQL for state storage.

**Acceptance Scenarios**:

1. **Given** a LLSD CR with `storage.sql: {type: postgres, connectionString: {secretKeyRef: ...}}`, **When** I apply the CR, **Then** the server uses PostgreSQL for SQL storage
2. **Given** a LLSD CR with `storage.kv: {type: redis, endpoint: "..."}`, **When** I apply the CR, **Then** the server uses Redis for key-value storage
3. **Given** a LLSD CR without storage configuration, **When** I apply the CR, **Then** the server uses default SQLite storage

### User Story 5 - Network Exposure Configuration (Priority: P2)

As a platform operator, I want to expose the llama-stack service externally with TLS, so that clients can access it securely from outside the cluster.

**Why this priority**: External access is a common production requirement.

**Independent Test**: Deploy a LLSD CR with `networking.expose: true` and `networking.tls`, verify Ingress/Route is created with TLS configured.

**Acceptance Scenarios**:

1. **Given** a LLSD CR with `networking.expose: true`, **When** I apply the CR, **Then** an Ingress/Route is created with an auto-generated hostname
2. **Given** a LLSD CR with `networking.expose: {hostname: "llama.example.com"}`, **When** I apply the CR, **Then** an Ingress/Route is created with the specified hostname
3. **Given** a LLSD CR with `networking.tls: {enabled: true, secretName: "..."}`, **When** I apply the CR, **Then** the server uses the specified TLS certificate

### User Story 6 - Full ConfigMap Override (Priority: P2)

As a power user, I want to provide my own complete config.yaml via ConfigMap, so that I can use features not yet exposed in the CRD schema.

**Why this priority**: Escape hatch for advanced use cases.

**Independent Test**: Deploy a LLSD CR with `overrideConfig.configMapName`, verify the server uses the ConfigMap contents.

**Acceptance Scenarios**:

1. **Given** a LLSD CR with `overrideConfig.configMapName: my-config`, **When** I apply the CR, **Then** the server uses the ConfigMap's config.yaml
2. **Given** a LLSD CR with both `providers` and `overrideConfig`, **When** I apply the CR, **Then** validation fails with a mutual exclusivity error
3. **Given** a LLSD CR with `overrideConfig` referencing a non-existent ConfigMap, **When** I apply the CR, **Then** the status shows a clear error about the missing ConfigMap

### User Story 7 - Migration from v1alpha1 (Priority: P2)

As an existing user, I want my v1alpha1 CRs to continue working after the operator upgrade, so that I don't experience downtime during the transition.

**Why this priority**: Backward compatibility is essential for adoption.

**Independent Test**: Apply a v1alpha1 CR, upgrade operator, verify the CR continues to work and can be retrieved as v1alpha2.

**Acceptance Scenarios**:

1. **Given** an existing v1alpha1 LLSD CR, **When** I upgrade the operator to support v1alpha2, **Then** the existing CR continues to function
2. **Given** a v1alpha1 CR, **When** I retrieve it as v1alpha2, **Then** the conversion webhook translates fields correctly
3. **Given** a v1alpha2 CR, **When** I retrieve it as v1alpha1, **Then** the conversion webhook translates fields correctly (where mappable)

### User Story 8 - Runtime Configuration Updates (Priority: P1)

As a platform operator, I want to update the LLSD CR (e.g., add a provider, change storage) and have the running LlamaStack instance pick up the changes automatically, so that I can manage configuration declaratively without manual restarts.

**Why this priority**: Day-2 operations are essential for production use. Without this, users must delete and recreate CRs to change configuration.

**Independent Test**: Deploy a LLSD CR, wait for Ready, modify the CR's providers section, verify the Pod restarts with the updated config.yaml.

**Acceptance Scenarios**:

1. **Given** a running LLSD instance, **When** I add a new provider to `spec.providers`, **Then** the operator regenerates config.yaml, creates a new ConfigMap, and triggers a rolling update
2. **Given** a running LLSD instance, **When** I modify `spec.providers` but the generated config.yaml is identical (e.g., whitespace-only change), **Then** the operator does NOT restart the Pod
3. **Given** a running LLSD instance, **When** I update `spec.providers` with an invalid configuration, **Then** the operator preserves the current running config, reports the error in status, and does NOT disrupt the running instance
4. **Given** a running LLSD instance, **When** I change `spec.distribution.name` to a different distribution, **Then** the operator updates both the image and config atomically in a single Deployment update

### Edge Cases

- **Provider with settings escape hatch**:
  - What: User specifies `providers.inference.settings: {max_tokens: 8192}`
  - Expected: Extra fields are passed through to config.yaml provider config section

- **Secret reference to non-existent secret**:
  - What: `secretKeyRef` points to a secret that doesn't exist
  - Expected: Reconciliation fails with clear error, status shows "Secret not found: {name}"

- **~~Expose with empty object~~** *(v1beta1: `expose` renamed to `externalAccess` with explicit `enabled` default false — see D-030. An empty `externalAccess: {}` means disabled.)*:
  - What: User specifies `externalAccess: {}`
  - Expected: External access is disabled (default `enabled: false`)

- **Disabled APIs conflict with providers**:
  - What: User configures `providers.inference` but also `disabledAPIs: [inference]` *(v1beta1: renamed from `disabled` — see D-031)*
  - Expected: Validation fails with error: "inference cannot be both in providers and disabledAPIs". Accepting contradictory config and silently ignoring part of it leads to user confusion. Failing fast is more Kubernetes-idiomatic.

- **Model references non-existent provider**:
  - What: `resources.models[].provider` references an ID not in `providers`
  - Expected: Validation fails with clear error listing available provider IDs

- **Config.yaml schema version mismatch**:
  - What: Distribution image has unsupported config.yaml version
  - Expected: Reconciliation fails with clear error about version incompatibility

- **CR update during active rollout**:
  - What: User updates the CR while a previous config change is still rolling out
  - Expected: Operator generates config from the latest CR spec, superseding the in-progress rollout. The Deployment converges to the newest desired state.

- **Operator upgrade with running LLSD instances**:
  - What: Operator is upgraded from v1 to v2, changing the image that `distribution.name: rh-dev` resolves to
  - Expected: Operator detects the image change, regenerates config using the new base config matching the new image, and updates the Deployment atomically (new image + new config together). If config generation fails for the new image, the operator preserves the current running Deployment and reports the error in status.

- **Config generation failure on update**:
  - What: User changes CR in a way that produces an invalid merged config (e.g., references a provider type not supported by the distribution)
  - Expected: Operator keeps the current running config and Deployment unchanged, sets `ConfigGenerated=False` with a descriptive error, and does not trigger a Pod restart

- **Secret references via settings vs secretRefs**:
  - What: User puts a secretKeyRef inside `settings` instead of using the `secretRefs` field
  - Expected: The `settings` map is passed through to config.yaml as-is without any secret resolution. Only the explicit `secretRefs` field triggers secret-to-env-var resolution *(v1beta1: `apiKey` removed — see D-035)*. This is a clear, unambiguous boundary (no heuristic matching of map shapes).

- **Tools specified without toolRuntime provider**:
  - What: User specifies `resources.tools: [websearch]` but does not configure `providers.toolRuntime`
  - Expected: If the base config provides a default toolRuntime provider, tools are registered with it. If no toolRuntime provider exists in either user config or base config, validation fails with: "resources.tools requires at least one toolRuntime provider to be configured"

- **Shields specified without safety provider**:
  - What: User specifies `resources.shields: [llama-guard]` but does not configure `providers.safety`
  - Expected: If the base config provides a default safety provider, shields are registered with it. If no safety provider exists in either user config or base config, validation fails with: "resources.shields requires at least one safety provider to be configured"

## Requirements

### Functional Requirements

#### CRD Schema (v1alpha2)

- **FR-001**: The CRD MUST define a new API version `v1alpha2` with the redesigned schema
- **FR-002**: The `spec.distribution` field MUST support both `name` (mapped) and `image` (direct) forms, mutually exclusive
- **FR-003**: The `spec.providers` section MUST support provider types: `inference`, `safety`, `vectorIo`, `toolRuntime` ~~, `telemetry`~~ *(telemetry removed in v1beta1 — see D-033)*
- **FR-004**: Each provider field MUST be a list of `ProviderConfig` objects (`[]ProviderConfig`). A single provider is expressed as a one-element list. This ensures kubebuilder validation markers apply to all provider fields and CEL rules can inspect provider IDs for uniqueness.
- **FR-005**: Each `ProviderConfig` MUST support fields: `id` (unique provider identifier), `provider` (type, required — *v1beta1: must have `remote::` or `inline::` prefix, see D-034*), `endpoint`, ~~`apiKey` (secretKeyRef),~~ `secretRefs` (named secret references map — *v1beta1: replaces `apiKey`, see D-035*), `settings` (escape hatch). Provider-specific connection fields (e.g., `host` for vectorIo) MUST use `secretRefs` entries rather than embedding `secretKeyRef` inside `settings`. The `secretRefs` field is a `map[string]SecretKeyRef` where each key becomes the env var field suffix. The `settings` map is passed through to config.yaml as-is without secret resolution.
- **FR-006**: The `spec.resources` section MUST support: `models`, `tools`, `shields`
- **FR-007**: `resources.models` MUST be a list of `ModelConfig` objects (`[]ModelConfig`), where only the `name` field is required. Simple model references use `ModelConfig` with just `name` set. `resources.tools` and `resources.shields` MUST be lists of strings.
- **FR-008**: The `spec.storage` section MUST have subsections: `kv` (key-value) and `sql` (relational)
- **FR-009**: The `spec.disabled` field MUST be a list of API names to disable *(v1beta1: renamed to `spec.disabledAPIs`, enum restricted to `agents`, `inference`, `tool_runtime`, `vector_io` — see D-031)*
- **FR-010**: The `spec.networking` section MUST consolidate: `port`, `tls`, `expose`, `allowedFrom`
- **FR-011**: The `networking.expose` field MUST be an object with optional `enabled` (bool) and `hostname` (string) fields. When `enabled` is true (or when the object is present with defaults), an Ingress/Route is created. When `hostname` is specified, it is used for the Ingress/Route hostname.
- **FR-012**: The `spec.workload` section MUST contain K8s deployment settings: `replicas`, `workers`, `resources`, `autoscaling`, `storage`, `podDisruptionBudget`, `topologySpreadConstraints`, `overrides`
- **FR-013**: The `spec.overrideConfig` field MUST be mutually exclusive with `providers`, `resources`, `storage`, `disabled`. The referenced ConfigMap MUST reside in the same namespace as the LLSD CR (consistent with namespace-scoped RBAC, constitution section 1.1)
- ~~**FR-014**: The `spec.externalProviders` field MUST remain for integration with spec 001~~ *(removed in v1beta1 — design not yet finalized, see D-032)*

#### Configuration Generation

- **FR-020**: The operator MUST resolve `distribution.name` to a concrete image reference using the embedded distribution registry (`distributions.json`) and operator config overrides (`image-overrides`)
- **FR-020a**: The operator MUST record the resolved image reference in `status.resolvedDistribution.image` after successful resolution
- **FR-020b**: When the resolved image changes between reconciliations (e.g., after operator upgrade changes the `distributions.json` mapping), the operator MUST regenerate config.yaml using the base config matching the new image and update the Deployment atomically (image + config in a single update)
- **FR-020c**: The operator MUST NOT update a running LLSD's config without also updating its image to match. Image and base config MUST always be consistent.
- **FR-021**: The operator MUST generate a complete config.yaml by merging user configuration over base defaults
- **FR-022**: The operator MUST resolve `secretKeyRef` references to environment variables with deterministic naming
- **FR-023**: The operator MUST create a ConfigMap containing the generated config.yaml
- **FR-024**: The ConfigMap name MUST include a content hash for change detection
- **FR-025**: The operator MUST set owner references on the generated ConfigMap for garbage collection
- **FR-025a**: During each reconciliation, the controller MUST delete generated ConfigMaps beyond the most recent 2 (current + previous). ConfigMaps are identified by the `app.kubernetes.io/managed-by` label and sorted by creation timestamp. This prevents accumulation of stale ConfigMaps during the CR's lifetime.
- **FR-026**: The operator MUST add a hash annotation to the Deployment to trigger rollouts on config changes
- **FR-027**: The operator MUST detect the config.yaml schema version from the base configuration
- **FR-028**: The operator MUST support config.yaml schema versions n and n-1 (current and previous)
- **FR-029**: The operator MUST reject unsupported config.yaml versions with error: "Unsupported config.yaml version {version}. Supported versions: {list}"

#### Base Config Extraction (Phased Approach)

The base config extraction follows a phased approach. Phase 1 provides an implementation that works without changes to distribution image build processes. Phase 2 adds OCI label-based extraction for distributions that support it.

**Phase 1 - Embedded Default Configs (MVP)**

- **FR-027a**: The operator MUST include embedded default configurations for all distribution names defined in `distributions.json`, shipped as part of the operator binary
- **FR-027a1**: The base config extraction MUST be implemented behind a `ConfigResolver` interface with a `Resolve(image string) ([]byte, error)` method. Phase 1 provides `EmbeddedConfigResolver`; Phase 2 adds `OCIConfigResolver`. The controller selects the resolver based on available config sources (OCI labels take precedence over embedded, per FR-027g).
- **FR-027b**: When `distribution.name` is specified, the operator MUST use the embedded config for that distribution as the base for config generation
- **FR-027c**: When `distribution.image` is specified (direct image reference, no named distribution), the operator MUST require `overrideConfig.configMapName` to provide the base configuration. If `overrideConfig` is not set and no OCI config labels are found (see Phase 2), the operator MUST set `ConfigGenerated=False` with reason `BaseConfigRequired` and message: "Direct image references require either overrideConfig.configMapName or OCI config labels on the image. See docs/configuration.md for details."
- **FR-027d**: The embedded configs MUST be versioned together with the distribution image mappings in `distributions.json`, ensuring each distribution name maps to a consistent (image, config) pair per operator release
- **FR-027e**: The operator MUST support air-gapped environments where images are mirrored to internal registries. The embedded config is used regardless of where the image is pulled from.

**Phase 2 - OCI Label Extraction (Enhancement)**

- **FR-027f**: Distribution images MAY include `config.yaml` in OCI labels using one of:
  - `io.llamastack.config.base64`: Base64-encoded config (for configs < 50KB)
  - `io.llamastack.config.layer` + `io.llamastack.config.path`: Layer digest and path reference (for larger configs)
- **FR-027g**: When OCI config labels are present on the resolved image, the operator MUST use the label-provided config as the base, taking precedence over embedded defaults
- **FR-027h**: The operator MUST use `k8schain` authentication to access registries using the same credentials as kubelet (imagePullSecrets, ServiceAccount)
- **FR-027i**: The operator MUST cache extracted configs by image digest to avoid repeated registry fetches
- **FR-027j**: OCI label extraction enables `distribution.image` usage without `overrideConfig`, since the image itself provides the base config

#### Provider Configuration

- **FR-030**: Provider `provider` field MUST map to `provider_type` ~~with `remote::` prefix (e.g., `vllm` becomes `remote::vllm`)~~ *(v1beta1: explicit `remote::` or `inline::` prefix required on the field value itself — no implicit normalization, see D-034)*
- **FR-031**: Provider `endpoint` field MUST map to `config.url` in config.yaml
- **FR-032**: Provider `apiKey.secretKeyRef` and `secretRefs` entries MUST be resolved to environment variables and referenced as `${env.LLSD_<PROVIDER_ID>_<FIELD>}`, where `<PROVIDER_ID>` is the provider's unique `id` (explicit or auto-generated per FR-035), uppercased with hyphens replaced by underscores. For `apiKey`, the field suffix is `API_KEY`. For `secretRefs`, the map key is uppercased with hyphens replaced by underscores. Example: provider ID `vllm-primary` with `apiKey` produces `LLSD_VLLM_PRIMARY_API_KEY`; `secretRefs.host` produces `LLSD_VLLM_PRIMARY_HOST`.
- **FR-033**: Provider `settings` MUST be merged into the provider's `config` section in config.yaml
- **FR-034**: When multiple providers are specified for the same API type, each MUST have an explicit `id` field. CEL validation enforces this at admission time.
- **FR-035**: A single provider (one-element list) without `id` MUST auto-generate `provider_id` from the `provider` field value

#### ~~Telemetry Provider~~ *(removed in v1beta1 — see D-033)*

- ~~**FR-036**: Telemetry providers follow the same schema as other provider types (FR-004, FR-005). The `provider` field maps to the telemetry backend (e.g., `opentelemetry`). The `endpoint` and `settings` fields configure the telemetry destination. No telemetry-specific fields are defined beyond the standard provider schema.~~ *(removed in v1beta1 — telemetry provider type doesn't exist upstream)*

#### Resource Registration

- **FR-040**: Simple model strings MUST be registered with the inference provider. When multiple inference providers are configured (list form), the first provider in list order is used. When a single provider is configured (object form), that provider is used.
- **FR-041**: Model objects with explicit `provider` MUST be registered with the specified provider
- **FR-042**: Model objects MAY include metadata fields: `contextLength`, `modelType`, `quantization`
- **FR-043**: Tools MUST be registered as tool groups with the configured toolRuntime provider. When no explicit toolRuntime provider is configured in the CR, the base config's toolRuntime provider is used. If no toolRuntime provider exists in either source, controller validation MUST fail with an actionable error
- **FR-044**: Shields MUST be registered with the configured safety provider. When no explicit safety provider is configured in the CR, the base config's safety provider is used. If no safety provider exists in either source, controller validation MUST fail with an actionable error

#### Storage Configuration

- **FR-050**: `storage.kv.type` MUST support values: `sqlite`, `redis`
- **FR-050a**: `storage.kv` for type `redis` MUST support fields: `endpoint`, `password` (secretKeyRef)
- **FR-050b**: When `storage.kv` is specified without `type`, the operator MUST default to `sqlite`
- **FR-051**: `storage.sql.type` MUST support values: `sqlite`, `postgres`
- **FR-051a**: When `storage.sql` is specified without `type`, the operator MUST default to `sqlite`
- **FR-052**: `storage.sql.connectionString` MUST support `secretKeyRef` for secure credential handling
- **FR-053**: When `storage` section is not specified at all, distribution defaults MUST be preserved (no override)

#### Networking Configuration

- **FR-060**: `networking.port` MUST default to 8321 if not specified
- **FR-061**: `networking.tls.enabled: true` MUST configure the server for TLS
- **FR-062**: `networking.tls.secretName` MUST reference a Kubernetes TLS secret
- **FR-063**: `networking.tls.caBundle` MUST support custom CA certificates via ConfigMap reference
- **FR-064**: `networking.expose: true` MUST create an Ingress/Route with auto-generated hostname
- **FR-065**: `networking.expose.hostname` MUST create an Ingress/Route with the specified hostname
- **FR-066**: `networking.allowedFrom` MUST configure NetworkPolicy for namespace-based access control

#### Validation

- **FR-070**: CEL validation MUST enforce mutual exclusivity between `overrideConfig` and each of `providers`, `resources`, `storage`, and `disabled`
- ~~**FR-071**: CEL validation MUST require explicit `id` on each `ProviderConfig` when a provider list has more than one element. Rule applied per provider type field (e.g., `self.providers.inference.size() <= 1 || self.providers.inference.all(p, has(p.id))`)~~ *(v1beta1: replaced by validating webhook for global provider ID uniqueness — see D-036)*
- **FR-072**: CEL validation MUST enforce unique provider IDs across all provider types. Since providers are typed slices, CEL can inspect IDs directly (e.g., check that the union of all provider IDs has no duplicates)
- **FR-073**: Controller validation MUST verify referenced Secrets exist before generating config
- **FR-074**: Controller validation MUST verify referenced ConfigMaps exist for `overrideConfig` and `caBundle`
- **FR-075**: Validation errors MUST include actionable messages with field paths
- **FR-076**: A validating admission webhook MUST validate CR creation and update operations for constraints that cannot be expressed in CEL (e.g., Secret existence checks, ConfigMap existence for `overrideConfig`, cross-field semantic validation such as provider ID references in resources)
- **FR-077**: The validating webhook MUST return structured error responses with field paths and actionable messages following Kubernetes API conventions
- **FR-078**: The validating webhook MUST be deployed as part of the operator installation and configured via the operator's kustomize manifests with appropriate certificate management
- ~~**FR-079**: CEL validation MUST enforce that `networking.tls.secretName` is required when `networking.tls.enabled` is true: `!self.tls.enabled || has(self.tls.secretName)`~~ *(v1beta1: TLS uses presence semantics — `secretName` is always required when `tls` is present, no `enabled` bool — see D-028)*
- **FR-079a**: CEL validation MUST enforce that `storage.kv.endpoint` is required when `storage.kv.type` is `redis`: `self.storage.kv.type != 'redis' || has(self.storage.kv.endpoint)`
- **FR-079b**: CEL validation MUST enforce that `storage.sql.connectionString` is required when `storage.sql.type` is `postgres`: `self.storage.sql.type != 'postgres' || has(self.storage.sql.connectionString)`
- **FR-079c**: CEL validation SHOULD warn when `storage.kv.endpoint` or `storage.kv.password` are specified with `storage.kv.type: sqlite` (unused fields)
- **FR-079d**: The validating webhook MUST reject CRs where `distribution.name` references a name not present in the embedded distribution registry (`distributions.json`). The error message MUST list available distribution names: "Unknown distribution \"{name}\". Available distributions: {list}"

#### API Version Conversion

- **FR-080**: A conversion webhook MUST translate between v1alpha1 and v1alpha2
- **FR-081**: v1alpha2 MUST be the storage version
- **FR-082**: v1alpha1 CRs MUST continue to be served for backward compatibility
- **FR-083**: Field mapping MUST follow the documented migration table (see Design Doc)

#### Integration with Spec 001

- **FR-090**: Generated config MUST serve as the base for external provider merging
- **FR-091**: External providers (from spec 001) MUST be additive to inline providers
- **FR-092**: When external provider ID conflicts with inline provider, external MUST override with warning

#### Runtime Configuration Updates

- **FR-095**: When the CR `spec` changes, the operator MUST regenerate config.yaml, create a new ConfigMap with an updated content hash, and update the Deployment to reference the new ConfigMap
- **FR-096**: The operator MUST NOT restart Pods if the generated config.yaml content is identical to the currently deployed config (content hash comparison)
- **FR-097**: If config generation or validation fails during a CR update, the operator MUST preserve the current running Deployment (image, ConfigMap, env vars) unchanged and set status condition `ConfigGenerated=False` with the failure reason. The running instance MUST NOT be disrupted.
- **FR-098**: When `spec.distribution` changes (name or image), the operator MUST update the Deployment atomically per FR-100
- **FR-099**: Status conditions MUST reflect the reconciliation state during updates: `ConfigGenerated` (config.yaml created successfully), `DeploymentUpdated` (Deployment spec updated), `Available` (at least one Pod is ready with the current config)

#### Distribution Lifecycle

- **FR-100**: The operator MUST update the Deployment's container image and its generated config atomically in a single Deployment update. There MUST be no intermediate state where the running image and config are mismatched.
- **FR-101**: When config generation fails after an operator upgrade (e.g., the new base config is incompatible with the user's CR fields), the operator MUST preserve the running Deployment per FR-097 and report the failure with reason `UpgradeConfigFailure`

### Non-Functional Requirements

- **NFR-001**: Configuration generation MUST be deterministic (same inputs produce same outputs)
- **NFR-002**: Configuration generation MUST complete within 5 seconds for typical configurations
- **NFR-003**: Secret resolution MUST NOT expose secret values in logs or status
- **NFR-004**: Error messages MUST be actionable (user can resolve without operator knowledge)
- **NFR-005**: The generated ConfigMap MUST be immutable (new ConfigMap on changes, not updates)
- **NFR-006**: Config extraction from images MUST be cached to avoid repeated image pulls
- **NFR-007**: The operator MUST emit Kubernetes Events for significant state changes: `ConfigGenerated` (Normal), `DeploymentUpdated` (Normal), `ConfigGenerationFailed` (Warning), `SecretResolutionFailed` (Warning). Events provide a time-ordered audit trail complementing status conditions (which only show current state).

### External Dependencies

#### Operator Build Requirements (Phase 1)

The operator binary MUST embed default configs for all named distributions:

| Artifact | Description | Required |
|----------|-------------|----------|
| `distributions.json` | Maps distribution names to image references | Yes |
| `configs/<name>/config.yaml` | Default config.yaml for each named distribution | Yes |

The `distributions.json` and corresponding config files are maintained together and updated as part of the operator release process. For downstream builds (e.g., RHOAI), the `image-overrides` mechanism in the operator ConfigMap allows overriding the embedded image references without rebuilding the operator.

#### Distribution Image Build Requirements (Phase 2)

Distribution images MAY include OCI labels for base config extraction. These labels are added post-build using `crane mutate`:

| Label | Description | Required |
|-------|-------------|----------|
| `io.llamastack.config.base64` | Base64-encoded config.yaml | For configs < 50KB |
| `io.llamastack.config.layer` | Layer digest containing config | For configs >= 50KB |
| `io.llamastack.config.path` | Path to config.yaml within layer | Required with `layer` |
| `io.llamastack.config.version` | Config schema version (e.g., "2") | Recommended |

**Build Process**:
```bash
# 1. Build and push image
docker build -t ${IMAGE}:build . && docker push ${IMAGE}:build

# 2. Add labels post-build (crane mutate updates config blob only, layers unchanged)
crane mutate ${IMAGE}:build \
  --label "io.llamastack.config.base64=$(crane export ${IMAGE}:build - | tar -xO app/config.yaml | base64 -w0)" \
  --label "io.llamastack.config.version=2" \
  -t ${IMAGE}:latest
```

**Why post-build**: Labels containing layer digests cannot be added during build (the layer digest is only known after build). `crane mutate` solves this by updating only the config blob without modifying layers.

**Adoption path**: OCI labels are optional in Phase 1. Distributions that adopt labels gain the ability to be used with `distribution.image` without `overrideConfig`. A future "LLS Distribution Image Specification" document will formalize the label contract.

### Key Entities

- **ProviderSpec**: Configuration for a single provider (inference, safety, vectorIo, etc.)
- **ResourceSpec**: Configuration for registered resources (models, tools, shields)
- **StorageSpec**: Configuration for state storage (kv and sql backends)
- **NetworkingSpec**: Configuration for network exposure (port, TLS, expose, allowedFrom)
- **WorkloadSpec**: Kubernetes deployment settings (replicas, resources, autoscaling)
- **ExposeConfig**: Expose configuration with `enabled` (bool) and `hostname` (string) fields
- **ResolvedDistributionStatus**: Tracks the resolved image reference, config source (embedded/oci-label), and config hash for change detection

## CRD Schema

### Complete v1alpha2 Spec Structure

```yaml
apiVersion: llamastack.io/v1alpha2
kind: LlamaStackDistribution
metadata:
  name: my-stack
spec:
  distribution:
    name: starter              # OR image: "registry/image:tag"

  providers:
    inference:
      - id: vllm-primary
        provider: vllm
        endpoint: "http://vllm:8000"
        apiKey:
          secretKeyRef: {name: vllm-creds, key: token}
        settings:
          max_tokens: 8192
    safety:
      - provider: llama-guard
    vectorIo:
      - provider: pgvector
        secretRefs:
          host:
            secretKeyRef: {name: pg-creds, key: host}

  resources:
    models:
      - name: "llama3.2-8b"
      - name: "llama3.2-70b"
        provider: vllm-primary
        contextLength: 128000
    tools:
      - websearch
      - rag
    shields:
      - llama-guard

  storage:
    kv:
      type: sqlite
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
      caBundle:
        configMapName: custom-ca
    expose:
      enabled: true
      # hostname: "llama.example.com"  # Optional: custom hostname
    allowedFrom:
      namespaces: ["app-ns"]
      labels: ["llama-access"]

  workload:
    replicas: 1
    workers: 2
    resources:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "2", memory: "4Gi"}
    autoscaling:
      minReplicas: 1
      maxReplicas: 5
      targetCPUUtilizationPercentage: 80
    storage:
      size: "10Gi"
      mountPath: "/.llama"
    podDisruptionBudget:
      maxUnavailable: 1
    topologySpreadConstraints: []
    overrides:
      serviceAccountName: "custom-sa"
      env: []
      command: []
      args: []
      volumes: []
      volumeMounts: []

  externalProviders:
    inference:
      - providerId: custom-inference
        image: registry.example.com/custom:v1

  # overrideConfig:
  #   configMapName: my-full-config
```

### Field Mapping: v1alpha1 to v1alpha2

| v1alpha1 Path | v1alpha2 Path |
|---------------|---------------|
| `spec.server.distribution` | `spec.distribution` |
| `spec.server.containerSpec.port` | `spec.networking.port` |
| `spec.server.containerSpec.resources` | `spec.workload.resources` |
| `spec.server.containerSpec.env` | `spec.workload.overrides.env` |
| `spec.server.containerSpec.command` | `spec.workload.overrides.command` |
| `spec.server.containerSpec.args` | `spec.workload.overrides.args` |
| `spec.server.userConfig` | `spec.overrideConfig` |
| `spec.server.storage` | `spec.workload.storage` |
| `spec.server.tlsConfig.caBundle` | `spec.networking.tls.caBundle` |
| `spec.server.autoscaling` | `spec.workload.autoscaling` |
| `spec.server.workers` | `spec.workload.workers` |
| `spec.server.podOverrides` | `spec.workload.overrides` |
| `spec.server.podDisruptionBudget` | `spec.workload.podDisruptionBudget` |
| `spec.server.topologySpreadConstraints` | `spec.workload.topologySpreadConstraints` |
| `spec.replicas` | `spec.workload.replicas` |
| `spec.network.exposeRoute` | `spec.networking.expose` |
| `spec.network.allowedFrom` | `spec.networking.allowedFrom` |
| *(new)* | `spec.providers` |
| *(new)* | `spec.resources` |
| *(new)* | `spec.storage` |
| *(new)* | `spec.disabled` |

## Controller Flow

```
┌─────────────────────────────────────────────────────────────────┐
│                      RECONCILIATION FLOW                        │
└─────────────────────────────────────────────────────────────────┘

1. Fetch LLSD CR
        │
        ▼
2. Resolve Distribution
   ├── distribution.name → lookup in distributions.json
   │   └── Check image-overrides in operator ConfigMap
   ├── distribution.image → use directly
   └── Record resolved image in status.resolvedDistribution
        │
        ▼
3. Validate Configuration (webhook + controller)
   ├── Check mutual exclusivity (providers vs overrideConfig)
   ├── Validate secret references exist
   ├── Validate ConfigMap references exist
   └── Validate provider ID references in resources
        │
        ▼
4. Determine Config Source
   ├── If overrideConfig: Use referenced ConfigMap directly
   └── If providers/resources: Generate config
        │
        ▼
5. Obtain Base Config
   ├── Check OCI labels on resolved image (Phase 2)
   ├── Fall back to embedded config for distribution name (Phase 1)
   └── Require overrideConfig if neither is available
        │
        ▼
6. Generate Configuration (if not using overrideConfig)
   ├── Merge user providers over base config providers
   ├── Expand resources to registered_resources format
   ├── Apply storage configuration
   ├── Apply disabled APIs
   ├── Resolve secretKeyRef to environment variables
   └── Validate generated config structure
        │
        ▼
7. Merge External Providers (from spec 001)
   ├── Add external providers to generated config
   └── Override on ID conflict (with warning)
        │
        ▼
8. Compare with Current State
   ├── Hash merged config against current ConfigMap
   ├── If identical → skip update, no Pod restart
   └── If different → proceed to update
        │
        ▼
9. Create ConfigMap + Update Deployment (atomic)
   ├── Create new ConfigMap with content hash in name
   ├── Set owner reference on ConfigMap
   ├── Update Deployment atomically:
   │   ├── Container image (from resolved distribution)
   │   ├── ConfigMap volume mount
   │   ├── Environment variables for secrets
   │   └── Hash annotation for rollout trigger
   └── On failure → preserve current Deployment, report error
        │
        ▼
10. Update Status
    ├── Set phase
    ├── Update conditions (ConfigGenerated, DeploymentUpdated,
    │   Available)
    ├── Record resolvedDistribution (image, configHash)
    └── Record config generation details
```

## Configuration Tiers

| Tier | Use Case | Mechanism | Example |
|------|----------|-----------|---------|
| 1 | Simple (80%) | Inline provider fields | `providers.inference: [{provider: vllm, endpoint: "..."}]` |
| 2 | Advanced (15%) | Per-provider settings | `providers.inference: [{..., settings: {max_tokens: 8192}}]` |
| 3 | Full Control (5%) | ConfigMap override | `overrideConfig: {configMapName: my-config}` |

## Status Reporting

### Printer Columns

The v1alpha2 CRD MUST define the following printer columns for `kubectl get` output (constitution §2.5):

```go
//+kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
//+kubebuilder:printcolumn:name="Distribution",type="string",JSONPath=".status.resolvedDistribution.image",priority=1
//+kubebuilder:printcolumn:name="Config",type="string",JSONPath=".status.configGeneration.configMapName",priority=1
//+kubebuilder:printcolumn:name="Providers",type="integer",JSONPath=".status.configGeneration.providerCount"
//+kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.availableReplicas"
//+kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"
```

Default `kubectl get llsd` output:

```
NAME       PHASE   PROVIDERS   AVAILABLE   AGE
my-stack   Ready   3           1           5m
```

Wide output (`kubectl get llsd -o wide`) includes `priority=1` columns (Distribution, Config).

### Status Fields

The status MUST include:

```yaml
status:
  phase: Ready
  conditions:
    - type: ConfigGenerated
      status: "True"
      reason: ConfigGenerationSucceeded
      message: "Generated config.yaml with 3 providers and 2 models"
    - type: DeploymentUpdated
      status: "True"
      reason: DeploymentUpdateSucceeded
      message: "Deployment updated with config my-stack-config-abc123"
    - type: Available
      status: "True"
      reason: MinimumReplicasAvailable
      message: "1/1 replicas available with current config"
    - type: SecretsResolved
      status: "True"
      reason: AllSecretsFound
      message: "Resolved 2 secret references"
  resolvedDistribution:
    image: "docker.io/llamastack/distribution-starter@sha256:abc123"
    configSource: embedded        # or "oci-label" (Phase 2)
    configHash: "sha256:def456"
  configGeneration:
    configMapName: my-stack-config-abc123
    generatedAt: "2026-02-02T12:00:00Z"
    providerCount: 3
    resourceCount: 2
```

## Security Considerations

- **Secret Handling**: Secret values MUST only be passed via environment variables, never embedded in ConfigMap
- **Environment Variable Naming**: Use deterministic, prefixed names: `LLSD_<PROVIDER_ID>_<FIELD>` (e.g., `LLSD_VLLM_PRIMARY_API_KEY`). Provider ID is uppercased with hyphens replaced by underscores.
- **ConfigMap Permissions**: Generated ConfigMaps inherit namespace RBAC
- **Image Extraction**: Config extraction from images uses read-only operations
- **Webhook Permissions**: The `ValidatingWebhookConfiguration` is a cluster-scoped resource, installed by OLM or kustomize during operator setup (not by the operator at runtime). This is a standard pattern for Kubernetes operators with admission webhooks and is an accepted deviation from constitution §1.1 (documented per constitution §Exceptions). The operator itself remains namespace-scoped at runtime.
- **Disabled + Provider Conflict**: Specifying providers for a disabled API type is a validation error, not a warning. This prevents contradictory config from being accepted and silently ignored.

## Clarifications

### Session 2026-03-10

- Q: How should the operator clean up old generated ConfigMaps during a CR's lifetime? → A: Controller deletes ConfigMaps beyond the last 2 during each reconciliation.
- Q: How should Phase 1 code handle the Phase 2 (OCI label) extension point? → A: Define a `ConfigResolver` interface now, implement `EmbeddedConfigResolver` for Phase 1. Phase 2 adds an `OCIConfigResolver` without refactoring.
- Q: Should the operator emit Kubernetes Events for config generation lifecycle? → A: Yes, emit Events for: config generated, deployment updated, config generation failed, secret resolution failed.
- Q: When should unknown distribution name be validated? → A: Webhook validates at admission time for instant feedback. The distribution registry is a static embedded resource, cheap to load in the webhook.

## Open Questions

- ~~**OQ-001**~~: Resolved. `expose: {}` is treated as `expose: true` (see Edge Cases).
- ~~**OQ-002**~~: Resolved. Disabled API + provider config conflict produces a **validation error** (not a warning). Accepting contradictory config and silently ignoring part of it leads to user confusion. The webhook rejects CRs where a provider type appears in both `providers` and `disabled`. (Originally resolved as warning in PR #242 review, changed to error after PR #253 review feedback from eoinfennessy)
- ~~**OQ-003**~~: Resolved. Environment variable naming uses the **provider ID** (not provider type or API type): `LLSD_<PROVIDER_ID>_<FIELD>`. The provider ID is unique across all providers (enforced by FR-072), ensuring no collisions. For single providers without explicit `id`, the auto-generated ID from FR-035 is used. Examples: `LLSD_VLLM_PRIMARY_API_KEY`, `LLSD_PGVECTOR_HOST`. Characters not valid in env var names (hyphens) are replaced with underscores and uppercased. (From PR #242 review)
- **OQ-004**: Should the operator create a default LlamaStackDistribution instance when installed? This is uncommon for Kubernetes operators but could improve the getting-started experience. If adopted, it should be opt-in via operator configuration (e.g., a Helm value or OLM parameter). (From team discussion, 2026-02-10)

## v1beta1 Folding Divergences (2026-04-29)

When the v1alpha2 types were folded into the `OGXServer` CRD (`ogx.io/v1beta1`) in [PR #289](https://github.com/ogx-ai/ogx-k8s-operator/pull/289), the following changes diverged from this spec. These reflect review feedback and improved API design decisions. See `specs/003-ogx-rename/research.md` (D-027 through D-038) for detailed rationale.

| This spec (v1alpha2) | v1beta1 OGXServer (actual) | Rationale |
|-----------------------|---------------------------|-----------|
| `spec.networking` | `spec.network` | Shorter, avoids conflation (D-026) |
| `spec.networking.tls.caBundle` nested under TLS | `spec.caBundle` top-level on OGXServerSpec | CA trust is a server-wide concern, not TLS-specific (D-027) |
| `spec.networking.tls.enabled` + `secretName` | `spec.network.tls` with presence semantics, `secretName` required | Avoids contradicting `enabled: false` + required `secretName` (D-028) |
| `spec.networking.allowedFrom` | `spec.network.policy` with native K8s types + `policyTypes` | Full NetworkPolicy power, avoids `network.networkPolicy` stutter (D-029) |
| `spec.networking.expose` (polymorphic, object or bool) | `spec.network.externalAccess` with explicit `enabled` (default false) | Clearer UX, mechanism-neutral naming (D-030) |
| `spec.disabled` | `spec.disabledAPIs` | Makes field purpose explicit (D-031) |
| `spec.externalProviders` | Removed | Design not yet finalized (D-032) |
| `providers.telemetry` | Removed | Provider type doesn't exist upstream (D-033) |
| `ProviderConfig.provider` — implicit `remote::` prefix | Must start with `remote::` or `inline::` (CEL-enforced) | Better UX, removes normalization (D-034) |
| `ProviderConfig.apiKey` (`*SecretKeyRef`) | `ProviderConfig.secretRefs` (`map[string]SecretKeyRef`) | Flexible named secret refs (D-035) |
| Per-slice CEL for provider ID uniqueness (FR-071) | Validating webhook for global ID uniqueness | Global uniqueness across all API types (D-036) |
| `CABundleConfig.configMapNamespace` | Removed | Same-namespace requirement for multi-tenancy (D-027) |
| `DefaultMountPath = "/.llama"` | `DefaultMountPath = "/.ogx"` | Consistent with OGX branding (D-038) |
| No label requirement on referenced ConfigMaps/Secrets | `ogx.io/watch: "true"` label required | Filtered informer cache (D-037) |
| FR-030: implicit `remote::` prefix normalization | Explicit prefix required, no normalization | Better UX (D-034) |
| FR-036: Telemetry provider | Removed | Doesn't exist (D-033) |

### Updated CRD Schema (v1beta1)

```yaml
apiVersion: ogx.io/v1beta1
kind: OGXServer
metadata:
  name: my-server
spec:
  distribution:
    name: starter              # OR image: "registry/image:tag"

  providers:
    inference:
      - id: vllm-primary
        provider: "remote::vllm"    # explicit prefix required
        endpoint: "http://vllm:8000"
        secretRefs:                  # replaces apiKey
          api-key:
            name: vllm-creds
            key: token
        settings:
          max_tokens: 8192
    safety:
      - provider: "remote::llama-guard"
    vectorIo:
      - provider: "remote::pgvector"
        secretRefs:
          host:
            name: pg-creds
            key: host
    # No telemetry provider (removed)

  resources:
    models:                          # typed []ModelConfig
      - name: "llama3.2-8b"
      - name: "llama3.2-70b"
        provider: vllm-primary
        contextLength: 128000
    tools:
      - websearch
      - rag
    shields:
      - llama-guard

  storage:
    kv:
      type: sqlite
    sql:
      type: postgres
      connectionString:
        name: pg-creds
        key: url

  disabledAPIs:                      # renamed from disabled; enum: agents, inference, tool_runtime, vector_io
    - agents

  caBundle:                          # top-level, not under tls
    configMapName: custom-ca
    # no configMapNamespace (same namespace required)

  network:                           # not networking
    port: 8321
    tls:                             # presence = enabled, no enabled bool
      secretName: my-tls-secret
    externalAccess:                  # not expose
      enabled: true
      hostname: "ogx.example.com"
    policy:                          # not networkPolicy
      enabled: true
      policyTypes:                   # follows K8s semantics
        - Ingress
        - Egress
      ingress:
        - from:
            - namespaceSelector:
                matchLabels:
                  app: frontend
      egress:
        - to:
            - namespaceSelector: {}

  workload:
    replicas: 1
    workers: 2
    resources:
      requests: {cpu: "500m", memory: "1Gi"}
      limits: {cpu: "2", memory: "4Gi"}
    autoscaling:
      minReplicas: 1
      maxReplicas: 5
      targetCPUUtilizationPercentage: 80
    storage:
      size: "10Gi"
      mountPath: "/.ogx"            # not /.llama
    podDisruptionBudget:
      maxUnavailable: 1
    overrides:
      serviceAccountName: "custom-sa"

  # No externalProviders (removed - design not finalized)

  # overrideConfig:
  #   configMapName: my-full-config
```

## References

- Design Document: [LlamaStackDistribution CRD v1alpha2 Schema Design](https://docs.google.com/document/d/10VhoQPb8bLGUo9yka4MXuEGIZClGf1oBr31TpK4NLD0/edit)
- Kubernetes API Conventions: https://github.com/kubernetes/community/blob/master/contributors/devel/sig-architecture/api-conventions.md
- Gateway API Design Guide: https://gateway-api.sigs.k8s.io/guides/api-design/
- Kubebuilder Good Practices: https://book.kubebuilder.io/reference/good-practices
- Related Spec: 001-deploy-time-providers-l1 (External Provider Injection)
