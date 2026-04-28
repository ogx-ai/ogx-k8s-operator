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
	"strings"
	"testing"
)

func TestDeriveProviderID(t *testing.T) {
	tests := []struct {
		name         string
		providerType string
		want         string
	}{
		{
			name:         "plain provider",
			providerType: "ollama",
			want:         "ollama",
		},
		{
			name:         "remote prefix",
			providerType: "remote::ollama",
			want:         "ollama",
		},
		{
			name:         "double prefix",
			providerType: "something::remote::vllm",
			want:         "vllm",
		},
		{
			name:         "empty string",
			providerType: "",
			want:         "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := deriveProviderID(tt.providerType); got != tt.want {
				t.Errorf("deriveProviderID(%q) = %q, want %q", tt.providerType, got, tt.want)
			}
		})
	}
}

func TestValidateProviderIDUniqueness(t *testing.T) {
	tests := []struct {
		name      string
		providers *ProvidersSpec
		wantErrs  int
	}{
		{
			name:      "empty providers",
			providers: &ProvidersSpec{},
			wantErrs:  0,
		},
		{
			name: "no collision across slices",
			providers: &ProvidersSpec{
				Inference: []ProviderConfig{{Provider: "ollama"}},
				VectorIo: []ProviderConfig{{Provider: "pgvector"}},
			},
			wantErrs: 0,
		},
		{
			name: "collision across inference and vectorIo",
			providers: &ProvidersSpec{
				Inference: []ProviderConfig{{ID: "shared-id", Provider: "ollama"}},
				VectorIo: []ProviderConfig{{ID: "shared-id", Provider: "pgvector"}},
			},
			wantErrs: 1,
		},
		{
			name: "collision via derived ID across slices",
			providers: &ProvidersSpec{
				Inference:   []ProviderConfig{{Provider: "remote::ollama"}},
				ToolRuntime: []ProviderConfig{{Provider: "ollama"}},
			},
			wantErrs: 1,
		},
		{
			name: "safety slice included in check",
			providers: &ProvidersSpec{
				Inference: []ProviderConfig{{ID: "my-provider", Provider: "ollama"}},
				Safety:    []ProviderConfig{{ID: "my-provider", Provider: "llama-guard"}},
			},
			wantErrs: 1,
		},
		{
			name: "multiple collisions",
			providers: &ProvidersSpec{
				Inference: []ProviderConfig{{ID: "dup1", Provider: "a"}, {ID: "dup2", Provider: "b"}},
				Safety:    []ProviderConfig{{ID: "dup1", Provider: "c"}},
				VectorIo: []ProviderConfig{{ID: "dup2", Provider: "d"}},
			},
			wantErrs: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateProviderIDUniqueness(tt.providers)
			if len(errs) != tt.wantErrs {
				t.Errorf("validateProviderIDUniqueness() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateProviderReferences(t *testing.T) {
	tests := []struct {
		name      string
		resources *ResourcesSpec
		providers *ProvidersSpec
		wantErrs  int
		errSubstr string
	}{
		{
			name: "valid provider reference",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3", Provider: "ollama"}},
			},
			providers: &ProvidersSpec{
				Inference: []ProviderConfig{{Provider: "ollama"}},
			},
			wantErrs: 0,
		},
		{
			name: "unknown provider reference",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3", Provider: "nonexistent"}},
			},
			providers: &ProvidersSpec{
				Inference: []ProviderConfig{{Provider: "ollama"}},
			},
			wantErrs:  1,
			errSubstr: "references unknown provider ID",
		},
		{
			name: "empty provider field is allowed",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3"}},
			},
			providers: &ProvidersSpec{
				Inference: []ProviderConfig{{Provider: "ollama"}},
			},
			wantErrs: 0,
		},
		{
			name: "reference to provider with explicit ID",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3", Provider: "my-ollama"}},
			},
			providers: &ProvidersSpec{
				Inference: []ProviderConfig{{ID: "my-ollama", Provider: "ollama"}},
			},
			wantErrs: 0,
		},
		{
			name: "reference to safety provider",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3", Provider: "llama-guard"}},
			},
			providers: &ProvidersSpec{
				Safety: []ProviderConfig{{Provider: "llama-guard"}},
			},
			wantErrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateProviderReferences(tt.resources, tt.providers)
			if len(errs) != tt.wantErrs {
				t.Errorf("validateProviderReferences() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
			if tt.errSubstr != "" && len(errs) > 0 {
				if !strings.Contains(errs[0].Detail, tt.errSubstr) {
					t.Errorf("error detail %q does not contain %q", errs[0].Detail, tt.errSubstr)
				}
			}
		})
	}
}

func TestValidateDistributionName(t *testing.T) {
	knownNames := []string{"starter", "remote-vllm", "meta-reference-gpu", "postgres-demo"}

	tests := []struct {
		name       string
		distName   string
		knownNames []string
		wantErrs   int
		errSubstr  string
	}{
		{
			name:       "valid distribution name",
			distName:   "starter",
			knownNames: knownNames,
			wantErrs:   0,
		},
		{
			name:       "unknown distribution name",
			distName:   "nonexistent",
			knownNames: knownNames,
			wantErrs:   1,
			errSubstr:  "unknown distribution",
		},
		{
			name:       "empty known names skips validation",
			distName:   "anything",
			knownNames: nil,
			wantErrs:   0,
		},
		{
			name:       "error lists available distributions",
			distName:   "bad",
			knownNames: knownNames,
			wantErrs:   1,
			errSubstr:  "starter",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateDistributionName(tt.distName, tt.knownNames)
			if len(errs) != tt.wantErrs {
				t.Errorf("validateDistributionName(%q) returned %d errors, want %d: %v", tt.distName, len(errs), tt.wantErrs, errs)
			}
			if tt.errSubstr != "" && len(errs) > 0 {
				if !strings.Contains(errs[0].Detail, tt.errSubstr) {
					t.Errorf("error detail %q does not contain %q", errs[0].Detail, tt.errSubstr)
				}
			}
		})
	}
}

func TestCollectValidationErrors(t *testing.T) {
	knownNames := []string{"starter", "remote-vllm"}

	tests := []struct {
		name     string
		server   *OGXServer
		wantErrs int
	}{
		{
			name: "valid server with all fields",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					Providers: &ProvidersSpec{
						Inference: []ProviderConfig{{Provider: "ollama"}},
					},
					Resources: &ResourcesSpec{
						Models: []ModelConfig{{Name: "llama3", Provider: "ollama"}},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "image-based distribution skips name validation",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "custom:latest"},
				},
			},
			wantErrs: 0,
		},
		{
			name: "multiple errors accumulated",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "unknown-dist"},
					Providers: &ProvidersSpec{
						Inference: []ProviderConfig{{ID: "dup", Provider: "ollama"}},
						VectorIo: []ProviderConfig{{ID: "dup", Provider: "pgvector"}},
					},
					Resources: &ResourcesSpec{
						Models: []ModelConfig{{Name: "llama3", Provider: "nonexistent"}},
					},
				},
			},
			wantErrs: 3,
		},
		{
			name: "no providers or resources is valid",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
				},
			},
			wantErrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &OGXServerValidator{EmbeddedDistributionNames: knownNames}
			errs := v.collectValidationErrors(tt.server)
			if len(errs) != tt.wantErrs {
				t.Errorf("collectValidationErrors() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}
