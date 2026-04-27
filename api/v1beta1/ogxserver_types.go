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
	DefaultMountPath = "/.llama"
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

// DistributionSpec defines the distribution configuration.
// Exactly one of name or image must be specified.
// +kubebuilder:validation:XValidation:rule="!(has(self.name) && has(self.image))",message="Only one of name or image can be specified"
type DistributionSpec struct {
	// Name is the distribution name that maps to supported distributions.
	// +optional
	Name string `json:"name,omitempty"`
	// Image is the direct container image reference to use.
	// +optional
	Image string `json:"image,omitempty"`
}

// SecretKeyRef is a reference to a key in a Kubernetes Secret.
type SecretKeyRef struct {
	// Name is the name of the Secret.
	Name string `json:"name"`
	// Key is the key within the Secret.
	Key string `json:"key"`
}

// ProviderConfig defines a single provider configuration.
type ProviderConfig struct {
	// ID is the unique identifier for this provider instance.
	ID string `json:"id"`
	// Provider is the provider type (e.g. "remote::ollama").
	Provider string `json:"provider"`
	// Endpoint is the URL for remote providers.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// APIKey references a Secret key holding the provider's API key.
	// +optional
	APIKey *SecretKeyRef `json:"apiKey,omitempty"`
	// Settings holds provider-specific configuration.
	// +optional
	Settings *apiextensionsv1.JSON `json:"settings,omitempty"`
}

// ProvidersSpec groups provider configurations by API category.
type ProvidersSpec struct {
	// Inference providers.
	// +optional
	Inference *apiextensionsv1.JSON `json:"inference,omitempty"`
	// Safety providers.
	// +optional
	Safety *apiextensionsv1.JSON `json:"safety,omitempty"`
	// VectorIO providers.
	// +optional
	VectorIO *apiextensionsv1.JSON `json:"vectorIo,omitempty"`
	// ToolRuntime providers.
	// +optional
	ToolRuntime *apiextensionsv1.JSON `json:"toolRuntime,omitempty"`
	// Telemetry providers.
	// +optional
	Telemetry *apiextensionsv1.JSON `json:"telemetry,omitempty"`
}

// ModelConfig defines a model to register with the server.
type ModelConfig struct {
	// Name is the model identifier.
	Name string `json:"name"`
	// Provider is the provider ID that serves this model.
	// +optional
	Provider string `json:"provider,omitempty"`
	// ContextLength is the maximum context window size.
	// +optional
	ContextLength *int32 `json:"contextLength,omitempty"`
	// ModelType classifies the model (e.g. llm, embedding).
	// +optional
	ModelType string `json:"modelType,omitempty"`
	// Quantization specifies the quantization format.
	// +optional
	Quantization string `json:"quantization,omitempty"`
}

// ResourcesSpec defines models, tools, and shields to register.
type ResourcesSpec struct {
	// Models to register. Each element is a JSON object for polymorphic form.
	// +optional
	Models []apiextensionsv1.JSON `json:"models,omitempty"`
	// Tools to register.
	// +optional
	Tools []apiextensionsv1.JSON `json:"tools,omitempty"`
	// Shields to register by name.
	// +optional
	Shields []string `json:"shields,omitempty"`
}

// KVStorageSpec configures key-value state storage.
type KVStorageSpec struct {
	// Type is the backend type: sqlite or redis.
	// +kubebuilder:validation:Enum=sqlite;redis
	Type string `json:"type"`
	// Endpoint is the connection endpoint for remote backends.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// Password references a Secret key holding the backend password.
	// +optional
	Password *SecretKeyRef `json:"password,omitempty"`
}

