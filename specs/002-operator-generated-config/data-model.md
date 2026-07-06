# Data Model: Operator-Generated Server Configuration (v1alpha2)

**Spec**: 002-operator-generated-config
**Created**: 2026-02-10

## Entity Overview

```
LlamaStackDistribution (CR)
├── Spec
│   ├── DistributionSpec          # Image source (name or direct image)
│   ├── ProvidersSpec             # Provider configuration (typed slices per API type)
│   │   ├── Inference             # []ProviderConfig
│   │   ├── Safety                # []ProviderConfig
│   │   ├── VectorIo              # []ProviderConfig
│   │   ├── ToolRuntime           # []ProviderConfig
│   │   └── Telemetry             # []ProviderConfig
│   ├── ResourcesSpec             # Registered resources
│   │   ├── Models                # []ModelConfig
│   │   ├── Tools                 # string[]
│   │   └── Shields               # string[]
│   ├── StorageSpec               # State storage backends
│   │   ├── KV                    # KVStorageSpec
│   │   └── SQL                   # SQLStorageSpec
│   ├── Disabled                  # []string (API names)
│   ├── NetworkingSpec            # Network configuration
│   │   ├── Port                  # int32
│   │   ├── TLS                   # TLSSpec
│   │   ├── Expose                # ExposeConfig (enabled + hostname)
│   │   └── AllowedFrom           # AllowedFromSpec
│   ├── WorkloadSpec              # Deployment settings
│   │   ├── Replicas              # *int32
│   │   ├── Workers               # *int32
│   │   ├── Resources             # ResourceRequirements
│   │   ├── Autoscaling           # AutoscalingSpec
│   │   ├── Storage               # PVCStorageSpec
│   │   ├── PodDisruptionBudget   # PodDisruptionBudgetSpec
│   │   ├── TopologySpread        # []TopologySpreadConstraint
│   │   └── Overrides             # WorkloadOverrides
│   ├── ExternalProviders         # (from spec 001)
│   └── OverrideConfig            # OverrideConfigSpec (mutually exclusive with providers/resources/storage)
└── Status
    ├── Phase                     # DistributionPhase enum
    ├── Conditions                # []metav1.Condition
    ├── ResolvedDistribution      # ResolvedDistributionStatus
    ├── ConfigGeneration          # ConfigGenerationStatus
    ├── Version                   # VersionInfo (existing)
    ├── DistributionConfig        # DistributionConfig (existing)
    ├── AvailableReplicas         # int32 (existing)
    ├── ServiceURL                # string (existing)
    └── RouteURL                  # *string (existing)
```

---

## Entity Definitions

### DistributionSpec

**Purpose**: Identifies the LlamaStack distribution image to deploy.

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `name` | string | No | - | Mutually exclusive with `image` (CEL) | Distribution name (e.g., `starter`, `remote-vllm`) |
| `image` | string | No | - | Mutually exclusive with `name` (CEL) | Direct container image reference |

**Validation rules**:
- XValidation: `!(has(self.name) && has(self.image))` - Only one of name or image
- At least one of `name` or `image` must be specified (CEL or webhook)

**Relationships**:
- `name` resolves to image via `distributions.json` + `image-overrides`
- Resolved image recorded in `status.resolvedDistribution.image`

---

### ProviderConfig

**Purpose**: Configuration for a single LlamaStack provider instance.

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `id` | string | Conditional | Auto-generated from `provider` (FR-035) | Required when list has >1 element (FR-034, CEL) | Unique provider identifier |
| `provider` | string | Yes | - | Required | Provider type (e.g., `vllm`, `llama-guard`, `pgvector`) |
| `endpoint` | string | No | - | URL format | Provider endpoint URL |
| `apiKey` | *SecretKeyRef | No | - | - | Secret reference for API authentication |
| `secretRefs` | map[string]SecretKeyRef | No | - | - | Named secret references for provider-specific connection fields |
| `settings` | map[string]interface{} | No | - | Unstructured (escape hatch) | Provider-specific settings merged into config (NO secret resolution) |

