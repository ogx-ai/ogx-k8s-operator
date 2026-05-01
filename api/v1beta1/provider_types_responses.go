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

// InlineBuiltinResponsesProvider configures inline::builtin for responses.
type InlineBuiltinResponsesProvider struct{}

func (p InlineBuiltinResponsesProvider) DeriveID() string { return "inline-builtin" }

// ResponsesRemoteProviders groups remote responses providers.
type ResponsesRemoteProviders struct {
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *ResponsesRemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	return deriveSliceIDs(r.Custom)
}

// ResponsesInlineProviders groups inline responses providers.
type ResponsesInlineProviders struct {
	// +optional
	Builtin *InlineBuiltinResponsesProvider `json:"builtin,omitempty"`
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *ResponsesInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	var ids []string
	if inl.Builtin != nil {
		ids = append(ids, inl.Builtin.DeriveID())
	}
	return append(ids, deriveSliceIDs(inl.Custom)...)
}

// ResponsesProvidersSpec configures responses providers.
type ResponsesProvidersSpec struct {
	// +optional
	Remote *ResponsesRemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *ResponsesInlineProviders `json:"inline,omitempty"`
}

func (s *ResponsesProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
