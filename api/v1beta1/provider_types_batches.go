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

// InlineReferenceProvider configures inline::reference for batches.
type InlineReferenceProvider struct {
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxConcurrentBatches *int `json:"maxConcurrentBatches,omitempty"`
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxConcurrentRequestsPerBatch *int `json:"maxConcurrentRequestsPerBatch,omitempty"`
}

func (p InlineReferenceProvider) DeriveID() string { return "inline-reference" }

// BatchesRemoteProviders groups remote batches providers.
type BatchesRemoteProviders struct {
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *BatchesRemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	return deriveSliceIDs(r.Custom)
}

// BatchesInlineProviders groups inline batches providers.
type BatchesInlineProviders struct {
	// +optional
	Reference *InlineReferenceProvider `json:"reference,omitempty"`
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *BatchesInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	var ids []string
	if inl.Reference != nil {
		ids = append(ids, inl.Reference.DeriveID())
	}
	return append(ids, deriveSliceIDs(inl.Custom)...)
}

// BatchesProvidersSpec configures batches providers.
type BatchesProvidersSpec struct {
	// +optional
	Remote *BatchesRemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *BatchesInlineProviders `json:"inline,omitempty"`
}

func (s *BatchesProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