**Mapping to config.yaml**:
- `provider` maps to `provider_type` with `remote::` prefix (FR-030)
- `endpoint` maps to `config.url` (FR-031)
- `apiKey` maps to `config.api_key` via env var `${env.LLSD_<PROVIDER_ID>_API_KEY}` (FR-032)
- `secretRefs.<key>` maps to `config.<key>` via env var `${env.LLSD_<PROVIDER_ID>_<KEY>}` (FR-032)
- `settings.*` merged into `config.*` (FR-033), passed through without secret resolution

---

### Provider Lists

**Purpose**: Each provider API type field is a typed `[]ProviderConfig` slice. A single provider is expressed as a one-element list. This provides kubebuilder validation, IDE autocompletion, and CEL inspection support.

| Scenario | Example | ID Requirement |
|----------|---------|----------------|
| Single provider | `inference: [{provider: vllm, endpoint: "..."}]` | Optional (auto-generated from `provider`) |
| Multiple providers | `inference: [{id: primary, provider: vllm, ...}, {id: fallback, ...}]` | Required on each item |

**Validation rules (CEL)**:
- When list has >1 element: each item MUST have explicit `id` (FR-034)
- All provider IDs MUST be unique across all API types (FR-072)
- No provider type may appear in both `providers` and `disabled` (FR-079d)

---

### SecretKeyRef

**Purpose**: Reference to a specific key in a Kubernetes Secret.

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `name` | string | Yes | - | Must reference existing Secret (webhook) | Secret name |
| `key` | string | Yes | - | - | Key within the Secret |

**Resolution**: At config generation time, each SecretKeyRef is converted to:
1. An environment variable definition: `{name: LLSD_<ID>_<FIELD>, valueFrom: {secretKeyRef: {name, key}}}`
2. A config.yaml reference: `${env.LLSD_<ID>_<FIELD>}`

---

### ResourcesSpec

**Purpose**: Declarative registration of models, tools, and shields.

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `models` | []ModelConfig | No | - | - | Models to register |
| `tools` | []string | No | - | - | Tool groups to register |
| `shields` | []string | No | - | - | Safety shields to register |

---

### ModelConfig

**Purpose**: Model registration with optional provider assignment and metadata.

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `name` | string | Yes | - | kubebuilder Required | Model identifier (e.g., `llama3.2-8b`) |
| `provider` | string | No | First inference provider | Must reference valid provider ID (webhook) | Provider ID for this model |
| `contextLength` | int | No | - | - | Model context window size |
| `modelType` | string | No | - | - | Model type classification |
| `quantization` | string | No | - | - | Quantization method |

**Usage**: Always specified as a typed struct. For simple model references, only `name` is required (e.g., `{name: "llama3.2-8b"}`). The first inference provider is used when `provider` is omitted.

---

### StorageSpec

**Purpose**: State storage backend configuration.

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `kv` | *KVStorageSpec | No | - | - | Key-value storage backend |
| `sql` | *SQLStorageSpec | No | - | - | Relational storage backend |

**When not specified**: Distribution defaults are preserved (no override).

---

### KVStorageSpec

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `type` | string | No | `sqlite` | Enum: `sqlite`, `redis` | Storage backend type |
| `endpoint` | string | Conditional | - | Required for `redis` | Redis endpoint URL |
| `password` | *SecretKeyRef | No | - | - | Redis authentication |

---

### SQLStorageSpec

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `type` | string | No | `sqlite` | Enum: `sqlite`, `postgres` | Storage backend type |
| `connectionString` | *SecretKeyRef | Conditional | - | Required for `postgres` | Database connection string |

---

### NetworkingSpec

**Purpose**: Network configuration for the LlamaStack service.

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `port` | int32 | No | 8321 | - | Server listen port |
| `tls` | *TLSSpec | No | - | - | TLS configuration |
| `expose` | *ExposeConfig | No | - | Polymorphic (bool or object) | External access configuration |
| `allowedFrom` | *AllowedFromSpec | No | - | - | Namespace-based access control |

---

### ExposeConfig

**Purpose**: Controls external service exposure via Ingress/Route.

| Field | Type | Required | Default | Description |
|-------|------|----------|---------|-------------|
| `enabled` | *bool | No | false | Enable external access via Ingress/Route |
| `hostname` | string | No | auto-generated | Custom hostname for Ingress/Route |

