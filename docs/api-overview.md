# API Reference

## Packages
- [llamastack.io/v1alpha1](#llamastackiov1alpha1)
- [ogx.io/v1beta1](#ogxiov1beta1)

## llamastack.io/v1alpha1

Package v1alpha1 contains API Schema definitions for the  v1alpha1 API group

### Resource Types
- [LlamaStackDistribution](#llamastackdistribution)
- [LlamaStackDistributionList](#llamastackdistributionlist)

#### AllowedFromSpec

AllowedFromSpec defines namespace-based access controls for NetworkPolicies.

_Appears in:_
- [NetworkSpec](#networkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `namespaces` _string array_ | Namespaces is an explicit list of namespace names allowed to access the service.<br />Use "*" to allow all namespaces. |  |  |
| `labels` _string array_ | Labels is a list of namespace label keys that are allowed to access the service.<br />A namespace matching any of these labels will be granted access (OR semantics).<br />Example: ["myproject/lls-allowed", "team/authorized"] |  |  |

#### AutoscalingSpec

AutoscalingSpec configures HorizontalPodAutoscaler targets.

_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minReplicas` _integer_ | MinReplicas is the lower bound replica count maintained by the HPA |  |  |
| `maxReplicas` _integer_ | MaxReplicas is the upper bound replica count maintained by the HPA |  |  |
| `targetCPUUtilizationPercentage` _integer_ | TargetCPUUtilizationPercentage configures CPU based scaling |  |  |
| `targetMemoryUtilizationPercentage` _integer_ | TargetMemoryUtilizationPercentage configures memory based scaling |  |  |

#### CABundleConfig

CABundleConfig defines the CA bundle configuration for custom certificates

_Appears in:_
- [TLSConfig](#tlsconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configMapName` _string_ | ConfigMapName is the name of the ConfigMap containing CA bundle certificates |  |  |
| `configMapNamespace` _string_ | ConfigMapNamespace is the namespace of the ConfigMap (defaults to the same namespace as the CR) |  |  |
| `configMapKeys` _string array_ | ConfigMapKeys specifies multiple keys within the ConfigMap containing CA bundle data<br />All certificates from these keys will be concatenated into a single CA bundle file<br />If not specified, defaults to [DefaultCABundleKey] |  | MaxItems: 50 <br /> |

#### ContainerSpec

ContainerSpec defines the llama-stack server container configuration.

_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ |  | llama-stack |  |
| `port` _integer_ |  |  |  |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#resourcerequirements-v1-core)_ |  |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envvar-v1-core) array_ |  |  |  |
| `command` _string array_ |  |  |  |
| `args` _string array_ |  |  |  |

#### DistributionConfig

DistributionConfig represents the configuration information from the providers endpoint.

_Appears in:_
- [LlamaStackDistributionStatus](#llamastackdistributionstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `activeDistribution` _string_ | ActiveDistribution shows which distribution is currently being used |  |  |
| `providers` _[ProviderInfo](#providerinfo) array_ |  |  |  |
| `availableDistributions` _object (keys:string, values:string)_ | AvailableDistributions lists all available distributions and their images |  |  |

#### DistributionPhase

_Underlying type:_ _string_

LlamaStackDistributionPhase represents the current phase of the LlamaStackDistribution

_Validation:_
- Enum: [Pending Initializing Ready Failed Terminating]

_Appears in:_
- [LlamaStackDistributionStatus](#llamastackdistributionstatus)

| Field | Description |
| --- | --- |
| `Pending` | LlamaStackDistributionPhasePending indicates that the distribution is pending initialization<br /> |
| `Initializing` | LlamaStackDistributionPhaseInitializing indicates that the distribution is being initialized<br /> |
| `Ready` | LlamaStackDistributionPhaseReady indicates that the distribution is ready to use<br /> |
| `Failed` | LlamaStackDistributionPhaseFailed indicates that the distribution has failed<br /> |
| `Terminating` | LlamaStackDistributionPhaseTerminating indicates that the distribution is being terminated<br /> |

#### DistributionType

DistributionType defines the distribution configuration for llama-stack.

_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the distribution name that maps to supported distributions. |  |  |
| `image` _string_ | Image is the direct container image reference to use |  |  |

#### LlamaStackDistribution

_Appears in:_
- [LlamaStackDistributionList](#llamastackdistributionlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `llamastack.io/v1alpha1` | | |
| `kind` _string_ | `LlamaStackDistribution` | | |
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[LlamaStackDistributionSpec](#llamastackdistributionspec)_ |  |  |  |
| `status` _[LlamaStackDistributionStatus](#llamastackdistributionstatus)_ |  |  |  |

#### LlamaStackDistributionList

LlamaStackDistributionList contains a list of LlamaStackDistribution.

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `llamastack.io/v1alpha1` | | |
| `kind` _string_ | `LlamaStackDistributionList` | | |
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[LlamaStackDistribution](#llamastackdistribution) array_ |  |  |  |

#### LlamaStackDistributionSpec

LlamaStackDistributionSpec defines the desired state of LlamaStackDistribution.

_Appears in:_
- [LlamaStackDistribution](#llamastackdistribution)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `replicas` _integer_ |  | 1 |  |
| `server` _[ServerSpec](#serverspec)_ |  |  |  |
| `network` _[NetworkSpec](#networkspec)_ | Network defines network access controls for the LlamaStack service |  |  |

#### LlamaStackDistributionStatus

LlamaStackDistributionStatus defines the observed state of LlamaStackDistribution.

_Appears in:_
- [LlamaStackDistribution](#llamastackdistribution)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[DistributionPhase](#distributionphase)_ | Phase represents the current phase of the distribution |  | Enum: [Pending Initializing Ready Failed Terminating] <br /> |
| `version` _[VersionInfo](#versioninfo)_ | Version contains version information for both operator and deployment |  |  |
| `distributionConfig` _[DistributionConfig](#distributionconfig)_ | DistributionConfig contains the configuration information from the providers endpoint |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations of the distribution's current state |  |  |
| `availableReplicas` _integer_ | AvailableReplicas is the number of available replicas |  |  |
| `serviceURL` _string_ | ServiceURL is the internal Kubernetes service URL where the distribution is exposed |  |  |
| `routeURL` _string_ | RouteURL is the external URL where the distribution is exposed (when exposeRoute is true).<br />nil when external access is not configured, empty string when Ingress exists but URL not ready. |  |  |

#### NetworkSpec

NetworkSpec defines network access controls for the LlamaStack service.

_Appears in:_
- [LlamaStackDistributionSpec](#llamastackdistributionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `exposeRoute` _boolean_ | ExposeRoute when true, creates an Ingress for external access.<br />Default is false (internal access only). | false |  |
| `allowedFrom` _[AllowedFromSpec](#allowedfromspec)_ | AllowedFrom defines which namespaces are allowed to access the LlamaStack service.<br />By default, only the LLSD namespace and the operator namespace are allowed. |  |  |

#### PodDisruptionBudgetSpec

PodDisruptionBudgetSpec defines voluntary disruption controls.

_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minAvailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#intorstring-intstr-util)_ | MinAvailable is the minimum number of pods that must remain available |  |  |
| `maxUnavailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#intorstring-intstr-util)_ | MaxUnavailable is the maximum number of pods that can be disrupted simultaneously |  |  |

#### PodOverrides

PodOverrides allows advanced pod-level customization.

_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serviceAccountName` _string_ | ServiceAccountName allows users to specify their own ServiceAccount<br />If not specified, the operator will use the default ServiceAccount |  |  |
| `terminationGracePeriodSeconds` _integer_ | TerminationGracePeriodSeconds is the time allowed for graceful pod shutdown.<br />If not specified, Kubernetes defaults to 30 seconds. |  |  |
| `volumes` _[Volume](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volume-v1-core) array_ |  |  |  |
| `volumeMounts` _[VolumeMount](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volumemount-v1-core) array_ |  |  |  |

#### ProviderHealthStatus

HealthStatus represents the health status of a provider

_Appears in:_
- [ProviderInfo](#providerinfo)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `status` _string_ |  |  |  |
| `message` _string_ |  |  |  |

#### ProviderInfo

ProviderInfo represents a single provider from the providers endpoint.

_Appears in:_
- [DistributionConfig](#distributionconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `api` _string_ |  |  |  |
| `provider_id` _string_ |  |  |  |
| `provider_type` _string_ |  |  |  |
| `config` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ |  |  |  |
| `health` _[ProviderHealthStatus](#providerhealthstatus)_ |  |  |  |

#### ServerSpec

ServerSpec defines the desired state of llama server.

_Appears in:_
- [LlamaStackDistributionSpec](#llamastackdistributionspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `distribution` _[DistributionType](#distributiontype)_ |  |  |  |
| `containerSpec` _[ContainerSpec](#containerspec)_ |  |  |  |
| `workers` _integer_ | Workers configures the number of uvicorn worker processes to run.<br />When set, the operator will launch llama-stack using uvicorn with the specified worker count.<br />Ref: https://fastapi.tiangolo.com/deployment/server-workers/<br />CPU requests are set to the number of workers when set, otherwise 1 full core |  | Minimum: 1 <br /> |
| `podOverrides` _[PodOverrides](#podoverrides)_ |  |  |  |
| `podDisruptionBudget` _[PodDisruptionBudgetSpec](#poddisruptionbudgetspec)_ | PodDisruptionBudget controls voluntary disruption tolerance for the server pods |  |  |
| `topologySpreadConstraints` _[TopologySpreadConstraint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#topologyspreadconstraint-v1-core) array_ | TopologySpreadConstraints defines fine-grained spreading rules |  |  |
| `autoscaling` _[AutoscalingSpec](#autoscalingspec)_ | Autoscaling configures HorizontalPodAutoscaler for the server pods |  |  |
| `storage` _[StorageSpec](#storagespec)_ | Storage defines the persistent storage configuration |  |  |
| `userConfig` _[UserConfigSpec](#userconfigspec)_ | UserConfig defines the user configuration for the llama-stack server |  |  |
| `tlsConfig` _[TLSConfig](#tlsconfig)_ | TLSConfig defines the TLS configuration for the llama-stack server |  |  |

#### StorageSpec

StorageSpec defines the persistent storage configuration

_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#quantity-resource-api)_ | Size is the size of the persistent volume claim created for holding persistent data of the llama-stack server |  |  |
| `mountPath` _string_ | MountPath is the path where the storage will be mounted in the container |  |  |

#### TLSConfig

TLSConfig defines the TLS configuration for the llama-stack server

_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `caBundle` _[CABundleConfig](#cabundleconfig)_ | CABundle defines the CA bundle configuration for custom certificates |  |  |

#### UserConfigSpec

_Appears in:_
- [ServerSpec](#serverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configMapName` _string_ | ConfigMapName is the name of the ConfigMap containing user configuration |  |  |
| `configMapNamespace` _string_ | ConfigMapNamespace is the namespace of the ConfigMap (defaults to the same namespace as the CR) |  |  |

#### VersionInfo

VersionInfo contains version-related information

_Appears in:_
- [LlamaStackDistributionStatus](#llamastackdistributionstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `operatorVersion` _string_ | OperatorVersion is the version of the operator managing this distribution |  |  |
| `llamaStackServerVersion` _string_ | LlamaStackServerVersion is the version of the LlamaStack server |  |  |
| `lastUpdated` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | LastUpdated represents when the version information was last updated |  |  |

## ogx.io/v1beta1

Package v1beta1 contains API Schema definitions for the ogx.io v1beta1 API group.

### Resource Types
- [OGXServer](#ogxserver)
- [OGXServerList](#ogxserverlist)

#### AutoscalingSpec

AutoscalingSpec configures HorizontalPodAutoscaler targets.

_Appears in:_
- [WorkloadSpec](#workloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minReplicas` _integer_ | MinReplicas is the lower bound replica count. |  |  |
| `maxReplicas` _integer_ | MaxReplicas is the upper bound replica count. |  |  |
| `targetCPUUtilizationPercentage` _integer_ | TargetCPUUtilizationPercentage configures CPU based scaling. |  |  |
| `targetMemoryUtilizationPercentage` _integer_ | TargetMemoryUtilizationPercentage configures memory based scaling. |  |  |

#### CABundleConfig

CABundleConfig defines the CA bundle configuration for custom certificates.

_Appears in:_
- [TLSSpec](#tlsspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configMapName` _string_ | ConfigMapName is the name of the ConfigMap containing CA bundle certificates. |  |  |
| `configMapNamespace` _string_ | ConfigMapNamespace is the namespace of the ConfigMap (defaults to the CR namespace). |  |  |
| `configMapKeys` _string array_ | ConfigMapKeys specifies keys within the ConfigMap containing CA bundle data.<br />All certificates from these keys will be concatenated into a single CA bundle file. |  | MaxItems: 50 <br /> |

#### ConfigGenerationStatus

ConfigGenerationStatus reports the state of config generation.

_Appears in:_
- [OGXServerStatus](#ogxserverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the spec generation that was last processed. |  |  |
| `configMapName` _string_ | ConfigMapName is the name of the generated ConfigMap. |  |  |

#### DistributionConfig

DistributionConfig represents the configuration from the providers endpoint.

_Appears in:_
- [OGXServerStatus](#ogxserverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `activeDistribution` _string_ |  |  |  |
| `providers` _[ProviderInfo](#providerinfo) array_ |  |  |  |
| `availableDistributions` _object (keys:string, values:string)_ |  |  |  |

#### DistributionSpec

DistributionSpec defines the distribution configuration.
Exactly one of name or image must be specified.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the distribution name that maps to supported distributions. |  |  |
| `image` _string_ | Image is the direct container image reference to use. |  |  |

#### KVStorageSpec

KVStorageSpec configures key-value state storage.

_Appears in:_
- [StateStorageSpec](#statestoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the backend type: sqlite or redis. |  | Enum: [sqlite redis] <br /> |
| `endpoint` _string_ | Endpoint is the connection endpoint for remote backends. |  |  |
| `password` _[SecretKeyRef](#secretkeyref)_ | Password references a Secret key holding the backend password. |  |  |

#### NetworkPolicySpec

NetworkPolicySpec configures the operator-managed NetworkPolicy for this server.
When nil or enabled with no rules, the operator generates safe defaults:
ingress on the service port from same-namespace and operator-namespace; egress unrestricted.
When ingress or egress rules are explicitly provided, they are used verbatim.
When any egress rules are configured, a kube-dns egress rule is auto-injected.

_Appears in:_
- [NetworkSpec](#networkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether the operator manages a NetworkPolicy for this server.<br />Defaults to true. Set to false to disable NetworkPolicy creation entirely. | true |  |
| `ingress` _[NetworkPolicyIngressRule](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#networkpolicyingressrule-v1-networking) array_ | Ingress rules. When nil, the operator generates default ingress rules<br />(allow from same-namespace and operator-namespace on the service port).<br />When explicitly set, these rules are used verbatim. |  |  |
| `egress` _[NetworkPolicyEgressRule](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#networkpolicyegressrule-v1-networking) array_ | Egress rules. When nil, egress is unrestricted (no Egress policyType set).<br />When explicitly set, these rules are used and a kube-dns egress rule is<br />auto-injected to prevent DNS breakage. |  |  |

#### NetworkSpec

NetworkSpec defines network access controls for the OGXServer.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _integer_ | Port overrides the default container and service port. |  |  |
| `tls` _[TLSSpec](#tlsspec)_ | TLS configures TLS termination. |  |  |
| `expose` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ | Expose configures external access (e.g. Ingress). Polymorphic JSON for flexibility. |  |  |
| `networkPolicy` _[NetworkPolicySpec](#networkpolicyspec)_ | NetworkPolicy configures the operator-managed NetworkPolicy.<br />When nil, the operator creates a default NetworkPolicy with safe ingress rules. |  |  |

#### OGXServer

OGXServer is the Schema for the ogxservers API.

_Appears in:_
- [OGXServerList](#ogxserverlist)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `ogx.io/v1beta1` | | |
| `kind` _string_ | `OGXServer` | | |
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ObjectMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#objectmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `spec` _[OGXServerSpec](#ogxserverspec)_ |  |  |  |
| `status` _[OGXServerStatus](#ogxserverstatus)_ |  |  |  |

#### OGXServerList

OGXServerList contains a list of OGXServer.

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `apiVersion` _string_ | `ogx.io/v1beta1` | | |
| `kind` _string_ | `OGXServerList` | | |
| `kind` _string_ | Kind is a string value representing the REST resource this object represents.<br />Servers may infer this from the endpoint the client submits requests to.<br />Cannot be updated.<br />In CamelCase.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#types-kinds |  |  |
| `apiVersion` _string_ | APIVersion defines the versioned schema of this representation of an object.<br />Servers should convert recognized schemas to the latest internal value, and<br />may reject unrecognized values.<br />More info: https://git.k8s.io/community/contributors/devel/sig-architecture/api-conventions.md#resources |  |  |
| `metadata` _[ListMeta](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#listmeta-v1-meta)_ | Refer to Kubernetes API documentation for fields of `metadata`. |  |  |
| `items` _[OGXServer](#ogxserver) array_ |  |  |  |

#### OGXServerPhase

_Underlying type:_ _string_

OGXServerPhase represents the current phase of the OGXServer.

_Validation:_
- Enum: [Pending Initializing Ready Failed Terminating]

_Appears in:_
- [OGXServerStatus](#ogxserverstatus)

| Field | Description |
| --- | --- |
| `Pending` |  |
| `Initializing` |  |
| `Ready` |  |
| `Failed` |  |
| `Terminating` |  |

#### OGXServerSpec

OGXServerSpec defines the desired state of OGXServer.

_Appears in:_
- [OGXServer](#ogxserver)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `distribution` _[DistributionSpec](#distributionspec)_ | Distribution specifies which OGX distribution image to deploy. |  |  |
| `providers` _[ProvidersSpec](#providersspec)_ | Providers configures inference, safety, and other provider backends. |  |  |
| `resources` _[ResourcesSpec](#resourcesspec)_ | Resources defines models, tools, and shields to register. |  |  |
| `storage` _[StateStorageSpec](#statestoragespec)_ | Storage configures state storage backends (KV, SQL). |  |  |
| `disabled` _string array_ | Disabled lists API categories to disable. |  |  |
| `network` _[NetworkSpec](#networkspec)_ | Network defines network access controls. |  |  |
| `workload` _[WorkloadSpec](#workloadspec)_ | Workload defines deployment, scaling, and pod configuration. |  |  |
| `externalProviders` _[ProviderConfig](#providerconfig) array_ | ExternalProviders references external provider configurations. |  |  |
| `overrideConfig` _[OverrideConfigSpec](#overrideconfigspec)_ | OverrideConfig references a user-provided ConfigMap that replaces all generated config.<br />Mutually exclusive with providers, resources, storage, and disabled. |  |  |

#### OGXServerStatus

OGXServerStatus defines the observed state of OGXServer.

_Appears in:_
- [OGXServer](#ogxserver)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[OGXServerPhase](#ogxserverphase)_ | Phase represents the current phase of the server. |  | Enum: [Pending Initializing Ready Failed Terminating] <br /> |
| `version` _[VersionInfo](#versioninfo)_ | Version contains version information for both operator and server. |  |  |
| `distributionConfig` _[DistributionConfig](#distributionconfig)_ | DistributionConfig contains provider information from the running server. |  |  |
| `resolvedDistribution` _[ResolvedDistributionStatus](#resolveddistributionstatus)_ | ResolvedDistribution reports the resolved distribution image. |  |  |
| `configGeneration` _[ConfigGenerationStatus](#configgenerationstatus)_ | ConfigGeneration reports the state of config generation. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations of the server's state. |  |  |
| `availableReplicas` _integer_ | AvailableReplicas is the number of available replicas. |  |  |
| `serviceURL` _string_ | ServiceURL is the internal Kubernetes service URL. |  |  |
| `routeURL` _string_ | RouteURL is the external URL when expose is configured. |  |  |

#### OverrideConfigSpec

OverrideConfigSpec references a user-provided ConfigMap for full config control.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `configMapName` _string_ | ConfigMapName is the name of the ConfigMap containing the server configuration. |  |  |

#### PVCStorageSpec

PVCStorageSpec defines PVC size and mount path.

_Appears in:_
- [WorkloadSpec](#workloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#quantity-resource-api)_ | Size is the PVC storage request. |  |  |
| `mountPath` _string_ | MountPath is the path where the PVC is mounted in the container. |  |  |

#### PodDisruptionBudgetSpec

PodDisruptionBudgetSpec defines voluntary disruption controls.

_Appears in:_
- [WorkloadSpec](#workloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minAvailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#intorstring-intstr-util)_ | MinAvailable is the minimum number of pods that must remain available. |  |  |
| `maxUnavailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#intorstring-intstr-util)_ | MaxUnavailable is the maximum number of pods that can be disrupted simultaneously. |  |  |

#### ProviderConfig

ProviderConfig defines a single provider configuration.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is the unique identifier for this provider instance. |  |  |
| `provider` _string_ | Provider is the provider type (e.g. "remote::ollama"). |  |  |
| `endpoint` _string_ | Endpoint is the URL for remote providers. |  |  |
| `apiKey` _[SecretKeyRef](#secretkeyref)_ | APIKey references a Secret key holding the provider's API key. |  |  |
| `settings` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ | Settings holds provider-specific configuration. |  |  |

#### ProviderHealthStatus

ProviderHealthStatus represents the health status of a provider.

_Appears in:_
- [ProviderInfo](#providerinfo)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `status` _string_ |  |  |  |
| `message` _string_ |  |  |  |

#### ProviderInfo

ProviderInfo represents a single provider from the providers endpoint.

_Appears in:_
- [DistributionConfig](#distributionconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `api` _string_ |  |  |  |
| `provider_id` _string_ |  |  |  |
| `provider_type` _string_ |  |  |  |
| `config` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ |  |  |  |
| `health` _[ProviderHealthStatus](#providerhealthstatus)_ |  |  |  |

#### ProvidersSpec

ProvidersSpec groups provider configurations by API category.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `inference` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ | Inference providers. |  |  |
| `safety` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ | Safety providers. |  |  |
| `vectorIo` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ | VectorIO providers. |  |  |
| `toolRuntime` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ | ToolRuntime providers. |  |  |
| `telemetry` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ | Telemetry providers. |  |  |

#### ResolvedDistributionStatus

ResolvedDistributionStatus reports the resolved distribution image.

_Appears in:_
- [OGXServerStatus](#ogxserverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `image` _string_ | Image is the resolved container image reference. |  |  |
| `source` _string_ | Source indicates how the image was resolved (e.g. "name", "image"). |  |  |

#### ResourcesSpec

ResourcesSpec defines models, tools, and shields to register.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `models` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io) array_ | Models to register. Each element is a JSON object for polymorphic form. |  |  |
| `tools` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io) array_ | Tools to register. |  |  |
| `shields` _string array_ | Shields to register by name. |  |  |

#### SQLStorageSpec

SQLStorageSpec configures SQL state storage.

_Appears in:_
- [StateStorageSpec](#statestoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the backend type: sqlite or postgres. |  | Enum: [sqlite postgres] <br /> |
| `connectionString` _[SecretKeyRef](#secretkeyref)_ | ConnectionString references a Secret key holding the full connection string. |  |  |

#### SecretKeyRef

SecretKeyRef is a reference to a key in a Kubernetes Secret.

_Appears in:_
- [KVStorageSpec](#kvstoragespec)
- [ProviderConfig](#providerconfig)
- [SQLStorageSpec](#sqlstoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Secret. |  |  |
| `key` _string_ | Key is the key within the Secret. |  |  |

#### StateStorageSpec

StateStorageSpec groups key-value and SQL storage backends.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kv` _[KVStorageSpec](#kvstoragespec)_ | KV configures key-value storage. |  |  |
| `sql` _[SQLStorageSpec](#sqlstoragespec)_ | SQL configures SQL storage. |  |  |

#### TLSSpec

TLSSpec defines TLS configuration.

_Appears in:_
- [NetworkSpec](#networkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled toggles TLS termination. |  |  |
| `secretName` _string_ | SecretName references a TLS Secret. |  |  |
| `caBundle` _[CABundleConfig](#cabundleconfig)_ | CABundle defines the CA bundle configuration for custom certificates. |  |  |

#### VersionInfo

VersionInfo contains version-related information.

_Appears in:_
- [OGXServerStatus](#ogxserverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `operatorVersion` _string_ |  |  |  |
| `serverVersion` _string_ |  |  |  |
| `lastUpdated` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ |  |  |  |

#### WorkloadOverrides

WorkloadOverrides allows advanced pod-level customization.

_Appears in:_
- [WorkloadSpec](#workloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serviceAccountName` _string_ | ServiceAccountName allows users to specify their own ServiceAccount. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envvar-v1-core) array_ | Env specifies additional environment variables. |  |  |
| `command` _string array_ | Command overrides the container entrypoint. |  |  |
| `args` _string array_ | Args overrides the container arguments. |  |  |
| `volumes` _[Volume](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volume-v1-core) array_ | Volumes specifies additional volumes. |  |  |
| `volumeMounts` _[VolumeMount](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volumemount-v1-core) array_ | VolumeMounts specifies additional volume mounts. |  |  |

#### WorkloadSpec

WorkloadSpec defines deployment-level configuration.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `replicas` _integer_ | Replicas is the desired pod count. | 1 |  |
| `workers` _integer_ | Workers configures the number of uvicorn worker processes. |  | Minimum: 1 <br /> |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#resourcerequirements-v1-core)_ | Resources defines CPU/memory requests and limits. |  |  |
| `autoscaling` _[AutoscalingSpec](#autoscalingspec)_ | Autoscaling configures HPA for the server pods. |  |  |
| `storage` _[PVCStorageSpec](#pvcstoragespec)_ | Storage defines PVC configuration. |  |  |
| `podDisruptionBudget` _[PodDisruptionBudgetSpec](#poddisruptionbudgetspec)_ | PodDisruptionBudget controls voluntary disruption tolerance. |  |  |
| `topologySpreadConstraints` _[TopologySpreadConstraint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#topologyspreadconstraint-v1-core) array_ | TopologySpreadConstraints defines fine-grained spreading rules. |  |  |
| `overrides` _[WorkloadOverrides](#workloadoverrides)_ | Overrides allows pod-level customization. |  |  |
