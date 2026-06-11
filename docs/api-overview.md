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
| `terminationGracePeriodSeconds` _[int64](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#int64-v1-core)_ | TerminationGracePeriodSeconds is the time allowed for graceful pod shutdown.<br />If not specified, Kubernetes defaults to 30 seconds. |  |  |
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
| `minReplicas` _integer_ | MinReplicas is the lower bound replica count. |  | Minimum: 1 <br /> |
| `maxReplicas` _integer_ | MaxReplicas is the upper bound replica count. |  | Minimum: 1 <br />Required: \{\} <br /> |
| `targetCPUUtilizationPercentage` _integer_ | TargetCPUUtilizationPercentage configures CPU-based scaling. |  | Maximum: 100 <br />Minimum: 1 <br /> |
| `targetMemoryUtilizationPercentage` _integer_ | TargetMemoryUtilizationPercentage configures memory-based scaling. |  | Maximum: 100 <br />Minimum: 1 <br /> |

#### AzureProvider

AzureProvider configures a remote::azure inference provider instance.

_Appears in:_
- [InferenceRemoteProviders](#inferenceremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `allowedModels` _string array_ | AllowedModels restricts which models can be registered with this provider.<br />When empty, all models are allowed. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `refreshModels` _boolean_ | RefreshModels controls whether the provider periodically refreshes<br />its model list from the remote endpoint. |  |  |
| `network` _[NetworkConfig](#networkconfig)_ | Network configures network settings (TLS, proxy, timeouts, headers)<br />for the remote connection. |  |  |
| `endpoint` _string_ | Endpoint is the Azure API base URL<br />(e.g., https://your-resource-name.openai.azure.com/openai/v1). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `apiKey` _[SecretKeyRef](#secretkeyref)_ | APIKey is the authentication credential for the Azure provider.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | Required: \{\} <br /> |
| `apiVersion` _string_ | APIVersion is the Azure API version (e.g., 2024-12-01-preview). |  |  |
| `apiType` _string_ | APIType is the Azure API type (e.g., azure). |  |  |

#### BatchesInlineProviders

BatchesInlineProviders groups inline batches providers.

_Appears in:_
- [BatchesProvidersSpec](#batchesprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `reference` _[InlineReferenceProvider](#inlinereferenceprovider)_ |  |  |  |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### BatchesProvidersSpec

BatchesProvidersSpec configures batches providers.

_Appears in:_
- [ProvidersSpec](#providersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `remote` _[BatchesRemoteProviders](#batchesremoteproviders)_ |  |  |  |
| `inline` _[BatchesInlineProviders](#batchesinlineproviders)_ |  |  |  |

#### BatchesRemoteProviders

BatchesRemoteProviders groups remote batches providers.

_Appears in:_
- [BatchesProvidersSpec](#batchesprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### BedrockProvider

BedrockProvider configures a remote::bedrock inference provider instance.

_Appears in:_
- [InferenceRemoteProviders](#inferenceremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `allowedModels` _string array_ | AllowedModels restricts which models can be registered with this provider.<br />When empty, all models are allowed. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `refreshModels` _boolean_ | RefreshModels controls whether the provider periodically refreshes<br />its model list from the remote endpoint. |  |  |
| `network` _[NetworkConfig](#networkconfig)_ | Network configures network settings (TLS, proxy, timeouts, headers)<br />for the remote connection. |  |  |
| `region` _string_ | Region is the AWS region for the Bedrock Runtime endpoint. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `apiKey` _[SecretKeyRef](#secretkeyref)_ | APIKey is the authentication credential for the Bedrock provider.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `awsAccessKeyId` _[SecretKeyRef](#secretkeyref)_ | AWSAccessKeyID is the AWS access key to use.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `awsSecretAccessKey` _[SecretKeyRef](#secretkeyref)_ | AWSSecretAccessKey is the AWS secret access key to use.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `awsSessionToken` _[SecretKeyRef](#secretkeyref)_ | AWSSessionToken is the AWS session token to use.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `awsRoleArn` _string_ | AWSRoleArn is the AWS role ARN to assume. |  |  |
| `awsWebIdentityTokenFile` _string_ | AWSWebIdentityTokenFile is the path to the web identity token file. |  |  |
| `awsRoleSessionName` _string_ | AWSRoleSessionName is the session name to use when assuming a role. |  |  |
| `profileName` _string_ | ProfileName is the AWS profile name that contains credentials to use. |  |  |
| `totalMaxAttempts` _integer_ | TotalMaxAttempts is the maximum number of attempts for a single request,<br />including the initial attempt. |  | Minimum: 1 <br /> |
| `retryMode` _string_ | RetryMode is the type of retries to perform (e.g., standard, adaptive). |  |  |
| `connectTimeout` _integer_ | ConnectTimeout is the connection timeout in seconds. |  | Minimum: 1 <br /> |
| `readTimeout` _integer_ | ReadTimeout is the read timeout in seconds. |  | Minimum: 1 <br /> |
| `sessionTTL` _integer_ | SessionTTL is the time in seconds until a session expires. |  | Minimum: 1 <br /> |

#### BraveSearchProvider

BraveSearchProvider configures a remote::brave-search tool runtime provider.

_Appears in:_
- [ToolRuntimeRemoteProviders](#toolruntimeremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `apiKey` _[SecretKeyRef](#secretkeyref)_ | APIKey is the Brave Search API key.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | Required: \{\} <br /> |
| `maxResults` _integer_ | MaxResults is the maximum number of search results to return. |  | Minimum: 1 <br /> |

#### CompactionConfig

_Underlying type:_ _[struct{SummarizationPrompt string "json:\"summarizationPrompt,omitempty\""; SummaryPrefix string "json:\"summaryPrefix,omitempty\""; SummarizationModel string "json:\"summarizationModel,omitempty\""; DefaultCompactThreshold *int "json:\"defaultCompactThreshold,omitempty\""; TokenizerEncoding string "json:\"tokenizerEncoding,omitempty\""}](#struct{summarizationprompt-string-"json:\"summarizationprompt,omitempty\"";-summaryprefix-string-"json:\"summaryprefix,omitempty\"";-summarizationmodel-string-"json:\"summarizationmodel,omitempty\"";-defaultcompactthreshold-*int-"json:\"defaultcompactthreshold,omitempty\"";-tokenizerencoding-string-"json:\"tokenizerencoding,omitempty\""})_

CompactionConfig configures conversation compaction behavior for responses.

_Appears in:_
- [InlineBuiltinResponsesProvider](#inlinebuiltinresponsesprovider)

#### ConfigGenerationStatus

ConfigGenerationStatus tracks config generation details.

_Appears in:_
- [OGXServerStatus](#ogxserverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `observedGeneration` _integer_ | ObservedGeneration is the spec generation that was last processed. |  |  |
| `configMapName` _string_ | ConfigMapName is the name of the generated ConfigMap. |  |  |
| `generatedAt` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ | GeneratedAt is the timestamp of the last generation. |  |  |
| `providerCount` _integer_ | ProviderCount is the number of configured providers. |  |  |
| `resourceCount` _integer_ | ResourceCount is the number of registered resources. |  |  |
| `configVersion` _integer_ | ConfigVersion is the config.yaml schema version. |  |  |

#### ConfigMapKeyRef

ConfigMapKeyRef references a key within a ConfigMap.
The ConfigMap must be in the same namespace as the OGXServer and must have
the label ogx.io/watch: "true" to be detected by the operator's cache.

_Appears in:_
- [IdentityConfig](#identityconfig)
- [OGXServerSpec](#ogxserverspec)
- [TrustConfig](#trustconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the ConfigMap. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | Key is the key within the ConfigMap. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z0-9]([a-zA-Z0-9\-_.]*[a-zA-Z0-9])?$` <br />Required: \{\} <br /> |

#### CustomProvider

CustomProvider defines the configuration for a custom provider instance.

_Appears in:_
- [BatchesInlineProviders](#batchesinlineproviders)
- [BatchesRemoteProviders](#batchesremoteproviders)
- [FilesInlineProviders](#filesinlineproviders)
- [FilesRemoteProviders](#filesremoteproviders)
- [InferenceInlineProviders](#inferenceinlineproviders)
- [InferenceRemoteProviders](#inferenceremoteproviders)
- [ResponsesInlineProviders](#responsesinlineproviders)
- [ResponsesRemoteProviders](#responsesremoteproviders)
- [ToolRuntimeInlineProviders](#toolruntimeinlineproviders)
- [ToolRuntimeRemoteProviders](#toolruntimeremoteproviders)
- [VectorIOInlineProviders](#vectorioinlineproviders)
- [VectorIORemoteProviders](#vectorioremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `type` _string_ | Type is the provider type, specified with a "remote::" or "inline::"<br />prefix (e.g., "remote::llama-guard", "inline::my-provider"). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `secretRefs` _object (keys:string, values:[SecretKeyRef](#secretkeyref))_ | SecretRefs is a map of named secret references for provider-specific<br />connection fields (e.g., host, password). Each key becomes the env var<br />field suffix and maps to config.<key> with env var substitution.<br />Each Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | MinProperties: 1 <br /> |
| `settings` _[JSON](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#json-v1-apiextensions-k8s-io)_ | Settings contains provider-specific configuration merged into the<br />provider's config section in config.yaml. Passed through as-is<br />without any secret resolution. Use secretRefs for secret values. |  |  |

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

DistributionSpec identifies the OGX distribution image to deploy.
Exactly one of name or image must be specified.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the distribution name that maps to a supported distribution (e.g., "starter", "remote-vllm").<br />Resolved to a container image via distributions.json and image-overrides. |  |  |
| `image` _string_ | Image is a direct container image reference to use. |  |  |

#### ExternalAccessConfig

ExternalAccessConfig controls external service exposure.

_Appears in:_
- [NetworkSpec](#networkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether external access is created. | false |  |
| `hostname` _string_ | Hostname sets a custom hostname for the external endpoint.<br />When omitted, an auto-generated hostname is used. |  |  |

#### FilesInlineProviders

FilesInlineProviders groups inline files providers.

_Appears in:_
- [FilesProvidersSpec](#filesprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `localfs` _[InlineLocalFSProvider](#inlinelocalfsprovider)_ |  |  |  |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### FilesProvidersSpec

FilesProvidersSpec configures files providers.

_Appears in:_
- [ProvidersSpec](#providersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `remote` _[FilesRemoteProviders](#filesremoteproviders)_ |  |  |  |
| `inline` _[FilesInlineProviders](#filesinlineproviders)_ |  |  |  |

#### FilesRemoteProviders

FilesRemoteProviders groups remote files providers.

_Appears in:_
- [FilesProvidersSpec](#filesprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `s3` _[S3Provider](#s3provider)_ |  |  |  |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### IdentityConfig

IdentityConfig configures client certificate identity for mTLS authentication.

_Appears in:_
- [TLSClientConfig](#tlsclientconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `cert` _[ConfigMapKeyRef](#configmapkeyref)_ | Cert references a ConfigMap key containing the PEM-encoded TLS client certificate.<br />The ConfigMap must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | Required: \{\} <br /> |
| `key` _[SecretKeyRef](#secretkeyref)_ | Key references a Secret key containing the PEM-encoded TLS client private key.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | Required: \{\} <br /> |

#### InferenceInlineProviders

InferenceInlineProviders groups inline inference providers.

_Appears in:_
- [InferenceProvidersSpec](#inferenceprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### InferenceProvidersSpec

InferenceProvidersSpec configures inference providers.

_Appears in:_
- [ProvidersSpec](#providersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `remote` _[InferenceRemoteProviders](#inferenceremoteproviders)_ |  |  |  |
| `inline` _[InferenceInlineProviders](#inferenceinlineproviders)_ |  |  |  |

#### InferenceRemoteProviders

InferenceRemoteProviders groups remote inference providers.

_Appears in:_
- [InferenceProvidersSpec](#inferenceprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `vllm` _[VLLMProvider](#vllmprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `openai` _[OpenAIProvider](#openaiprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `azure` _[AzureProvider](#azureprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `bedrock` _[BedrockProvider](#bedrockprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `vertexai` _[VertexAIProvider](#vertexaiprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `watsonx` _[WatsonxProvider](#watsonxprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### InlineBuiltinResponsesProvider

InlineBuiltinResponsesProvider configures inline::builtin for responses.

_Appears in:_
- [ResponsesInlineProviders](#responsesinlineproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `vectorStoresConfig` _[VectorStoresConfig](#vectorstoresconfig)_ | VectorStoresConfig configures vector store behavior for file search<br />and retrieval-augmented generation. |  |  |
| `compactionConfig` _[CompactionConfig](#compactionconfig)_ | CompactionConfig configures conversation compaction behavior<br />and prompt templates. |  |  |

#### InlineFileSearchProvider

InlineFileSearchProvider configures inline::file-search.

_Appears in:_
- [ToolRuntimeInlineProviders](#toolruntimeinlineproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `vectorStoresConfig` _[VectorStoresConfig](#vectorstoresconfig)_ | VectorStoresConfig configures vector store behavior for file search. |  |  |

#### InlineLocalFSProvider

InlineLocalFSProvider configures inline::localfs.

_Appears in:_
- [FilesInlineProviders](#filesinlineproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `ttlSecs` _integer_ | TTLSecs is the time-to-live in seconds for uploaded files. |  | Minimum: 1 <br /> |

#### InlineReferenceProvider

InlineReferenceProvider configures inline::reference for batches.

_Appears in:_
- [BatchesInlineProviders](#batchesinlineproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `maxConcurrentBatches` _integer_ | MaxConcurrentBatches is the maximum number of concurrent batches<br />to process simultaneously. |  | Minimum: 1 <br /> |
| `maxConcurrentRequestsPerBatch` _integer_ | MaxConcurrentRequestsPerBatch is the maximum number of concurrent<br />requests to process per batch. |  | Minimum: 1 <br /> |

#### KVStorageSpec

KVStorageSpec configures the key-value storage backend.

_Appears in:_
- [StateStorageSpec](#statestoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the KV storage backend type. | sqlite | Enum: [sqlite redis] <br /> |
| `endpoint` _string_ | Endpoint is the Redis endpoint URL. Required when type is "redis". |  |  |
| `password` _[SecretKeyRef](#secretkeyref)_ | Password references a Secret for Redis authentication.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |

#### MilvusProvider

MilvusProvider configures a remote::milvus vector I/O provider instance.

_Appears in:_
- [VectorIORemoteProviders](#vectorioremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `uri` _string_ | URI is the URI of the Milvus server. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `token` _[SecretKeyRef](#secretkeyref)_ | Token is the authentication token for the Milvus server.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `consistencyLevel` _string_ | ConsistencyLevel is the consistency level of the Milvus server. |  |  |

#### ModelConfig

ModelConfig defines a model registration with optional provider assignment and metadata.

_Appears in:_
- [ResourcesSpec](#resourcesspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the model identifier (e.g., "llama3.2-8b"). |  | MinLength: 1 <br />Required: \{\} <br /> |
| `provider` _string_ | Provider is the ID of the provider to register this model with.<br />Defaults to the first inference provider when omitted. |  |  |
| `contextLength` _integer_ | ContextLength is the model context window size. |  |  |
| `modelType` _string_ | ModelType is the model type classification. |  |  |
| `quantization` _string_ | Quantization is the quantization method. |  |  |

#### ModelContextProtocolProvider

ModelContextProtocolProvider configures remote::model-context-protocol.

_Appears in:_
- [ToolRuntimeRemoteProviders](#toolruntimeremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |

#### NetworkPolicySpec

NetworkPolicySpec configures the operator-managed NetworkPolicy for this server.

Ingress is always enforced unless explicitly omitted from policyTypes.
The operator always includes default ingress rules (allow from same-namespace
and operator-namespace on the service port), merging them with any
user-specified rules.

Egress is unrestricted by default. It is only enforced when egress rules
are provided or "Egress" is explicitly included in policyTypes.
When any egress rules are configured, or when "Egress" is explicitly included in
policyTypes, a kube-dns egress rule is auto-injected to prevent DNS breakage.

_Appears in:_
- [NetworkSpec](#networkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `enabled` _boolean_ | Enabled controls whether the operator manages a NetworkPolicy for this server.<br />Defaults to true. Set to false to disable NetworkPolicy creation entirely. | true |  |
| `policyTypes` _[PolicyType](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#policytype-v1-networking) array_ | PolicyTypes specifies which policy directions are enforced.<br />Follows Kubernetes NetworkPolicy semantics: when omitted or empty,<br />Ingress is always included and Egress is included only if egress<br />rules are provided. |  | items:Enum: [Ingress Egress] <br /> |
| `ingress` _[NetworkPolicyIngressRule](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#networkpolicyingressrule-v1-networking) array_ | Ingress defines additional ingress rules, merged with operator defaults<br />(allow from same-namespace and operator-namespace on the service port). |  |  |
| `egress` _[NetworkPolicyEgressRule](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#networkpolicyegressrule-v1-networking) array_ | Egress rules. When non-empty, a kube-dns egress rule is auto-injected<br />to prevent DNS breakage. |  |  |

#### NetworkSpec

NetworkSpec defines network access controls for the OGXServer.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `port` _integer_ | Port is the server listen port. | 8321 | Maximum: 65535 <br />Minimum: 1 <br /> |
| `tls` _[TLSSpec](#tlsspec)_ | TLS configures optional TLS termination for the server.<br />When omitted, the server listens over plain HTTP. |  |  |
| `externalAccess` _[ExternalAccessConfig](#externalaccessconfig)_ | ExternalAccess controls external service exposure. |  |  |
| `policy` _[NetworkPolicySpec](#networkpolicyspec)_ | Policy configures the operator-managed NetworkPolicy.<br />When nil, the operator creates a default NetworkPolicy with safe ingress rules. |  |  |

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
| `distribution` _[DistributionSpec](#distributionspec)_ | Distribution identifies the OGX distribution to deploy. |  | Required: \{\} <br /> |
| `providers` _[ProvidersSpec](#providersspec)_ | Providers configures providers by API type.<br />Mutually exclusive with overrideConfig. |  |  |
| `resources` _[ResourcesSpec](#resourcesspec)_ | Resources declares models and tools to register.<br />Mutually exclusive with overrideConfig. |  |  |
| `storage` _[StateStorageSpec](#statestoragespec)_ | Storage configures state storage backends (KV and SQL).<br />Mutually exclusive with overrideConfig. |  |  |
| `disabledAPIs` _string array_ | DisabledAPIs lists API names to remove from the generated config.<br />Mutually exclusive with overrideConfig. |  | MaxItems: 6 <br />MinItems: 1 <br />items:Enum: [batches inference responses tool_runtime vector_io files] <br /> |
| `registryRefreshIntervalSeconds` _integer_ | RegistryRefreshIntervalSeconds configures how often the server refreshes<br />its model registry, in seconds. When omitted, the server's built-in<br />default is used. |  | Minimum: 1 <br /> |
| `network` _[NetworkSpec](#networkspec)_ | Network defines network access controls. |  |  |
| `tls` _[TLSClientConfig](#tlsclientconfig)_ | TLS configures outbound TLS trust anchors and client identity for<br />connections to providers and backends. |  |  |
| `workload` _[WorkloadSpec](#workloadspec)_ | Workload consolidates Kubernetes deployment settings. |  |  |
| `overrideConfig` _[ConfigMapKeyRef](#configmapkeyref)_ | OverrideConfig references a ConfigMap key containing a full config.yaml override.<br />Mutually exclusive with providers, resources, storage, and disabledAPIs.<br />The ConfigMap must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |

#### OGXServerStatus

OGXServerStatus defines the observed state of OGXServer.

_Appears in:_
- [OGXServer](#ogxserver)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `phase` _[OGXServerPhase](#ogxserverphase)_ | Phase represents the current phase of the server. |  | Enum: [Pending Initializing Ready Failed Terminating] <br /> |
| `version` _[VersionInfo](#versioninfo)_ | Version contains version information for both operator and server. |  |  |
| `distributionConfig` _[DistributionConfig](#distributionconfig)_ | DistributionConfig contains provider information from the running server. |  |  |
| `resolvedDistribution` _[ResolvedDistributionStatus](#resolveddistributionstatus)_ | ResolvedDistribution tracks the resolved image and config source. |  |  |
| `configGeneration` _[ConfigGenerationStatus](#configgenerationstatus)_ | ConfigGeneration tracks config generation details. |  |  |
| `conditions` _[Condition](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#condition-v1-meta) array_ | Conditions represent the latest available observations of the server's state. |  |  |
| `availableReplicas` _integer_ | AvailableReplicas is the number of available replicas. |  |  |
| `serviceURL` _string_ | ServiceURL is the internal Kubernetes service URL. |  |  |
| `externalURL` _string_ | ExternalURL is the external URL when external access is configured. |  |  |

#### OpenAIProvider

OpenAIProvider configures a remote::openai inference provider instance.

_Appears in:_
- [InferenceRemoteProviders](#inferenceremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `allowedModels` _string array_ | AllowedModels restricts which models can be registered with this provider.<br />When empty, all models are allowed. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `refreshModels` _boolean_ | RefreshModels controls whether the provider periodically refreshes<br />its model list from the remote endpoint. |  |  |
| `network` _[NetworkConfig](#networkconfig)_ | Network configures network settings (TLS, proxy, timeouts, headers)<br />for the remote connection. |  |  |
| `endpoint` _string_ | Endpoint is the base URL for the OpenAI API. |  |  |
| `apiKey` _[SecretKeyRef](#secretkeyref)_ | APIKey is the authentication credential for the OpenAI provider.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | Required: \{\} <br /> |

#### PVCStorageSpec

PVCStorageSpec defines PVC storage for persistent data.

_Appears in:_
- [WorkloadSpec](#workloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `size` _[Quantity](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#quantity-resource-api)_ | Size is the size of the PVC. |  |  |
| `mountPath` _string_ | MountPath is the container mount path for the PVC. | /.ogx |  |

#### PgvectorProvider

PgvectorProvider configures a remote::pgvector vector I/O provider instance.

_Appears in:_
- [VectorIORemoteProviders](#vectorioremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `host` _string_ | Host is the PostgreSQL server hostname. |  |  |
| `port` _integer_ | Port is the PostgreSQL server port. |  | Maximum: 65535 <br />Minimum: 1 <br /> |
| `db` _string_ | DB is the PostgreSQL database name. |  |  |
| `user` _string_ | User is the PostgreSQL username. |  |  |
| `password` _[SecretKeyRef](#secretkeyref)_ | Password is the PostgreSQL password.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | Required: \{\} <br /> |
| `distanceMetric` _string_ | DistanceMetric is the distance metric used for vector search. |  | Enum: [COSINE L2 L1 INNER_PRODUCT] <br /> |
| `vectorIndex` _[VectorIndexConfig](#vectorindexconfig)_ | VectorIndex configures the vector index strategy for<br />Approximate Nearest Neighbor (ANN) search. |  |  |

#### PodDisruptionBudgetSpec

PodDisruptionBudgetSpec defines voluntary disruption controls.

_Appears in:_
- [WorkloadSpec](#workloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `minAvailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#intorstring-intstr-util)_ | MinAvailable is the minimum number of pods that must remain available. |  |  |
| `maxUnavailable` _[IntOrString](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#intorstring-intstr-util)_ | MaxUnavailable is the maximum number of pods that can be disrupted simultaneously. |  |  |

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

ProvidersSpec configures providers by API type.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `inference` _[InferenceProvidersSpec](#inferenceprovidersspec)_ |  |  |  |
| `vectorIo` _[VectorIOProvidersSpec](#vectorioprovidersspec)_ |  |  |  |
| `toolRuntime` _[ToolRuntimeProvidersSpec](#toolruntimeprovidersspec)_ |  |  |  |
| `files` _[FilesProvidersSpec](#filesprovidersspec)_ |  |  |  |
| `batches` _[BatchesProvidersSpec](#batchesprovidersspec)_ |  |  |  |
| `responses` _[ResponsesProvidersSpec](#responsesprovidersspec)_ |  |  |  |

#### ProxyConfig

ProxyConfig configures HTTP proxy settings for remote provider connections.

_Appears in:_
- [NetworkConfig](#networkconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `url` _string_ | URL is the proxy URL for all connections. |  |  |
| `http` _string_ | HTTP is the proxy URL for HTTP connections. |  |  |
| `https` _string_ | HTTPS is the proxy URL for HTTPS connections. |  |  |
| `cacert` _string_ | CACert is the path to a CA certificate for verifying the proxy's certificate. |  |  |
| `noProxy` _string array_ | NoProxy is a list of hosts that should bypass the proxy. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |

#### QdrantProvider

QdrantProvider configures a remote::qdrant vector I/O provider instance.

_Appears in:_
- [VectorIORemoteProviders](#vectorioremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `url` _string_ | URL is the URL of the Qdrant server. |  |  |
| `host` _string_ | Host is the hostname of the Qdrant server. |  |  |
| `port` _integer_ | Port is the REST API port of the Qdrant server. |  | Maximum: 65535 <br />Minimum: 1 <br /> |
| `apiKey` _[SecretKeyRef](#secretkeyref)_ | APIKey is the authentication key for the Qdrant server.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `location` _string_ | Location is the Qdrant server location identifier. |  |  |
| `grpcPort` _integer_ | GRPCPort is the gRPC port of the Qdrant server. |  | Maximum: 65535 <br />Minimum: 1 <br /> |
| `preferGrpc` _boolean_ | PreferGRPC controls whether to prefer gRPC over REST for communication. |  |  |
| `https` _boolean_ | HTTPS controls whether to use HTTPS for the connection. |  |  |
| `prefix` _string_ | Prefix is the URL path prefix for the Qdrant server. |  |  |
| `timeout` _integer_ | Timeout is the connection timeout in seconds. |  | Minimum: 1 <br /> |

#### QualifiedModel

QualifiedModel identifies a model with its provider.

_Appears in:_
- [ContextualRetrievalParams](#contextualretrievalparams)
- [RewriteQueryParams](#rewritequeryparams)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `providerId` _string_ | ProviderID is the provider to use for this model. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `modelId` _string_ | ModelID is the model identifier. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `embeddingDimensions` _integer_ | EmbeddingDimensions is the dimensionality of the embedding vectors. |  | Minimum: 1 <br /> |

#### RemoteInferenceCommonConfig

RemoteInferenceCommonConfig contains fields shared by all remote inference providers.

_Appears in:_
- [AzureProvider](#azureprovider)
- [BedrockProvider](#bedrockprovider)
- [OpenAIProvider](#openaiprovider)
- [VLLMProvider](#vllmprovider)
- [VertexAIProvider](#vertexaiprovider)
- [WatsonxProvider](#watsonxprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `allowedModels` _string array_ | AllowedModels restricts which models can be registered with this provider.<br />When empty, all models are allowed. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `refreshModels` _boolean_ | RefreshModels controls whether the provider periodically refreshes<br />its model list from the remote endpoint. |  |  |
| `network` _[NetworkConfig](#networkconfig)_ | Network configures network settings (TLS, proxy, timeouts, headers)<br />for the remote connection. |  |  |

#### ResolvedDistributionStatus

ResolvedDistributionStatus tracks the resolved distribution image for change detection.

_Appears in:_
- [OGXServerStatus](#ogxserverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `image` _string_ | Image is the resolved container image reference (with digest when available). |  |  |
| `configSource` _string_ | ConfigSource indicates the config origin: "embedded" or "oci-label". |  |  |
| `configHash` _string_ | ConfigHash is the SHA256 hash of the base config used. |  |  |

#### ResourcesSpec

ResourcesSpec defines declarative registration of models and tools.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `models` _[ModelConfig](#modelconfig) array_ | Models to register with inference providers. |  | MinItems: 1 <br /> |
| `tools` _string array_ | Tools are tool group names to register with the toolRuntime provider. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |

#### ResponsesInlineProviders

ResponsesInlineProviders groups inline responses providers.

_Appears in:_
- [ResponsesProvidersSpec](#responsesprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `builtin` _[InlineBuiltinResponsesProvider](#inlinebuiltinresponsesprovider)_ |  |  |  |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### ResponsesProvidersSpec

ResponsesProvidersSpec configures responses providers.

_Appears in:_
- [ProvidersSpec](#providersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `remote` _[ResponsesRemoteProviders](#responsesremoteproviders)_ |  |  |  |
| `inline` _[ResponsesInlineProviders](#responsesinlineproviders)_ |  |  |  |

#### ResponsesRemoteProviders

ResponsesRemoteProviders groups remote responses providers.

_Appears in:_
- [ResponsesProvidersSpec](#responsesprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### RoutedProviderBase

RoutedProviderBase contains fields common to all routed (non-singleton) provider instances.

_Appears in:_
- [AzureProvider](#azureprovider)
- [BedrockProvider](#bedrockprovider)
- [BraveSearchProvider](#bravesearchprovider)
- [CustomProvider](#customprovider)
- [InlineFileSearchProvider](#inlinefilesearchprovider)
- [MilvusProvider](#milvusprovider)
- [ModelContextProtocolProvider](#modelcontextprotocolprovider)
- [OpenAIProvider](#openaiprovider)
- [PgvectorProvider](#pgvectorprovider)
- [QdrantProvider](#qdrantprovider)
- [TavilySearchProvider](#tavilysearchprovider)
- [VLLMProvider](#vllmprovider)
- [VertexAIProvider](#vertexaiprovider)
- [WatsonxProvider](#watsonxprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |

#### S3Provider

S3Provider configures a remote::s3 files provider instance.

_Appears in:_
- [FilesRemoteProviders](#filesremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `bucketName` _string_ | BucketName is the S3 bucket name to store files. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `region` _string_ | Region is the AWS region where the bucket is located. |  |  |
| `awsAccessKeyId` _[SecretKeyRef](#secretkeyref)_ | AWSAccessKeyID is the AWS access key ID (optional if using IAM roles).<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `awsSecretAccessKey` _[SecretKeyRef](#secretkeyref)_ | AWSSecretAccessKey is the AWS secret access key (optional if using IAM roles).<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `endpointUrl` _string_ | EndpointURL is a custom S3 endpoint URL (for MinIO, LocalStack, etc.). |  |  |
| `autoCreateBucket` _boolean_ | AutoCreateBucket controls whether to automatically create the S3 bucket<br />if it doesn't exist. |  |  |

#### SQLStorageSpec

SQLStorageSpec configures the relational storage backend.

_Appears in:_
- [StateStorageSpec](#statestoragespec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `type` _string_ | Type is the SQL storage backend type. | sqlite | Enum: [sqlite postgres] <br /> |
| `connectionString` _[SecretKeyRef](#secretkeyref)_ | ConnectionString references a Secret containing the database connection string.<br />Required when type is "postgres".<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |

#### SecretKeyRef

SecretKeyRef references a specific key in a Kubernetes Secret.
The Secret must be in the same namespace as the OGXServer and must have
the label ogx.io/watch: "true" to be detected by the operator's cache.

_Appears in:_
- [AzureProvider](#azureprovider)
- [BedrockProvider](#bedrockprovider)
- [BraveSearchProvider](#bravesearchprovider)
- [CustomProvider](#customprovider)
- [IdentityConfig](#identityconfig)
- [KVStorageSpec](#kvstoragespec)
- [MilvusProvider](#milvusprovider)
- [OpenAIProvider](#openaiprovider)
- [PgvectorProvider](#pgvectorprovider)
- [QdrantProvider](#qdrantprovider)
- [S3Provider](#s3provider)
- [SQLStorageSpec](#sqlstoragespec)
- [TavilySearchProvider](#tavilysearchprovider)
- [VLLMProvider](#vllmprovider)
- [WatsonxProvider](#watsonxprovider)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `name` _string_ | Name is the name of the Kubernetes Secret. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `key` _string_ | Key is the key within the Secret. |  | MaxLength: 253 <br />MinLength: 1 <br />Pattern: `^[a-zA-Z0-9]([a-zA-Z0-9\-_.]*[a-zA-Z0-9])?$` <br />Required: \{\} <br /> |

#### StateStorageSpec

StateStorageSpec groups key-value and SQL storage backends.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `kv` _[KVStorageSpec](#kvstoragespec)_ | KV configures key-value storage. |  |  |
| `sql` _[SQLStorageSpec](#sqlstoragespec)_ | SQL configures SQL storage. |  |  |

#### TLSClientConfig

TLSClientConfig groups outbound TLS settings: trust anchors and client identity.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `trust` _[TrustConfig](#trustconfig)_ | Trust configures CA certificates for verifying outbound TLS connections<br />to providers and backends. |  |  |
| `identity` _[IdentityConfig](#identityconfig)_ | Identity configures client certificate and key for mTLS authentication<br />with providers and backends. |  |  |

#### TLSConfig

TLSConfig configures TLS settings for remote provider connections.

_Appears in:_
- [NetworkConfig](#networkconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `verify` _boolean_ | Verify controls whether TLS certificate verification is enabled.<br />Trust anchors and client identity are configured globally via spec.tls. |  |  |
| `minVersion` _string_ | MinVersion sets the minimum TLS version. |  | Enum: [TLSv1.2 TLSv1.3] <br /> |
| `ciphers` _string array_ | Ciphers is a list of allowed TLS cipher suites. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |

#### TLSSpec

TLSSpec defines TLS termination configuration for the server.

_Appears in:_
- [NetworkSpec](#networkspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `secretName` _string_ | SecretName references a Kubernetes TLS Secret containing a valid TLS certificate<br />for server TLS termination. The Secret must be in the same namespace as the<br />OGXServer and must have the label ogx.io/watch: "true" to be detected by the<br />operator's cache. |  | MinLength: 1 <br />Required: \{\} <br /> |

#### TavilySearchProvider

TavilySearchProvider configures a remote::tavily-search tool runtime provider.

_Appears in:_
- [ToolRuntimeRemoteProviders](#toolruntimeremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `apiKey` _[SecretKeyRef](#secretkeyref)_ | APIKey is the Tavily Search API key.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | Required: \{\} <br /> |
| `maxResults` _integer_ | MaxResults is the maximum number of search results to return. |  | Minimum: 1 <br /> |

#### TimeoutConfig

TimeoutConfig configures network timeout settings.

_Appears in:_
- [NetworkConfig](#networkconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `connect` _integer_ | Connect is the connection timeout in seconds. |  | Minimum: 1 <br /> |
| `read` _integer_ | Read is the read timeout in seconds. |  | Minimum: 1 <br /> |

#### ToolRuntimeInlineProviders

ToolRuntimeInlineProviders groups inline tool runtime providers.

_Appears in:_
- [ToolRuntimeProvidersSpec](#toolruntimeprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `fileSearch` _[InlineFileSearchProvider](#inlinefilesearchprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### ToolRuntimeProvidersSpec

ToolRuntimeProvidersSpec configures tool runtime providers.

_Appears in:_
- [ProvidersSpec](#providersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `remote` _[ToolRuntimeRemoteProviders](#toolruntimeremoteproviders)_ |  |  |  |
| `inline` _[ToolRuntimeInlineProviders](#toolruntimeinlineproviders)_ |  |  |  |

#### ToolRuntimeRemoteProviders

ToolRuntimeRemoteProviders groups remote tool runtime providers.

_Appears in:_
- [ToolRuntimeProvidersSpec](#toolruntimeprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `braveSearch` _[BraveSearchProvider](#bravesearchprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `tavilySearch` _[TavilySearchProvider](#tavilysearchprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `modelContextProtocol` _[ModelContextProtocolProvider](#modelcontextprotocolprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### TrustConfig

TrustConfig configures trust anchors for verifying outbound TLS connections.

_Appears in:_
- [TLSClientConfig](#tlsclientconfig)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `caCertificates` _[ConfigMapKeyRef](#configmapkeyref) array_ | CACertificates lists ConfigMap keys containing PEM-encoded CA certificates.<br />All certificates are concatenated into a single trust bundle.<br />Referenced ConfigMaps must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | MinItems: 1 <br /> |

#### VLLMProvider

VLLMProvider configures a remote::vllm inference provider instance.

_Appears in:_
- [InferenceRemoteProviders](#inferenceremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `allowedModels` _string array_ | AllowedModels restricts which models can be registered with this provider.<br />When empty, all models are allowed. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `refreshModels` _boolean_ | RefreshModels controls whether the provider periodically refreshes<br />its model list from the remote endpoint. |  |  |
| `network` _[NetworkConfig](#networkconfig)_ | Network configures network settings (TLS, proxy, timeouts, headers)<br />for the remote connection. |  |  |
| `endpoint` _string_ | Endpoint is the URL for the vLLM model serving endpoint. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `apiToken` _[SecretKeyRef](#secretkeyref)_ | APIToken is the authentication token for the vLLM endpoint.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  |  |
| `maxTokens` _integer_ | MaxTokens is the maximum number of tokens to generate. |  | Minimum: 1 <br /> |

#### VectorIOInlineProviders

VectorIOInlineProviders groups inline vector I/O providers.

_Appears in:_
- [VectorIOProvidersSpec](#vectorioprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### VectorIOProvidersSpec

VectorIOProvidersSpec configures vector I/O providers.

_Appears in:_
- [ProvidersSpec](#providersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `remote` _[VectorIORemoteProviders](#vectorioremoteproviders)_ |  |  |  |
| `inline` _[VectorIOInlineProviders](#vectorioinlineproviders)_ |  |  |  |

#### VectorIORemoteProviders

VectorIORemoteProviders groups remote vector I/O providers.

_Appears in:_
- [VectorIOProvidersSpec](#vectorioprovidersspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `pgvector` _[PgvectorProvider](#pgvectorprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `milvus` _[MilvusProvider](#milvusprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `qdrant` _[QdrantProvider](#qdrantprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |
| `custom` _[CustomProvider](#customprovider) array_ |  |  | MaxItems: 100 <br />MinItems: 1 <br /> |

#### VectorIndexConfig

_Underlying type:_ _[struct{HNSW *HNSWConfig "json:\"hnsw,omitempty\""; IVFFlat *IVFFlatConfig "json:\"ivfFlat,omitempty\""}](#struct{hnsw-*hnswconfig-"json:\"hnsw,omitempty\"";-ivfflat-*ivfflatconfig-"json:\"ivfflat,omitempty\""})_

VectorIndexConfig configures the vector index strategy for PGVector.
Exactly one of hnsw or ivfFlat must be specified.

_Appears in:_
- [PgvectorProvider](#pgvectorprovider)

#### VectorStoresConfig

_Underlying type:_ _[struct{DefaultProviderID string "json:\"defaultProviderId,omitempty\""; DefaultEmbeddingModel *QualifiedModel "json:\"defaultEmbeddingModel,omitempty\""; DefaultRerankerModel *RerankerModel "json:\"defaultRerankerModel,omitempty\""; RewriteQueryParams *RewriteQueryParams "json:\"rewriteQueryParams,omitempty\""; FileSearchParams *FileSearchDisplayParams "json:\"fileSearchParams,omitempty\""; ContextPromptParams *ContextPromptParams "json:\"contextPromptParams,omitempty\""; AnnotationPromptParams *AnnotationPromptParams "json:\"annotationPromptParams,omitempty\""; FileIngestionParams *FileIngestionParams "json:\"fileIngestionParams,omitempty\""; ChunkRetrievalParams *ChunkRetrievalParams "json:\"chunkRetrievalParams,omitempty\""; FileBatchParams *FileBatchParams "json:\"fileBatchParams,omitempty\""; ContextualRetrievalParams *ContextualRetrievalParams "json:\"contextualRetrievalParams,omitempty\""}](#struct{defaultproviderid-string-"json:\"defaultproviderid,omitempty\"";-defaultembeddingmodel-*qualifiedmodel-"json:\"defaultembeddingmodel,omitempty\"";-defaultrerankermodel-*rerankermodel-"json:\"defaultrerankermodel,omitempty\"";-rewritequeryparams-*rewritequeryparams-"json:\"rewritequeryparams,omitempty\"";-filesearchparams-*filesearchdisplayparams-"json:\"filesearchparams,omitempty\"";-contextpromptparams-*contextpromptparams-"json:\"contextpromptparams,omitempty\"";-annotationpromptparams-*annotationpromptparams-"json:\"annotationpromptparams,omitempty\"";-fileingestionparams-*fileingestionparams-"json:\"fileingestionparams,omitempty\"";-chunkretrievalparams-*chunkretrievalparams-"json:\"chunkretrievalparams,omitempty\"";-filebatchparams-*filebatchparams-"json:\"filebatchparams,omitempty\"";-contextualretrievalparams-*contextualretrievalparams-"json:\"contextualretrievalparams,omitempty\""})_

VectorStoresConfig configures vector store behavior for responses and file search.

_Appears in:_
- [InlineBuiltinResponsesProvider](#inlinebuiltinresponsesprovider)
- [InlineFileSearchProvider](#inlinefilesearchprovider)

#### VersionInfo

VersionInfo contains version-related information.

_Appears in:_
- [OGXServerStatus](#ogxserverstatus)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `operatorVersion` _string_ |  |  |  |
| `serverVersion` _string_ |  |  |  |
| `lastUpdated` _[Time](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#time-v1-meta)_ |  |  |  |

#### VertexAIProvider

VertexAIProvider configures a remote::vertexai inference provider instance.

_Appears in:_
- [InferenceRemoteProviders](#inferenceremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `allowedModels` _string array_ | AllowedModels restricts which models can be registered with this provider.<br />When empty, all models are allowed. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `refreshModels` _boolean_ | RefreshModels controls whether the provider periodically refreshes<br />its model list from the remote endpoint. |  |  |
| `network` _[NetworkConfig](#networkconfig)_ | Network configures network settings (TLS, proxy, timeouts, headers)<br />for the remote connection. |  |  |
| `project` _string_ | Project is the Google Cloud project ID for Vertex AI. |  | MinLength: 1 <br />Required: \{\} <br /> |
| `location` _string_ | Location is the Google Cloud location for Vertex AI. |  |  |

#### WatsonxProvider

WatsonxProvider configures a remote::watsonx inference provider instance.

_Appears in:_
- [InferenceRemoteProviders](#inferenceremoteproviders)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `id` _string_ | ID is a unique provider identifier. Derived from the provider<br />type when omitted. Must be unique across all providers. |  |  |
| `allowedModels` _string array_ | AllowedModels restricts which models can be registered with this provider.<br />When empty, all models are allowed. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `refreshModels` _boolean_ | RefreshModels controls whether the provider periodically refreshes<br />its model list from the remote endpoint. |  |  |
| `network` _[NetworkConfig](#networkconfig)_ | Network configures network settings (TLS, proxy, timeouts, headers)<br />for the remote connection. |  |  |
| `endpoint` _string_ | Endpoint is the base URL for accessing watsonx.ai. |  |  |
| `apiKey` _[SecretKeyRef](#secretkeyref)_ | APIKey is the authentication credential for the watsonx provider.<br />The Secret must be in the same namespace as the OGXServer<br />and must have the label ogx.io/watch: "true". |  | Required: \{\} <br /> |
| `projectId` _string_ | ProjectID is the watsonx.ai project ID. |  |  |
| `timeout` _integer_ | Timeout is the timeout in seconds for HTTP requests. |  | Minimum: 1 <br /> |

#### WorkloadOverrides

WorkloadOverrides allows low-level customization of the Pod template.

_Appears in:_
- [WorkloadSpec](#workloadspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `serviceAccountName` _string_ | ServiceAccountName specifies a custom ServiceAccount. |  |  |
| `env` _[EnvVar](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#envvar-v1-core) array_ | Env specifies additional environment variables. |  | MinItems: 1 <br /> |
| `command` _string array_ | Command overrides the container command. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `args` _string array_ | Args overrides the container arguments. |  | MinItems: 1 <br />items:MinLength: 1 <br /> |
| `volumes` _[Volume](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volume-v1-core) array_ | Volumes adds additional volumes to the Pod. |  | MinItems: 1 <br /> |
| `volumeMounts` _[VolumeMount](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#volumemount-v1-core) array_ | VolumeMounts adds additional volume mounts to the container. |  | MinItems: 1 <br /> |

#### WorkloadSpec

WorkloadSpec consolidates Kubernetes deployment settings.

_Appears in:_
- [OGXServerSpec](#ogxserverspec)

| Field | Description | Default | Validation |
| --- | --- | --- | --- |
| `replicas` _integer_ | Replicas is the desired Pod replica count. | 1 | Minimum: 0 <br /> |
| `workers` _integer_ | Workers configures the number of uvicorn worker processes. |  | Minimum: 1 <br /> |
| `resources` _[ResourceRequirements](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#resourcerequirements-v1-core)_ | Resources defines CPU/memory requests and limits. |  |  |
| `autoscaling` _[AutoscalingSpec](#autoscalingspec)_ | Autoscaling configures HPA for the server pods. |  |  |
| `storage` _[PVCStorageSpec](#pvcstoragespec)_ | Storage defines PVC configuration. |  |  |
| `podDisruptionBudget` _[PodDisruptionBudgetSpec](#poddisruptionbudgetspec)_ | PodDisruptionBudget controls voluntary disruption tolerance. |  |  |
| `topologySpreadConstraints` _[TopologySpreadConstraint](https://kubernetes.io/docs/reference/generated/kubernetes-api/v1.31/#topologyspreadconstraint-v1-core) array_ | TopologySpreadConstraints defines Pod spreading rules. |  | MinItems: 1 <br /> |
| `overrides` _[WorkloadOverrides](#workloadoverrides)_ | Overrides allows pod-level customization. |  |  |