**Behavior**:
- `expose: {enabled: true}`: Create Ingress/Route with auto-generated hostname
- `expose: {enabled: true, hostname: "llama.example.com"}`: Create with specified hostname
- `expose: {}`: Treated as enabled with defaults (presence implies intent)
- Not specified: No external access

---

### TLSSpec

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `enabled` | bool | No | false | - | Enable TLS on server |
| `secretName` | string | Conditional | - | Required when `enabled: true` | K8s TLS Secret name |
| `caBundle` | *CABundleConfig | No | - | - | Custom CA certificates |

---

### WorkloadSpec

**Purpose**: Kubernetes Deployment settings (consolidates v1alpha1's scattered fields).

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `replicas` | *int32 | No | 1 | - | Pod replica count |
| `workers` | *int32 | No | - | Min: 1 | Uvicorn worker processes |
| `resources` | *ResourceRequirements | No | Defaults from constants | - | CPU/memory requests and limits |
| `autoscaling` | *AutoscalingSpec | No | - | - | HPA configuration |
| `storage` | *PVCStorageSpec | No | - | - | PVC for persistent data |
| `podDisruptionBudget` | *PodDisruptionBudgetSpec | No | - | - | PDB configuration |
| `topologySpreadConstraints` | []TopologySpreadConstraint | No | - | - | Pod spreading rules |
| `overrides` | *WorkloadOverrides | No | - | - | Low-level Pod overrides |

---

### WorkloadOverrides

| Field | Type | Description |
|-------|------|-------------|
| `serviceAccountName` | string | Custom ServiceAccount |
| `env` | []EnvVar | Additional environment variables |
| `command` | []string | Override container command |
| `args` | []string | Override container arguments |
| `volumes` | []Volume | Additional volumes |
| `volumeMounts` | []VolumeMount | Additional volume mounts |

---

### OverrideConfigSpec

**Purpose**: Full config.yaml override via user-provided ConfigMap (Tier 3 escape hatch).

| Field | Type | Required | Default | Validation | Description |
|-------|------|----------|---------|------------|-------------|
| `configMapName` | string | Yes | - | Must reference existing ConfigMap in the same namespace as the CR (webhook) | ConfigMap containing config.yaml |

**Mutual exclusivity** (CEL): Cannot be specified alongside `providers`, `resources`, `storage`, or `disabled`.
**Namespace scoping**: The referenced ConfigMap MUST reside in the same namespace as the LLSD CR (consistent with namespace-scoped RBAC, constitution section 1.1).

---

## Status Entities

### ResolvedDistributionStatus (new)

| Field | Type | Description |
|-------|------|-------------|
| `image` | string | Resolved container image reference (with digest when available) |
| `configSource` | string | Config origin: `embedded` or `oci-label` |
| `configHash` | string | SHA256 hash of the base config used |

### ConfigGenerationStatus (new)

| Field | Type | Description |
|-------|------|-------------|
| `configMapName` | string | Name of the generated ConfigMap |
| `generatedAt` | metav1.Time | Timestamp of last generation |
| `providerCount` | int | Number of configured providers |
| `resourceCount` | int | Number of registered resources |
| `configVersion` | int | Config.yaml schema version |

### Status Conditions (new)

| Type | True Reason | False Reason | Description |
|------|-------------|--------------|-------------|
| `ConfigGenerated` | `ConfigGenerationSucceeded` | `ConfigGenerationFailed` | Config.yaml generated successfully |
| `DeploymentUpdated` | `DeploymentUpdateSucceeded` | `DeploymentUpdateFailed` | Deployment spec reflects current config |
| `Available` | `MinimumReplicasAvailable` | `ReplicasUnavailable` | At least one Pod ready with current config |
| `SecretsResolved` | `AllSecretsFound` | `SecretNotFound` | All secretKeyRef references valid |

---

## State Transitions

### CR Lifecycle

```
                    ┌─────────┐
              ┌─────│ Pending │◄────── CR Created
              │     └────┬────┘
              │          │ Distribution resolved
              │     ┌────▼─────────┐
              │     │ Initializing │◄── Config generated, Deployment created
              │     └────┬─────────┘
              │          │ Pods ready
              │     ┌────▼───┐
              │     │ Ready  │◄── Normal operating state
              │     └────┬───┘
              │          │ Config update / error
              │     ┌────▼───┐
              ├────►│ Failed │◄── Config generation error / validation failure
              │     └────┬───┘
              │          │ User fixes CR
              │          └──── Back to Initializing
              │
              │     ┌──────────────┐
              └────►│ Terminating  │◄── CR deleted
                    └──────────────┘
```

### Config Generation Flow

```
CR Spec Change
      │
      ▼
Resolve Distribution (name → image)
      │
      ▼
Determine Config Source
├── overrideConfig → Use referenced ConfigMap directly
├── providers/resources/storage → Generate config
└── none specified → Use distribution default config
      │
      ▼
Generate Config (if applicable)
├── Load base config (embedded or OCI label)
├── Merge user providers over base
├── Expand resources to registered_resources
├── Apply storage configuration
├── Apply disabled APIs
├── Resolve secretKeyRef to env vars
└── Validate generated config
      │
      ▼
Compare Hash with Current
├── Identical → No update (skip)
└── Different → Create new ConfigMap + Update Deployment atomically
      │
      ▼
Update Status Conditions
```

---

## Relationship Map

```
distributions.json ──────► DistributionSpec.name ──► Resolved Image
                                                          │
image-overrides (ConfigMap) ─────────────────────────────┘
                                                          │
                                           ┌──────────────▼──────────────┐
                                           │  Base Config (embedded/OCI) │
                                           └──────────────┬──────────────┘
                                                          │
ProvidersSpec ─────────────────────────────────────────────┼──► Generated config.yaml
ResourcesSpec (models reference provider IDs) ────────────┤
StorageSpec ──────────────────────────────────────────────┤
Disabled ─────────────────────────────────────────────────┘
                                                          │
                                           ┌──────────────▼──────────────┐
                                           │  ConfigMap (hash-named)     │
                                           └──────────────┬──────────────┘
                                                          │
SecretKeyRef ──► EnvVar definitions ──────────────────────┼──► Deployment
NetworkingSpec ──► Ingress/Route + NetworkPolicy           │
WorkloadSpec ──► Deployment settings ─────────────────────┘
ExternalProviders (spec 001) ──► Merged into config ──────┘
```

---

## v1alpha1 to v1alpha2 Field Migration

| v1alpha1 Entity | v1alpha2 Entity | Transformation |
|-----------------|-----------------|----------------|
| `LlamaStackDistributionSpec.Replicas` | `WorkloadSpec.Replicas` | Direct move |
| `ServerSpec.Distribution` | `DistributionSpec` | Direct move |
| `ContainerSpec.Port` | `NetworkingSpec.Port` | Direct move |
| `ContainerSpec.Resources` | `WorkloadSpec.Resources` | Direct move |
| `ContainerSpec.Env` | `WorkloadOverrides.Env` | Direct move |
| `ContainerSpec.Command` | `WorkloadOverrides.Command` | Direct move |
| `ContainerSpec.Args` | `WorkloadOverrides.Args` | Direct move |
| `UserConfigSpec` | `OverrideConfigSpec` | Rename |
| `StorageSpec` (PVC) | `WorkloadSpec.Storage` (PVC) | Move (different from new StorageSpec for state backends) |
| `TLSConfig.CABundle` | `NetworkingSpec.TLS.CABundle` | Move into consolidated networking |
| `ServerSpec.Autoscaling` | `WorkloadSpec.Autoscaling` | Direct move |
| `ServerSpec.Workers` | `WorkloadSpec.Workers` | Direct move |
| `PodOverrides` | `WorkloadOverrides` | Rename + expand |
| `PodDisruptionBudgetSpec` | `WorkloadSpec.PodDisruptionBudget` | Direct move |
| `TopologySpreadConstraints` | `WorkloadSpec.TopologySpreadConstraints` | Direct move |
| `NetworkSpec.ExposeRoute` | `NetworkingSpec.Expose` | Bool to typed struct (enabled + hostname) |
| `NetworkSpec.AllowedFrom` | `NetworkingSpec.AllowedFrom` | Direct move |
| *(new)* | `ProvidersSpec` | New in v1alpha2 |
| *(new)* | `ResourcesSpec` | New in v1alpha2 |
| *(new)* | `StorageSpec` (state backends) | New in v1alpha2 |
| *(new)* | `Disabled` | New in v1alpha2 |
