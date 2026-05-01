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

// PgvectorProvider configures a remote::pgvector vector I/O provider instance.
type PgvectorProvider struct {
	RoutedProviderBase `json:",inline"`
	// +optional
	// +kubebuilder:default:="localhost"
	Host string `json:"host,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	// +kubebuilder:default:=5432
	Port int `json:"port,omitempty"`
	// +optional
	// +kubebuilder:default:="postgres"
	DB string `json:"db,omitempty"`
	// +optional
	// +kubebuilder:default:="postgres"
	User string `json:"user,omitempty"`
	// +kubebuilder:validation:Required
	Password SecretKeyRef `json:"password"`
}

func (p PgvectorProvider) DeriveID() string { return p.deriveOrDefault("remote-pgvector") }

// MilvusProvider configures a remote::milvus vector I/O provider instance.
type MilvusProvider struct {
	RoutedProviderBase `json:",inline"`
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`
	// +optional
	Token *SecretKeyRef `json:"token,omitempty"`
}

func (p MilvusProvider) DeriveID() string { return p.deriveOrDefault("remote-milvus") }

// QdrantProvider configures a remote::qdrant vector I/O provider instance.
// +kubebuilder:validation:XValidation:rule="has(self.url) || has(self.host)",message="at least one of url or host must be specified"
type QdrantProvider struct {
	RoutedProviderBase `json:",inline"`
	// +optional
	URL string `json:"url,omitempty"`
	// +optional
	Host string `json:"host,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port *int `json:"port,omitempty"`
	// +optional
	APIKey *SecretKeyRef `json:"apiKey,omitempty"`
}

func (p QdrantProvider) DeriveID() string { return p.deriveOrDefault("remote-qdrant") }

// VectorIORemoteProviders groups remote vector I/O providers.
type VectorIORemoteProviders struct {
	// +optional
	Pgvector []PgvectorProvider `json:"pgvector,omitempty"`
	// +optional
	Milvus []MilvusProvider `json:"milvus,omitempty"`
	// +optional
	Qdrant []QdrantProvider `json:"qdrant,omitempty"`
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *VectorIORemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	return slices.Concat(
		deriveSliceIDs(r.Pgvector), deriveSliceIDs(r.Milvus),
		deriveSliceIDs(r.Qdrant), deriveSliceIDs(r.Custom),
	)
}

// VectorIOInlineProviders groups inline vector I/O providers.
type VectorIOInlineProviders struct {
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *VectorIOInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	return deriveSliceIDs(inl.Custom)
}

// VectorIOProvidersSpec configures vector I/O providers.
type VectorIOProvidersSpec struct {
	// +optional
	Remote *VectorIORemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *VectorIOInlineProviders `json:"inline,omitempty"`
}

func (s *VectorIOProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