// SQLStorageSpec configures SQL state storage.
type SQLStorageSpec struct {
	// Type is the backend type: sqlite or postgres.
	// +kubebuilder:validation:Enum=sqlite;postgres
	Type string `json:"type"`
	// ConnectionString references a Secret key holding the full connection string.
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

// TLSSpec defines TLS configuration.
type TLSSpec struct {
	// Enabled toggles TLS termination.
	// +optional
	Enabled bool `json:"enabled,omitempty"`
	// SecretName references a TLS Secret.
	// +optional
	SecretName string `json:"secretName,omitempty"`
	// CABundle defines the CA bundle configuration for custom certificates.
	// +optional
	CABundle *CABundleConfig `json:"caBundle,omitempty"`
}

// NetworkPolicySpec configures the operator-managed NetworkPolicy for this server.
// When nil or enabled with no rules, the operator generates safe defaults:
// ingress on the service port from same-namespace and operator-namespace; egress unrestricted.
// When ingress or egress rules are explicitly provided, they are used verbatim.
// When any egress rules are configured, a kube-dns egress rule is auto-injected.
type NetworkPolicySpec struct {
	// Enabled controls whether the operator manages a NetworkPolicy for this server.
	// Defaults to true. Set to false to disable NetworkPolicy creation entirely.
	// +optional
	// +kubebuilder:default:=true
	Enabled *bool `json:"enabled,omitempty"`
	// Ingress rules. When nil, the operator generates default ingress rules
	// (allow from same-namespace and operator-namespace on the service port).
	// When explicitly set, these rules are used verbatim.
	// +optional
	Ingress []networkingv1.NetworkPolicyIngressRule `json:"ingress,omitempty"`
	// Egress rules. When nil, egress is unrestricted (no Egress policyType set).
	// When explicitly set, these rules are used and a kube-dns egress rule is
	// auto-injected to prevent DNS breakage.
	// +optional
	Egress []networkingv1.NetworkPolicyEgressRule `json:"egress,omitempty"`
}

// NetworkSpec defines network access controls for the OGXServer.
type NetworkSpec struct {
	// Port overrides the default container and service port.
	// +optional
	Port *int32 `json:"port,omitempty"`
	// TLS configures TLS termination.
	// +optional
	TLS *TLSSpec `json:"tls,omitempty"`
	// Expose configures external access (e.g. Ingress). Polymorphic JSON for flexibility.
	// +optional
	Expose *apiextensionsv1.JSON `json:"expose,omitempty"`
	// NetworkPolicy configures the operator-managed NetworkPolicy.
	// When nil, the operator creates a default NetworkPolicy with safe ingress rules.
	// +optional
	NetworkPolicy *NetworkPolicySpec `json:"networkPolicy,omitempty"`
}

// PVCStorageSpec defines PVC size and mount path.
type PVCStorageSpec struct {
	// Size is the PVC storage request.
	// +optional
	Size *resource.Quantity `json:"size,omitempty"`
	// MountPath is the path where the PVC is mounted in the container.
	// +optional
	MountPath string `json:"mountPath,omitempty"`
}

// PodDisruptionBudgetSpec defines voluntary disruption controls.
type PodDisruptionBudgetSpec struct {
	// MinAvailable is the minimum number of pods that must remain available.
	// +optional
	MinAvailable *intstr.IntOrString `json:"minAvailable,omitempty"`
	// MaxUnavailable is the maximum number of pods that can be disrupted simultaneously.
	// +optional
	MaxUnavailable *intstr.IntOrString `json:"maxUnavailable,omitempty"`
}

// AutoscalingSpec configures HorizontalPodAutoscaler targets.
type AutoscalingSpec struct {
	// MinReplicas is the lower bound replica count.
	// +optional
	MinReplicas *int32 `json:"minReplicas,omitempty"`
	// MaxReplicas is the upper bound replica count.
	MaxReplicas int32 `json:"maxReplicas"`
	// TargetCPUUtilizationPercentage configures CPU based scaling.
	// +optional
	TargetCPUUtilizationPercentage *int32 `json:"targetCPUUtilizationPercentage,omitempty"`
	// TargetMemoryUtilizationPercentage configures memory based scaling.
	// +optional
	TargetMemoryUtilizationPercentage *int32 `json:"targetMemoryUtilizationPercentage,omitempty"`
}

// WorkloadOverrides allows advanced pod-level customization.
type WorkloadOverrides struct {
	// ServiceAccountName allows users to specify their own ServiceAccount.
	// +optional
	ServiceAccountName string `json:"serviceAccountName,omitempty"`
	// Env specifies additional environment variables.
	// +optional
	Env []corev1.EnvVar `json:"env,omitempty"`
	// Command overrides the container entrypoint.
	// +optional
	Command []string `json:"command,omitempty"`
	// Args overrides the container arguments.
	// +optional
	Args []string `json:"args,omitempty"`
	// Volumes specifies additional volumes.
	// +optional
	Volumes []corev1.Volume `json:"volumes,omitempty"`
	// VolumeMounts specifies additional volume mounts.
	// +optional
	VolumeMounts []corev1.VolumeMount `json:"volumeMounts,omitempty"`
}

// WorkloadSpec defines deployment-level configuration.
type WorkloadSpec struct {
	// Replicas is the desired pod count.
	// +kubebuilder:default:=1
	// +optional
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
	// TopologySpreadConstraints defines fine-grained spreading rules.
	// +optional
	TopologySpreadConstraints []corev1.TopologySpreadConstraint `json:"topologySpreadConstraints,omitempty"`
	// Overrides allows pod-level customization.
	// +optional
	Overrides *WorkloadOverrides `json:"overrides,omitempty"`
}

// OverrideConfigSpec references a user-provided ConfigMap for full config control.
type OverrideConfigSpec struct {
	// ConfigMapName is the name of the ConfigMap containing the server configuration.
	ConfigMapName string `json:"configMapName"`
}

// OGXServerSpec defines the desired state of OGXServer.
//
//nolint:lll
//+kubebuilder:validation:XValidation:rule="!(has(self.overrideConfig) && (has(self.providers) || has(self.resources) || has(self.storage) || has(self.disabled)))",message="overrideConfig is mutually exclusive with providers, resources, storage, and disabled"
type OGXServerSpec struct {
	// Distribution specifies which OGX distribution image to deploy.
	Distribution DistributionSpec `json:"distribution"`
	// Providers configures inference, safety, and other provider backends.
	// +optional
	Providers *ProvidersSpec `json:"providers,omitempty"`
	// Resources defines models, tools, and shields to register.
	// +optional
	Resources *ResourcesSpec `json:"resources,omitempty"`
	// Storage configures state storage backends (KV, SQL).
	// +optional
	Storage *StateStorageSpec `json:"storage,omitempty"`
	// Disabled lists API categories to disable.
	// +optional
	Disabled []string `json:"disabled,omitempty"`
	// Network defines network access controls.
	// +optional
	Network *NetworkSpec `json:"network,omitempty"`
	// Workload defines deployment, scaling, and pod configuration.
	// +optional
	Workload *WorkloadSpec `json:"workload,omitempty"`
	// ExternalProviders references external provider configurations.
	// +optional
	ExternalProviders []ProviderConfig `json:"externalProviders,omitempty"`
	// OverrideConfig references a user-provided ConfigMap that replaces all generated config.
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

// ResolvedDistributionStatus reports the resolved distribution image.
type ResolvedDistributionStatus struct {
	// Image is the resolved container image reference.
	Image string `json:"image,omitempty"`
	// Source indicates how the image was resolved (e.g. "name", "image").
	Source string `json:"source,omitempty"`
}

// ConfigGenerationStatus reports the state of config generation.
type ConfigGenerationStatus struct {
	// ObservedGeneration is the spec generation that was last processed.
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
	// ConfigMapName is the name of the generated ConfigMap.
	ConfigMapName string `json:"configMapName,omitempty"`
}

// OGXServerStatus defines the observed state of OGXServer.
type OGXServerStatus struct {
	// Phase represents the current phase of the server.
	Phase OGXServerPhase `json:"phase,omitempty"`
	// Version contains version information for both operator and server.
	Version VersionInfo `json:"version,omitempty"`
	// DistributionConfig contains provider information from the running server.
	DistributionConfig DistributionConfig `json:"distributionConfig,omitempty"`
	// ResolvedDistribution reports the resolved distribution image.
	ResolvedDistribution ResolvedDistributionStatus `json:"resolvedDistribution,omitempty"`
	// ConfigGeneration reports the state of config generation.
	ConfigGeneration ConfigGenerationStatus `json:"configGeneration,omitempty"`
	// Conditions represent the latest available observations of the server's state.
	Conditions []metav1.Condition `json:"conditions,omitempty"`
	// AvailableReplicas is the number of available replicas.
	AvailableReplicas int32 `json:"availableReplicas,omitempty"`
	// ServiceURL is the internal Kubernetes service URL.
	ServiceURL string `json:"serviceURL,omitempty"`
	// RouteURL is the external URL when expose is configured.
	// +optional
	RouteURL *string `json:"routeURL,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:shortName=ogxs
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Phase",type="string",JSONPath=".status.phase"
// +kubebuilder:printcolumn:name="Operator Version",type="string",JSONPath=".status.version.operatorVersion"
// +kubebuilder:printcolumn:name="Server Version",type="string",JSONPath=".status.version.serverVersion"
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
