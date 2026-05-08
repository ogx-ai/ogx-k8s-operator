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

import "slices"

// TLSConfig configures TLS settings for remote provider connections.
type TLSConfig struct {
	// Verify controls whether TLS certificate verification is enabled.
	// Trust anchors and client identity are configured globally via spec.tls.
	// +optional
	Verify *bool `json:"verify,omitempty"`
	// MinVersion sets the minimum TLS version.
	// +optional
	// +kubebuilder:validation:Enum=TLSv1.2;TLSv1.3
	MinVersion string `json:"minVersion,omitempty"`
	// Ciphers is a list of allowed TLS cipher suites.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	Ciphers []string `json:"ciphers,omitempty"`
}

// ProxyConfig configures HTTP proxy settings for remote provider connections.
// +kubebuilder:validation:XValidation:rule="!has(self.url) || self.url.size() > 0",message="url must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.http) || self.http.size() > 0",message="http must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.https) || self.https.size() > 0",message="https must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.cacert) || self.cacert.size() > 0",message="cacert must not be empty if specified"
type ProxyConfig struct {
	// URL is the proxy URL for all connections.
	// +optional
	URL *string `json:"url,omitempty"`
	// HTTP is the proxy URL for HTTP connections.
	// +optional
	HTTP *string `json:"http,omitempty"`
	// HTTPS is the proxy URL for HTTPS connections.
	// +optional
	HTTPS *string `json:"https,omitempty"`
	// CACert is the path to a CA certificate for verifying the proxy's certificate.
	// +optional
	CACert *string `json:"cacert,omitempty"`
	// NoProxy is a list of hosts that should bypass the proxy.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	NoProxy []string `json:"noProxy,omitempty"`
}

// TimeoutConfig configures network timeout settings.
type TimeoutConfig struct {
	// Connect is the connection timeout in seconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Connect *int `json:"connect,omitempty"`
	// Read is the read timeout in seconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Read *int `json:"read,omitempty"`
}

// NetworkConfig configures network settings for remote provider connections.
type NetworkConfig struct {
	// TLS configures TLS/SSL settings.
	// +optional
	TLS *TLSConfig `json:"tls,omitempty"`
	// Proxy configures HTTP proxy settings.
	// +optional
	Proxy *ProxyConfig `json:"proxy,omitempty"`
	// Timeout configures connection and read timeout settings.
	// +optional
	Timeout *TimeoutConfig `json:"timeout,omitempty"`
	// Headers specifies additional HTTP headers to include in all requests.
	// +optional
	// +kubebuilder:validation:MinProperties=1
	Headers map[string]string `json:"headers,omitempty"`
}

// RemoteInferenceCommonConfig contains fields shared by all remote inference providers.
type RemoteInferenceCommonConfig struct {
	// AllowedModels restricts which models can be registered with this provider.
	// When empty, all models are allowed.
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:items:MinLength=1
	AllowedModels []string `json:"allowedModels,omitempty"`
	// RefreshModels controls whether the provider periodically refreshes
	// its model list from the remote endpoint.
	// +optional
	RefreshModels *bool `json:"refreshModels,omitempty"`
	// Network configures network settings (TLS, proxy, timeouts, headers)
	// for the remote connection.
	// +optional
	Network *NetworkConfig `json:"network,omitempty"`
}

// VLLMProvider configures a remote::vllm inference provider instance.
type VLLMProvider struct {
	RoutedProviderBase          `json:",inline"`
	RemoteInferenceCommonConfig `json:",inline"`
	// Endpoint is the URL for the vLLM model serving endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// APIToken is the authentication token for the vLLM endpoint.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +optional
	APIToken *SecretKeyRef `json:"apiToken,omitempty"`
	// MaxTokens is the maximum number of tokens to generate.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxTokens *int `json:"maxTokens,omitempty"`
}

func (p VLLMProvider) DeriveID() string { return p.deriveOrDefault("remote-vllm") }

// OpenAIProvider configures a remote::openai inference provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.endpoint) || self.endpoint.size() > 0",message="endpoint must not be empty if specified"
type OpenAIProvider struct {
	RoutedProviderBase          `json:",inline"`
	RemoteInferenceCommonConfig `json:",inline"`
	// Endpoint is the base URL for the OpenAI API.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// APIKey is the authentication credential for the OpenAI provider.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +kubebuilder:validation:Required
	APIKey SecretKeyRef `json:"apiKey"`
}

func (p OpenAIProvider) DeriveID() string { return p.deriveOrDefault("remote-openai") }

// AzureProvider configures a remote::azure inference provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.apiVersion) || self.apiVersion.size() > 0",message="apiVersion must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.apiType) || self.apiType.size() > 0",message="apiType must not be empty if specified"
type AzureProvider struct {
	RoutedProviderBase          `json:",inline"`
	RemoteInferenceCommonConfig `json:",inline"`
	// Endpoint is the Azure API base URL
	// (e.g., https://your-resource-name.openai.azure.com/openai/v1).
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// APIKey is the authentication credential for the Azure provider.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +kubebuilder:validation:Required
	APIKey SecretKeyRef `json:"apiKey"`
	// APIVersion is the Azure API version (e.g., 2024-12-01-preview).
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
	// APIType is the Azure API type (e.g., azure).
	// +optional
	APIType string `json:"apiType,omitempty"`
}

func (p AzureProvider) DeriveID() string { return p.deriveOrDefault("remote-azure") }

