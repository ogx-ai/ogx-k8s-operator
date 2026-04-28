/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package v1beta1

//nolint:gci
import (
	"errors"
	"fmt"
	"regexp"

	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

const (
	// DefaultContainerName is the default name for the container.
	DefaultContainerName = "ogx"
	// DefaultServerPort is the default port for the server.
	DefaultServerPort int32 = 8321
	// DefaultServicePortName is the default name for the service port.
	DefaultServicePortName = "http"
	// DefaultLabelKey is the default key for labels.
	DefaultLabelKey = "app"
	// DefaultLabelValue is the default value for labels.
	DefaultLabelValue = "ogx"
	// DefaultMountPath is the default mount path for storage.
	DefaultMountPath = "/.ogx"
	// OGXServerKind is the kind name for OGXServer resources.
	OGXServerKind = "OGXServer"

	// AdoptStorageAnnotation triggers PVC adoption from a legacy LlamaStackDistribution.
	AdoptStorageAnnotation = "ogx.io/adopt-storage"
	// AdoptNetworkingAnnotation triggers Service/Ingress adoption from a legacy LlamaStackDistribution.
	AdoptNetworkingAnnotation = "ogx.io/adopt-networking"
	// AdoptedFromAnnotation is set on adopted child resources to record the legacy source.
	AdoptedFromAnnotation = "ogx.io/adopted-from"
	// AdoptedAtAnnotation is set on adopted child resources with an RFC 3339 timestamp.
	AdoptedAtAnnotation = "ogx.io/adopted-at"
)

var (
	// DefaultStorageSize is the default size for persistent storage.
	DefaultStorageSize = resource.MustParse("10Gi")
	// DefaultServerCPURequest ensures the HPA and scheduler have baseline values.
	DefaultServerCPURequest = resource.MustParse("500m")
	// DefaultServerMemoryRequest ensures the HPA and scheduler have baseline values.
	DefaultServerMemoryRequest = resource.MustParse("1Gi")

	// dns1123LabelMaxLen is the maximum length of an RFC 1123 DNS label.
	dns1123LabelMaxLen = 63
	// dns1123LabelRegex matches valid RFC 1123 DNS labels.
	dns1123LabelRegex = regexp.MustCompile(`^[a-z0-9]([a-z0-9\-]{0,61}[a-z0-9])?$`)
)

// DistributionSpec identifies the OGX distribution image to deploy.
// Exactly one of name or image must be specified.
// +kubebuilder:validation:XValidation:rule="!(has(self.name) && has(self.image))",message="only one of name or image can be specified"
// +kubebuilder:validation:XValidation:rule="has(self.name) || has(self.image)",message="one of name or image must be specified"
type DistributionSpec struct {
	// Name is the distribution name that maps to a supported distribution (e.g., "starter", "remote-vllm").
	// Resolved to a container image via distributions.json and image-overrides.
	// +optional
	Name string `json:"name,omitempty"`
	// Image is a direct container image reference to use.
	// +optional
	Image string `json:"image,omitempty"`
}

// SecretKeyRef references a specific key in a Kubernetes Secret.
type SecretKeyRef struct {
	// Name is the name of the Kubernetes Secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Key is the key within the Secret.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Key string `json:"key"`
}

// ProviderConfig defines the configuration for a single provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.id) || self.id.size() > 0",message="id must not be empty if specified"
type ProviderConfig struct {
	// ID is a unique provider identifier. Required when multiple providers are
	// configured for the same API type. Auto-generated from provider when omitted
	// for single-provider configurations.
	// +optional
	ID string `json:"id,omitempty"`
	// Provider is the provider type (e.g., "vllm", "llama-guard", "pgvector").
	// Maps to provider_type with "remote::" prefix in config.yaml.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Provider string `json:"provider"`
	// Endpoint is the provider endpoint URL. Maps to config.url in config.yaml.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// SecretRefs is a map of named secret references for provider-specific
	// connection fields (e.g., host, password). Each key becomes the env var
	// field suffix and maps to config.<key> with env var substitution.
	// Use this instead of embedding secretKeyRef inside settings.
	// +optional
	// +kubebuilder:validation:MinProperties=1
	SecretRefs map[string]SecretKeyRef `json:"secretRefs,omitempty"`
	// Settings contains provider-specific settings merged into the provider's
	// config section in config.yaml. Acts as an escape hatch for fields not
	// directly exposed in the CRD schema. Passed through as-is without any
	// secret resolution. Use secretRefs for secret values.
	// +optional
	Settings *apiextensionsv1.JSON `json:"settings,omitempty"`
}

// ProvidersSpec contains typed provider slices for each API type.
type ProvidersSpec struct {
	// Inference configures inference providers (e.g., vLLM, TGI).
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self.size() <= 1 || self.all(p, has(p.id))",message="each provider must have an explicit id when multiple providers are specified"
	Inference []ProviderConfig `json:"inference,omitempty"`
	// Safety configures safety providers (e.g., llama-guard).
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self.size() <= 1 || self.all(p, has(p.id))",message="each provider must have an explicit id when multiple providers are specified"
	Safety []ProviderConfig `json:"safety,omitempty"`
	// VectorIo configures vector I/O providers (e.g., pgvector, chromadb).
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self.size() <= 1 || self.all(p, has(p.id))",message="each provider must have an explicit id when multiple providers are specified"
	VectorIo []ProviderConfig `json:"vectorIo,omitempty"`
	// ToolRuntime configures tool runtime providers.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self.size() <= 1 || self.all(p, has(p.id))",message="each provider must have an explicit id when multiple providers are specified"
	ToolRuntime []ProviderConfig `json:"toolRuntime,omitempty"`
	// Telemetry configures telemetry providers (e.g., opentelemetry).
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:XValidation:rule="self.size() <= 1 || self.all(p, has(p.id))",message="each provider must have an explicit id when multiple providers are specified"
	Telemetry []ProviderConfig `json:"telemetry,omitempty"`
}

// ModelConfig defines a model registration with optional provider assignment and metadata.
// +kubebuilder:validation:XValidation:rule="!has(self.provider) || self.provider.size() > 0",message="provider must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.modelType) || self.modelType.size() > 0",message="modelType must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.quantization) || self.quantization.size() > 0",message="quantization must not be empty if specified"
type ModelConfig struct {
	// Name is the model identifier (e.g., "llama3.2-8b").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
	// Provider is the ID of the provider to register this model with.
	// Defaults to the first inference provider when omitted.
	// +optional
	Provider string `json:"provider,omitempty"`
	// ContextLength is the model context window size.
	// +optional
	ContextLength *int `json:"contextLength,omitempty"`
	// ModelType is the model type classification.
	// +optional
	ModelType string `json:"modelType,omitempty"`
	// Quantization is the quantization method.
	// +optional
	Quantization string `json:"quantization,omitempty"`
}

// ResourcesSpec defines declarative registration of models, tools, and shields.
type ResourcesSpec struct {
	// Models to register with inference providers.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Models []ModelConfig `json:"models,omitempty"`
	// Tools are tool group names to register with the toolRuntime provider.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	Tools []string `json:"tools,omitempty"`
	// Shields to register by name.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	Shields []string `json:"shields,omitempty"`
}

// KVStorageSpec configures the key-value storage backend.
// +kubebuilder:validation:XValidation:rule="self.type != 'redis' || has(self.endpoint)",message="endpoint is required when type is redis"
// +kubebuilder:validation:XValidation:rule="!has(self.endpoint) || self.type == 'redis'",message="endpoint is only valid when type is redis"
// +kubebuilder:validation:XValidation:rule="!has(self.password) || self.type == 'redis'",message="password is only valid when type is redis"
type KVStorageSpec struct {
	// Type is the KV storage backend type.
	// +kubebuilder:validation:Enum=sqlite;redis
	// +kubebuilder:default:="sqlite"
	// +optional
	Type string `json:"type,omitempty"`
	// Endpoint is the Redis endpoint URL. Required when type is "redis".
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// Password references a Secret for Redis authentication.
	// +optional
	Password *SecretKeyRef `json:"password,omitempty"`
}

// SQLStorageSpec configures the relational storage backend.
// +kubebuilder:validation:XValidation:rule="self.type != 'postgres' || has(self.connectionString)",message="connectionString is required when type is postgres"
// +kubebuilder:validation:XValidation:rule="!has(self.connectionString) || self.type == 'postgres'",message="connectionString is only valid when type is postgres"
type SQLStorageSpec struct {
	// Type is the SQL storage backend type.
	// +kubebuilder:validation:Enum=sqlite;postgres
	// +kubebuilder:default:="sqlite"
	// +optional
	Type string `json:"type,omitempty"`
	// ConnectionString references a Secret containing the database connection string.
	// Required when type is "postgres".
	// +optional
	ConnectionString *SecretKeyRef `json:"connectionString,omitempty"`
}

// StateStorageSpec groups key-value and SQL storage backends.
type StateStorageSpec struct {
	// KV configures key-value storage.
	// +optional
	KV *KVStorageSpec `json:"kv,omitempty"`
	// SQL configures SQL storage.
	// +optional
	SQL *SQLStorageSpec `json:"sql,omitempty"`
}

// CABundleConfig defines the CA bundle configuration for custom certificates.
type CABundleConfig struct {
	// ConfigMapName is the name of the ConfigMap containing CA bundle certificates.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConfigMapName string `json:"configMapName"`
	// ConfigMapNamespace is the namespace of the ConfigMap (defaults to the CR namespace).
	// +optional
	ConfigMapNamespace string `json:"configMapNamespace,omitempty"`
	// ConfigMapKeys specifies keys within the ConfigMap containing CA bundle data.
	// All certificates from these keys will be concatenated into a single CA bundle file.
	// +optional
	// +kubebuilder:validation:MaxItems=50
	// +kubebuilder:validation:Items:Pattern="^[a-zA-Z0-9]([a-zA-Z0-9\\-_.]*[a-zA-Z0-9])?$"
	// +kubebuilder:validation:Items:MaxLength=253
	ConfigMapKeys []string `json:"configMapKeys,omitempty"`
}

// TLSSpec defines TLS termination configuration for the server.
type TLSSpec struct {
	// SecretName references a Kubernetes TLS Secret containing a valid TLS certificate
	// for server TLS termination.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	SecretName string `json:"secretName"`
}

// NetworkPolicySpec configures the operator-managed NetworkPolicy for this server.
//
// Ingress is always enforced unless explicitly omitted from policyTypes.
// The operator always includes default ingress rules (allow from same-namespace
// and operator-namespace on the service port), merging them with any
// user-specified rules.
//
// Egress is unrestricted by default. It is only enforced when egress rules
// are provided or "Egress" is explicitly included in policyTypes.
// When any egress rules are configured, or when "Egress" is explicitly included in
// policyTypes, a kube-dns egress rule is auto-injected to prevent DNS breakage.
type NetworkPolicySpec struct {
	// Enabled controls whether the operator manages a NetworkPolicy for this server.
	// Defaults to true. Set to false to disable NetworkPolicy creation entirely.
	// +optional
	// +kubebuilder:default:=true
	Enabled *bool `json:"enabled,omitempty"`
	// PolicyTypes specifies which policy directions are enforced.
	// Follows Kubernetes NetworkPolicy semantics: when omitted or empty,
	// Ingress is always included and Egress is included only if egress
	// rules are provided.
	// +optional
	// +kubebuilder:validation:items:Enum=Ingress;Egress
	PolicyTypes []networkingv1.PolicyType `json:"policyTypes,omitempty"`
	// Ingress defines additional ingress rules, merged with operator defaults
	// (allow from same-namespace and operator-namespace on the service port).
	// +optional
	Ingress []networkingv1.NetworkPolicyIngressRule `json:"ingress,omitempty"`
	// Egress rules. When non-empty, a kube-dns egress rule is auto-injected
	// to prevent DNS breakage.
	// +optional
	Egress []networkingv1.NetworkPolicyEgressRule `json:"egress,omitempty"`
}

// ExternalAccessConfig controls external service exposure.
// +kubebuilder:validation:XValidation:rule="!has(self.hostname) || self.hostname.size() > 0",message="hostname must not be empty if specified"
type ExternalAccessConfig struct {
	// Enabled controls whether external access is created.
	// +optional
	// +kubebuilder:default:=false
	Enabled bool `json:"enabled,omitempty"`
	// Hostname sets a custom hostname for the external endpoint.
	// When omitted, an auto-generated hostname is used.
	// +optional
	Hostname string `json:"hostname,omitempty"`
}

// NetworkSpec defines network access controls for the OGXServer.
type NetworkSpec struct {
	// Port is the server listen port.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default:=8321
	Port int32 `json:"port,omitempty"`
	// TLS configures optional TLS termination for the server.
	// When omitted, the server listens over plain HTTP.
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`
	// ExternalAccess controls external service exposure.
	// +optional
	ExternalAccess *ExternalAccessConfig `json:"externalAccess,omitempty"`
	// Policy configures the operator-managed NetworkPolicy.
	// When nil, the operator creates a default NetworkPolicy with safe ingress rules.
	// +optional
	Policy *NetworkPolicySpec `json:"policy,omitempty"`
}

// PVCStorageSpec defines PVC storage for persistent data.
// +kubebuilder:validation:XValidation:rule="!has(self.mountPath) || self.mountPath.size() > 0",message="mountPath must not be empty if specified"
type PVCStorageSpec struct {
	// Size is the size of the PVC.
	// +optional
	Size *resource.Quantity `json:"size,omitempty"`
	// MountPath is the container mount path for the PVC.
	// +optional
	// +kubebuilder:default:="/.ogx"
	MountPath string `json:"mountPath,omitempty"`
}

// PodDisruptionBudgetSpec defines voluntary disruption controls.
// +kubebuilder:validation:XValidation:rule="has(self.minAvailable) || has(self.maxUnavailable)",message="at least one of minAvailable or maxUnavailable must be specified"
// +kubebuilder:validation:XValidation:rule="!(has(self.minAvailable) && has(self.maxUnavailable))",message="minAvailable and maxUnavailable are mutually exclusive"
type PodDisruptionBudgetSpec struct {
	// MinAvailable is the minimum number of pods that must remain available.
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`
	// MaxUnavailable is the maximum number of pods that can be disrupted simultaneously.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// AutoscalingSpec configures HorizontalPodAutoscaler targets.
// +kubebuilder:validation:XValidation:rule="!has(self.minReplicas) || self.maxReplicas >= self.minReplicas",message="maxReplicas must be greater than or equal to minReplicas"
type AutoscalingSpec struct {
	// MinReplicas is the lower bound replica count.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// MaxReplicas is the upper bound replica count.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Minimum=1
	MaxReplicas int32 `json:"maxReplicas"`
	// TargetCPUUtilizationPercentage configures CPU-based scaling.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`
	// TargetMemoryUtilizationPercentage configures memory-based scaling.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=100
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`
}

// WorkloadOverrides allows low-level customization of the Pod template.
// +kubebuilder:validation:XValidation:rule="!has(self.serviceAccountName) || self.serviceAccountName.size() > 0",message="serviceAccountName must not be empty if specified"
type WorkloadOverrides struct {
	// ServiceAccountName specifies a custom ServiceAccount.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// Env specifies additional environment variables.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Env []corev1.EnvVar `json:"env,omitempty"`
	// Command overrides the container command.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	Command []string `json:"command,omitempty"`
	// Args overrides the container arguments.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	Args []string `json:"args,omitempty"`
	// Volumes adds additional volumes to the Pod.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Volumes []corev1.Volume `json:"volumes,omitempty"`
	// VolumeMounts adds additional volume mounts to the container.
	// +optional
	// +kubebuilder:validation:MinItems=1
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`
}

// WorkloadSpec consolidates Kubernetes deployment settings.
type WorkloadSpec struct {
	// Replicas is the desired Pod replica count.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default:=1
	Replicas *int32 `json:"replicas,omitempty"`
	// Workers configures the number of uvicorn worker processes.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Workers *int32 `json:"workers,omitempty"`
	// Resources defines CPU/memory requests and limits.
	// +optional
	Resources *corev1.ResourceRequirements `json:"resources,omitempty"`
	// Autoscaling configures HPA for the server pods.
	// +optional
	Autoscaling *AutoscalingSpec `json:"autoscaling,omitempty"`
	// Storage defines PVC configuration.
	// +optional
	Storage *PVCStorageSpec `json:"storage,omitempty"`
	// PodDisruptionBudget controls voluntary disruption tolerance.
	// +optional
	PodDisruptionBudget *PodDisruptionBudgetSpec `json:"podDisruptionBudget,omitempty"`
	// TopologySpreadConstraints defines Pod spreading rules.
	// +optional
	// +kubebuilder:validation:MinItems=1
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	// Overrides allows pod-level customization.
	// +optional
	Overrides *WorkloadOverrides `json:"overrides,omitempty"`
}

// OverrideConfigSpec specifies a user-provided ConfigMap for full config.yaml override.
// Mutually exclusive with providers, resources, storage, and disabled.
type OverrideConfigSpec struct {
	// ConfigMapName is the name of the ConfigMap containing config.yaml.
	// Must be in the same namespace as the CR.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ConfigMapName string `json:"configMapName"`
}

// ExternalProviderRef references an external provider image.
type ExternalProviderRef struct {
	// ProviderID is the unique provider identifier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProviderID string `json:"providerId"`
	// Image is the container image containing the provider implementation.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Image string `json:"image"`
}

// ExternalProvidersSpec defines external provider injection.
type ExternalProvidersSpec struct {
	// Inference lists external inference providers to inject.
	// +optional
	// +kubebuilder:validation:MinItems=1
	Inference []ExternalProviderRef `json:"inference,omitempty"`
}

// OGXServerSpec defines the desired state of OGXServer.
// +kubebuilder:validation:XValidation:rule="!has(self.overrideConfig) || !has(self.providers)",message="overrideConfig and providers are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.overrideConfig) || !has(self.resources)",message="overrideConfig and resources are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.overrideConfig) || !has(self.storage)",message="overrideConfig and storage are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.overrideConfig) || !has(self.disabled)",message="overrideConfig and disabled are mutually exclusive"
// +kubebuilder:validation:XValidation:rule="!has(self.providers) || !has(self.disabled) || !self.disabled.exists(d, d == 'inference') || !has(self.providers.inference) || self.providers.inference.size() == 0",message="inference cannot be both in providers and disabled"
// +kubebuilder:validation:XValidation:rule="!has(self.providers) || !has(self.disabled) || !self.disabled.exists(d, d == 'vector_io') || !has(self.providers.vectorIo) || self.providers.vectorIo.size() == 0",message="vector_io cannot be both in providers and disabled"
// +kubebuilder:validation:XValidation:rule="!has(self.providers) || !has(self.disabled) || !self.disabled.exists(d, d == 'tool_runtime') || !has(self.providers.toolRuntime) || self.providers.toolRuntime.size() == 0",message="tool_runtime cannot be both in providers and disabled"
// +kubebuilder:validation:XValidation:rule="!has(self.providers) || !has(self.disabled) || !self.disabled.exists(d, d == 'telemetry') || !has(self.providers.telemetry) || self.providers.telemetry.size() == 0",message="telemetry cannot be both in providers and disabled"
//
//nolint:lll // kubebuilder markers cannot be split across lines.
type OGXServerSpec struct {
	// Distribution identifies the OGX distribution to deploy.
	// +kubebuilder:validation:Required
	Distribution DistributionSpec `json:"distribution"`
	// Providers configures providers by API type.
	// Mutually exclusive with overrideConfig.
	// +optional
	Providers *ProvidersSpec `json:"providers,omitempty"`
	// Resources declares models, tools, and shields to register.
	// Mutually exclusive with overrideConfig.
	// +optional
	Resources *ResourcesSpec `json:"resources,omitempty"`
	// Storage configures state storage backends (KV and SQL).
	// Mutually exclusive with overrideConfig.
	// +optional
	Storage *StateStorageSpec `json:"storage,omitempty"`
	// Disabled lists API names to remove from the generated config.
	// Mutually exclusive with overrideConfig.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:Enum=agents;inference;telemetry;tool_runtime;vector_io
	Disabled []string `json:"disabled,omitempty"`
	// Network defines network access controls.
	// +optional
	Network *NetworkSpec `json:"network,omitempty"`
	// CABundle defines the CA bundle configuration for custom certificates
	// used to verify outbound TLS connections to providers and backends.
	// +optional
	CABundle *CABundleConfig `json:"caBundle,omitempty"`
	// Workload consolidates Kubernetes deployment settings.
	// +optional
	Workload *WorkloadSpec `json:"workload,omitempty"`
	// ExternalProviders configures external provider injection.
	// +optional
	ExternalProviders *ExternalProvidersSpec `json:"externalProviders,omitempty"`
	// OverrideConfig specifies a user-provided ConfigMap for full config.yaml override.
	// Mutually exclusive with providers, resources, storage, and disabled.
	// +optional
	OverrideConfig *OverrideConfigSpec `json:"overrideConfig,omitempty"`
}

// OGXServerPhase represents the current phase of the OGXServer.
// +kubebuilder:validation:Enum=Pending;Initializing;Ready;Failed;Terminating
type OGXServerPhase string

const (
	OGXServerPhasePending      OGXServerPhase = "Pending"
	OGXServerPhaseInitializing OGXServerPhase = "Initializing"
	OGXServerPhaseReady        OGXServerPhase = "Ready"
	OGXServerPhaseFailed       OGXServerPhase = "Failed"
	OGXServerPhaseTerminating  OGXServerPhase = "Terminating"
)

// ProviderHealthStatus represents the health status of a provider.
type ProviderHealthStatus struct {
	Status  string `json:"status"`
	Message string `json:"message"`
}

// ProviderInfo represents a single provider from the providers endpoint.
type ProviderInfo struct {
	API          string               `json:"api"`
	ProviderID   string               `json:"provider_id"`
	ProviderType string               `json:"provider_type"`
	Config       apiextensionsv1.JSON `json:"config"`
	Health       ProviderHealthStatus `json:"health"`
}

// DistributionConfig represents the configuration from the providers endpoint.
type DistributionConfig struct {
	ActiveDistribution     string            `json:"activeDistribution,omitempty"`
	Providers              []ProviderInfo    `json:"providers,omitempty"`
	AvailableDistributions map[string]string `json:"availableDistributions,omitempty"`
}

// VersionInfo contains version-related information.
type VersionInfo struct {
	OperatorVersion string      `json:"operatorVersion,omitempty"`
	ServerVersion   string      `json:"serverVersion,omitempty"`
	LastUpdated     metav1.Time `json:"lastUpdated,omitempty"`
}

// ResolvedDistributionStatus tracks the resolved distribution image for change detection.
type ResolvedDistributionStatus struct {
	// Image is the resolved container image reference (with digest when available).
	Image string `json:"image,omitempty"`
	// ConfigSource indicates the config origin: "embedded" or "oci-label".
	ConfigSource string `json:"configSource,omitempty"`
	// ConfigHash is the SHA256 hash of the base config used.
	ConfigHash string `json:"configHash,omitempty"`
}

// ConfigGenerationStatus tracks config generation details.
type ConfigGenerationStatus struct {
	// ObservedGeneration is the spec generation that was last processed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ConfigMapName is the name of the generated ConfigMap.
	ConfigMapName string `json:"configMapName,omitempty"`
	// GeneratedAt is the timestamp of the last generation.
	GeneratedAt metav1.Time `json:"generatedAt,omitempty"`
	// ProviderCount is the number of configured providers.
	ProviderCount int `json:"providerCount,omitempty"`
	// ResourceCount is the number of registered resources.
	ResourceCount int `json:"resourceCount,omitempty"`
	// ConfigVersion is the config.yaml schema version.
	ConfigVersion int `json:"configVersion,omitempty"`
}

// OGXServerStatus defines the observed state of OGXServer.
type OGXServerStatus struct {
	// Phase represents the current phase of the server.
	Phase OGXServerPhase `json:"phase,omitempty"`
	// Version contains version information for both operator and server.
	Version VersionInfo `json:"version,omitempty"`
	// DistributionConfig contains provider information from the running server.
	DistributionConfig DistributionConfig `json:"distributionConfig,omitempty"`
	// ResolvedDistribution tracks the resolved image and config source.
	// +optional
	ResolvedDistribution *ResolvedDistributionStatus `json:"resolvedDistribution,omitempty"`
	// ConfigGeneration tracks config generation details.
	// +optional
	ConfigGeneration *ConfigGenerationStatus `json:"configGeneration,omitempty"`
	// Conditions represent the latest available observations of the server's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// AvailableReplicas is the number of available replicas.
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
	// ServiceURL is the internal Kubernetes service URL.
	ServiceURL string `json:"serviceURL,omitempty"`
	// ExternalURL is the external URL when external access is configured.
	// +optional
	ExternalURL *string `json:"externalURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=ogxs
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Distribution",type="string",JSONPath=".status.resolvedDistribution.image",priority=1
// +kubebuilder:printcolumn:name="Config",type="string",JSONPath=".status.configGeneration.configMapName",priority=1
// +kubebuilder:printcolumn:name="Providers",type="integer",JSONPath=".status.configGeneration.providerCount"
// +kubebuilder:printcolumn:name="Operator Version",type="string",JSONPath=".status.version.operatorVersion",priority=1
// +kubebuilder:printcolumn:name="Server Version",type="string",JSONPath=".status.version.serverVersion",priority=1
// +kubebuilder:printcolumn:name="Available",type="integer",JSONPath=".status.availableReplicas"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// OGXServer is the Schema for the ogxservers API.
type OGXServer struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OGXServerSpec   `json:"spec"`
	Status OGXServerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OGXServerList contains a list of OGXServer.
type OGXServerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OGXServer `json:"items"`
}

func init() { //nolint:gochecknoinits
	SchemeBuilder.Register(&OGXServer{}, &OGXServerList{})
}

// GetAdoptStorageSource returns the legacy LLSD name from the adopt-storage annotation, or empty string.
func (r *OGXServer) GetAdoptStorageSource() string {
	if r.Annotations == nil {
		return ""
	}
	return r.Annotations[AdoptStorageAnnotation]
}

// GetAdoptNetworkingSource returns the legacy LLSD name from the adopt-networking annotation, or empty string.
func (r *OGXServer) GetAdoptNetworkingSource() string {
	if r.Annotations == nil {
		return ""
	}
	return r.Annotations[AdoptNetworkingAnnotation]
}

// GetEffectivePVCName returns the PVC name the reconciler should use.
// When the adopt-storage annotation is present, the adopted PVC name is "{legacyName}-pvc".
// Otherwise the default convention is "{instanceName}-pvc".
func (r *OGXServer) GetEffectivePVCName() string {
	if src := r.GetAdoptStorageSource(); src != "" {
		return src + "-pvc"
	}
	return r.Name + "-pvc"
}

// ValidateAdoptionAnnotation validates that the given annotation value is a valid
// RFC 1123 DNS label: non-empty, lowercase alphanumeric or '-', at most 63 characters,
// starting and ending with an alphanumeric character.
func ValidateAdoptionAnnotation(value string) error {
	if value == "" {
		return errors.New("failed to validate adoption annotation: value must not be empty")
	}
	if len(value) > dns1123LabelMaxLen {
		return fmt.Errorf("failed to validate adoption annotation: value %q exceeds %d characters", value, dns1123LabelMaxLen)
	}
	if !dns1123LabelRegex.MatchString(value) {
		return fmt.Errorf("failed to validate adoption annotation: value %q is not a valid RFC 1123 DNS label", value)
	}
	return nil
}
