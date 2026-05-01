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

// SafetyRemoteProviders groups remote safety providers.
type SafetyRemoteProviders struct {
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *SafetyRemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	return deriveSliceIDs(r.Custom)
}

// SafetyInlineProviders groups inline safety providers.
type SafetyInlineProviders struct {
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *SafetyInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	return deriveSliceIDs(inl.Custom)
}

// SafetyProvidersSpec configures safety providers.
type SafetyProvidersSpec struct {
	// +optional
	Remote *SafetyRemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *SafetyInlineProviders `json:"inline,omitempty"`
}

func (s *SafetyProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