// BedrockProvider configures a remote::bedrock inference provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.awsRoleArn) || self.awsRoleArn.size() > 0",message="awsRoleArn must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.awsWebIdentityTokenFile) || self.awsWebIdentityTokenFile.size() > 0",message="awsWebIdentityTokenFile must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.awsRoleSessionName) || self.awsRoleSessionName.size() > 0",message="awsRoleSessionName must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.profileName) || self.profileName.size() > 0",message="profileName must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.retryMode) || self.retryMode.size() > 0",message="retryMode must not be empty if specified"
//
//nolint:lll // kubebuilder marker cannot be split across lines.
type BedrockProvider struct {
	RoutedProviderBase          `json:",inline"`
	RemoteInferenceCommonConfig `json:",inline"`
	// Region is the AWS region for the Bedrock Runtime endpoint.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`
	// APIKey is the authentication credential for the Bedrock provider.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +optional
	APIKey *SecretKeyRef `json:"apiKey,omitempty"`
	// AWSAccessKeyID is the AWS access key to use.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +optional
	AWSAccessKeyID *SecretKeyRef `json:"awsAccessKeyId,omitempty"`
	// AWSSecretAccessKey is the AWS secret access key to use.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +optional
	AWSSecretAccessKey *SecretKeyRef `json:"awsSecretAccessKey,omitempty"`
	// AWSSessionToken is the AWS session token to use.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +optional
	AWSSessionToken *SecretKeyRef `json:"awsSessionToken,omitempty"`
	// AWSRoleArn is the AWS role ARN to assume.
	// +optional
	AWSRoleArn string `json:"awsRoleArn,omitempty"`
	// AWSWebIdentityTokenFile is the path to the web identity token file.
	// +optional
	AWSWebIdentityTokenFile string `json:"awsWebIdentityTokenFile,omitempty"`
	// AWSRoleSessionName is the session name to use when assuming a role.
	// +optional
	AWSRoleSessionName string `json:"awsRoleSessionName,omitempty"`
	// ProfileName is the AWS profile name that contains credentials to use.
	// +optional
	ProfileName string `json:"profileName,omitempty"`
	// TotalMaxAttempts is the maximum number of attempts for a single request,
	// including the initial attempt.
	// +optional
	// +kubebuilder:validation:Minimum=1
	TotalMaxAttempts *int `json:"totalMaxAttempts,omitempty"`
	// RetryMode is the type of retries to perform (e.g., standard, adaptive).
	// +optional
	RetryMode string `json:"retryMode,omitempty"`
	// ConnectTimeout is the connection timeout in seconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ConnectTimeout *int `json:"connectTimeout,omitempty"`
	// ReadTimeout is the read timeout in seconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ReadTimeout *int `json:"readTimeout,omitempty"`
	// SessionTTL is the time in seconds until a session expires.
	// +optional
	// +kubebuilder:validation:Minimum=1
	SessionTTL *int `json:"sessionTTL,omitempty"`
}

func (p BedrockProvider) DeriveID() string { return p.deriveOrDefault("remote-bedrock") }

// VertexAIProvider configures a remote::vertexai inference provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.location) || self.location.size() > 0",message="location must not be empty if specified"
type VertexAIProvider struct {
	RoutedProviderBase          `json:",inline"`
	RemoteInferenceCommonConfig `json:",inline"`
	// Project is the Google Cloud project ID for Vertex AI.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`
	// Location is the Google Cloud location for Vertex AI.
	// +optional
	Location string `json:"location,omitempty"`
}

func (p VertexAIProvider) DeriveID() string { return p.deriveOrDefault("remote-vertexai") }

// WatsonxProvider configures a remote::watsonx inference provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.endpoint) || self.endpoint.size() > 0",message="endpoint must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.projectId) || self.projectId.size() > 0",message="projectId must not be empty if specified"
type WatsonxProvider struct {
	RoutedProviderBase          `json:",inline"`
	RemoteInferenceCommonConfig `json:",inline"`
	// Endpoint is the base URL for accessing watsonx.ai.
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// APIKey is the authentication credential for the watsonx provider.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +kubebuilder:validation:Required
	APIKey SecretKeyRef `json:"apiKey"`
	// ProjectID is the watsonx.ai project ID.
	// +optional
	ProjectID string `json:"projectId,omitempty"`
	// Timeout is the timeout in seconds for HTTP requests.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Timeout *int `json:"timeout,omitempty"`
}

func (p WatsonxProvider) DeriveID() string { return p.deriveOrDefault("remote-watsonx") }

// InferenceRemoteProviders groups remote inference providers.
type InferenceRemoteProviders struct {
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	VLLM []VLLMProvider `json:"vllm,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	OpenAI []OpenAIProvider `json:"openai,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Azure []AzureProvider `json:"azure,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Bedrock []BedrockProvider `json:"bedrock,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	VertexAI []VertexAIProvider `json:"vertexai,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Watsonx []WatsonxProvider `json:"watsonx,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *InferenceRemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	return slices.Concat(
		deriveSliceIDs(r.VLLM), deriveSliceIDs(r.OpenAI), deriveSliceIDs(r.Azure),
		deriveSliceIDs(r.Bedrock), deriveSliceIDs(r.VertexAI), deriveSliceIDs(r.Watsonx),
		deriveSliceIDs(r.Custom),
	)
}

// InferenceInlineProviders groups inline inference providers.
type InferenceInlineProviders struct {
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *InferenceInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	return deriveSliceIDs(inl.Custom)
}

// InferenceProvidersSpec configures inference providers.
type InferenceProvidersSpec struct {
	// +optional
	Remote *InferenceRemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *InferenceInlineProviders `json:"inline,omitempty"`
}

func (s *InferenceProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
