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

// VLLMProvider configures a remote::vllm inference provider instance.
type VLLMProvider struct {
	RoutedProviderBase `json:",inline"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// +optional
	APIToken *SecretKeyRef `json:"apiToken,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxTokens *int `json:"maxTokens,omitempty"`
}

func (p VLLMProvider) DeriveID() string { return p.deriveOrDefault("remote-vllm") }

// OpenAIProvider configures a remote::openai inference provider instance.
type OpenAIProvider struct {
	RoutedProviderBase `json:",inline"`
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// +kubebuilder:validation:Required
	APIKey SecretKeyRef `json:"apiKey"`
}

func (p OpenAIProvider) DeriveID() string { return p.deriveOrDefault("remote-openai") }

// AzureProvider configures a remote::azure inference provider instance.
type AzureProvider struct {
	RoutedProviderBase `json:",inline"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Endpoint string `json:"endpoint"`
	// +kubebuilder:validation:Required
	APIKey SecretKeyRef `json:"apiKey"`
	// +optional
	APIVersion string `json:"apiVersion,omitempty"`
}

func (p AzureProvider) DeriveID() string { return p.deriveOrDefault("remote-azure") }

// BedrockProvider configures a remote::bedrock inference provider instance.
type BedrockProvider struct {
	RoutedProviderBase `json:",inline"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Region string `json:"region"`
	// +optional
	AWSAccessKeyID *SecretKeyRef `json:"awsAccessKeyId,omitempty"`
	// +optional
	AWSSecretAccessKey *SecretKeyRef `json:"awsSecretAccessKey,omitempty"`
	// +optional
	AWSSessionToken *SecretKeyRef `json:"awsSessionToken,omitempty"`
	// +optional
	AWSRoleArn string `json:"awsRoleArn,omitempty"`
}

func (p BedrockProvider) DeriveID() string { return p.deriveOrDefault("remote-bedrock") }

// VertexAIProvider configures a remote::vertexai inference provider instance.
type VertexAIProvider struct {
	RoutedProviderBase `json:",inline"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Project string `json:"project"`
	// +optional
	// +kubebuilder:default:="global"
	Location string `json:"location,omitempty"`
}

func (p VertexAIProvider) DeriveID() string { return p.deriveOrDefault("remote-vertexai") }

// WatsonxProvider configures a remote::watsonx inference provider instance.
type WatsonxProvider struct {
	RoutedProviderBase `json:",inline"`
	// +optional
	Endpoint string `json:"endpoint,omitempty"`
	// +kubebuilder:validation:Required
	APIKey SecretKeyRef `json:"apiKey"`
	// +optional
	ProjectID string `json:"projectId,omitempty"`
}

func (p WatsonxProvider) DeriveID() string { return p.deriveOrDefault("remote-watsonx") }

// InlineSentenceTransformersProvider enables inline::sentence-transformers.
type InlineSentenceTransformersProvider struct {
	RoutedProviderBase `json:",inline"`
}

func (p InlineSentenceTransformersProvider) DeriveID() string {
	return p.deriveOrDefault("inline-sentence-transformers")
}

// InferenceRemoteProviders groups remote inference providers.
type InferenceRemoteProviders struct {
	// +optional
	VLLM []VLLMProvider `json:"vllm,omitempty"`
	// +optional
	OpenAI []OpenAIProvider `json:"openai,omitempty"`
	// +optional
	Azure []AzureProvider `json:"azure,omitempty"`
	// +optional
	Bedrock []BedrockProvider `json:"bedrock,omitempty"`
	// +optional
	VertexAI []VertexAIProvider `json:"vertexai,omitempty"`
	// +optional
	Watsonx []WatsonxProvider `json:"watsonx,omitempty"`
	// +optional
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
	SentenceTransformers []InlineSentenceTransformersProvider `json:"sentenceTransformers,omitempty"`
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *InferenceInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	return slices.Concat(deriveSliceIDs(inl.SentenceTransformers), deriveSliceIDs(inl.Custom))
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
