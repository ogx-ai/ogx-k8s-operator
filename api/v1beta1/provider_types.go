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

import (
	"slices"
	"strings"

	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

// Provider is implemented by all provider types (typed and custom).
// +kubebuilder:object:generate=false
type Provider interface {
	DeriveID() string
}

// RoutedProviderBase contains fields common to all routed (non-singleton) provider instances.
// +kubebuilder:validation:XValidation:rule="!has(self.id) || self.id.size() > 0",message="id must not be empty if specified"
type RoutedProviderBase struct {
	// ID is a unique provider identifier. Derived from the provider
	// type when omitted. Must be unique across all providers.
	// +optional
	ID string `json:"id,omitempty"`
}

// CustomProvider defines the configuration for a custom provider instance.
// +kubebuilder:validation:XValidation:rule="self.type.startsWith('remote::') || self.type.startsWith('inline::')",message="type must have a 'remote::' or 'inline::' prefix (e.g., 'remote::llama-guard', 'inline::my-provider')"
//
//nolint:lll // CEL validation rule
type CustomProvider struct {
	RoutedProviderBase `json:",inline"`
	// Type is the provider type, specified with a "remote::" or "inline::"
	// prefix (e.g., "remote::llama-guard", "inline::my-provider").
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Type string `json:"type"`
	// SecretRefs is a map of named secret references for provider-specific
	// connection fields (e.g., host, password). Each key becomes the env var
	// field suffix and maps to config.<key> with env var substitution.
	// +optional
	// +kubebuilder:validation:MinProperties=1
	SecretRefs map[string]SecretKeyRef `json:"secretRefs,omitempty"`
	// Settings contains provider-specific configuration merged into the
	// provider's config section in config.yaml. Passed through as-is
	// without any secret resolution. Use secretRefs for secret values.
	// +optional
	Settings *apiextensionsv1.JSON `json:"settings,omitempty"`
}

func (c CustomProvider) DeriveID() string {
	return c.deriveOrDefault(strings.ReplaceAll(c.Type, "::", "-"))
}

// ProvidersSpec configures providers by API type.
type ProvidersSpec struct {
	// +optional
	Inference *InferenceProvidersSpec `json:"inference,omitempty"`
	// +optional
	Safety *SafetyProvidersSpec `json:"safety,omitempty"`
	// +optional
	VectorIo *VectorIOProvidersSpec `json:"vectorIo,omitempty"`
	// +optional
	ToolRuntime *ToolRuntimeProvidersSpec `json:"toolRuntime,omitempty"`
	// +optional
	Files *FilesProvidersSpec `json:"files,omitempty"`
	// +optional
	Batches *BatchesProvidersSpec `json:"batches,omitempty"`
	// +optional
	Responses *ResponsesProvidersSpec `json:"responses,omitempty"`
}

func (s *ProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Inference.IDs(), s.Safety.IDs(), s.VectorIo.IDs(), s.ToolRuntime.IDs(), s.Files.IDs(), s.Batches.IDs(), s.Responses.IDs())
}

func (b RoutedProviderBase) deriveOrDefault(defaultID string) string {
	if b.ID != "" {
		return b.ID
	}
	return defaultID
}

// deriveSliceIDs maps DeriveID over a provider slice.
func deriveSliceIDs[T Provider](items []T) []string {
	ids := make([]string, len(items))
	for i := range items {
		ids[i] = items[i].DeriveID()
	}
	return ids
}
