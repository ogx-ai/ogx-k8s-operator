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

// BraveSearchProvider configures a remote::brave-search tool runtime provider.
type BraveSearchProvider struct {
	RoutedProviderBase `json:",inline"`
	// +kubebuilder:validation:Required
	APIKey SecretKeyRef `json:"apiKey"`
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxResults *int `json:"maxResults,omitempty"`
}

func (p BraveSearchProvider) DeriveID() string { return p.deriveOrDefault("remote-brave-search") }

// TavilySearchProvider configures a remote::tavily-search tool runtime provider.
type TavilySearchProvider struct {
	RoutedProviderBase `json:",inline"`
	// +kubebuilder:validation:Required
	APIKey SecretKeyRef `json:"apiKey"`
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxResults *int `json:"maxResults,omitempty"`
}

func (p TavilySearchProvider) DeriveID() string { return p.deriveOrDefault("remote-tavily-search") }

// ModelContextProtocolProvider configures remote::model-context-protocol.
type ModelContextProtocolProvider struct {
	RoutedProviderBase `json:",inline"`
}

func (p ModelContextProtocolProvider) DeriveID() string {
	return p.deriveOrDefault("remote-model-context-protocol")
}

// InlineFileSearchProvider configures inline::file-search.
type InlineFileSearchProvider struct {
	RoutedProviderBase `json:",inline"`
}

func (p InlineFileSearchProvider) DeriveID() string { return p.deriveOrDefault("inline-file-search") }

// ToolRuntimeRemoteProviders groups remote tool runtime providers.
type ToolRuntimeRemoteProviders struct {
	// +optional
	BraveSearch []BraveSearchProvider `json:"braveSearch,omitempty"`
	// +optional
	TavilySearch []TavilySearchProvider `json:"tavilySearch,omitempty"`
	// +optional
	ModelContextProtocol []ModelContextProtocolProvider `json:"modelContextProtocol,omitempty"`
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *ToolRuntimeRemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	return slices.Concat(
		deriveSliceIDs(r.BraveSearch), deriveSliceIDs(r.TavilySearch),
		deriveSliceIDs(r.ModelContextProtocol), deriveSliceIDs(r.Custom),
	)
}

// ToolRuntimeInlineProviders groups inline tool runtime providers.
type ToolRuntimeInlineProviders struct {
	// +optional
	FileSearch []InlineFileSearchProvider `json:"fileSearch,omitempty"`
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *ToolRuntimeInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	return slices.Concat(deriveSliceIDs(inl.FileSearch), deriveSliceIDs(inl.Custom))
}

// ToolRuntimeProvidersSpec configures tool runtime providers.
type ToolRuntimeProvidersSpec struct {
	// +optional
	Remote *ToolRuntimeRemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *ToolRuntimeInlineProviders `json:"inline,omitempty"`
}

func (s *ToolRuntimeProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
