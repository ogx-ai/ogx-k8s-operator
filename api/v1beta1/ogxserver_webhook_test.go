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

func TestDeriveID(t *testing.T) {
	tests := []struct {
		name string
		got  string
		want string
	}{
		{"VLLM default", (VLLMProvider{}).DeriveID(), "remote-vllm"},
		{"VLLM explicit", (VLLMProvider{RoutedProviderBase: RoutedProviderBase{ID: "my-vllm"}}).DeriveID(), "my-vllm"},
		{"OpenAI default", (OpenAIProvider{}).DeriveID(), "remote-openai"},
		{"Azure default", (AzureProvider{}).DeriveID(), "remote-azure"},
		{"Bedrock default", (BedrockProvider{}).DeriveID(), "remote-bedrock"},
		{"VertexAI default", (VertexAIProvider{}).DeriveID(), "remote-vertexai"},
		{"Watsonx default", (WatsonxProvider{}).DeriveID(), "remote-watsonx"},
		{"SentenceTransformers default", (InlineSentenceTransformersProvider{}).DeriveID(), "inline-sentence-transformers"},
		{"Pgvector default", (PgvectorProvider{}).DeriveID(), "remote-pgvector"},
		{"Milvus default", (MilvusProvider{}).DeriveID(), "remote-milvus"},
		{"Qdrant default", (QdrantProvider{}).DeriveID(), "remote-qdrant"},
		{"BraveSearch default", (BraveSearchProvider{}).DeriveID(), "remote-brave-search"},
		{"TavilySearch default", (TavilySearchProvider{}).DeriveID(), "remote-tavily-search"},
		{"MCP default", (ModelContextProtocolProvider{}).DeriveID(), "remote-model-context-protocol"},
		{"FileSearch default", (InlineFileSearchProvider{}).DeriveID(), "inline-file-search"},
		{"S3 default", (S3Provider{}).DeriveID(), "remote-s3"},
		{"LocalFS default", (InlineLocalFSProvider{}).DeriveID(), "inline-localfs"},
		{"Custom remote", (CustomProvider{Type: "remote::llama-guard"}).DeriveID(), "remote-llama-guard"},
		{"Custom inline", (CustomProvider{Type: "inline::my-thing"}).DeriveID(), "inline-my-thing"},
		{"Custom explicit ID", (CustomProvider{RoutedProviderBase: RoutedProviderBase{ID: "my-guard"}, Type: "remote::llama-guard"}).DeriveID(), "my-guard"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.got != tt.want {
				t.Errorf("DeriveID() = %q, want %q", tt.got, tt.want)
			}
		})
	}
}

func TestProvidersSpecIDs(t *testing.T) {
	t.Run("nil spec", func(t *testing.T) {
		var spec *ProvidersSpec
		if ids := spec.IDs(); len(ids) != 0 {
			t.Errorf("IDs() = %v, want empty", ids)
		}
	})

	t.Run("empty spec", func(t *testing.T) {
		spec := &ProvidersSpec{}
		if ids := spec.IDs(); len(ids) != 0 {
			t.Errorf("IDs() = %v, want empty", ids)
		}
	})

	t.Run("single VLLM provider", func(t *testing.T) {
		spec := &ProvidersSpec{
			Inference: &InferenceProvidersSpec{
				Remote: &InferenceRemoteProviders{
					VLLM: []VLLMProvider{{Endpoint: "https://vllm:8000"}},
				},
			},
		}
		assertSliceEqual(t, spec.Inference.Remote.IDs(), []string{"remote-vllm"})
		assertSliceEqual(t, spec.IDs(), []string{"remote-vllm"})
	})

	t.Run("multiple providers across API types", func(t *testing.T) {
		spec := &ProvidersSpec{
			Inference: &InferenceProvidersSpec{
				Remote: &InferenceRemoteProviders{
					VLLM: []VLLMProvider{{
						RoutedProviderBase: RoutedProviderBase{ID: "vllm-gpu"},
						Endpoint:           "https://vllm:8000",
					}},
				},
				Inline: &InferenceInlineProviders{
					SentenceTransformers: []InlineSentenceTransformersProvider{{}},
				},
			},
			VectorIo: &VectorIOProvidersSpec{
				Remote: &VectorIORemoteProviders{
					Pgvector: []PgvectorProvider{{Password: SecretKeyRef{Name: "s", Key: "k"}}},
				},
			},
			Files: &FilesProvidersSpec{
				Remote: &FilesRemoteProviders{
					S3: &S3Provider{BucketName: "my-bucket"},
				},
			},
		}
		assertSliceEqual(t, spec.Inference.Remote.IDs(), []string{"vllm-gpu"})
		assertSliceEqual(t, spec.Inference.Inline.IDs(), []string{"inline-sentence-transformers"})
		assertSliceEqual(t, spec.VectorIo.Remote.IDs(), []string{"remote-pgvector"})
		assertSliceEqual(t, spec.Files.Remote.IDs(), []string{"remote-s3"})

		ids := spec.IDs()
		if len(ids) != 4 {
			t.Errorf("IDs() = %v, want 4 elements", ids)
		}
	})

	t.Run("duplicate IDs returned", func(t *testing.T) {
		spec := &ProvidersSpec{
			Inference: &InferenceProvidersSpec{
				Remote: &InferenceRemoteProviders{
					VLLM: []VLLMProvider{
						{Endpoint: "https://vllm1:8000"},
						{Endpoint: "https://vllm2:8000"},
					},
				},
			},
		}
		assertSliceEqual(t, spec.Inference.Remote.IDs(), []string{"remote-vllm", "remote-vllm"})
	})
}

func assertSliceEqual(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("got %v (len %d), want %v (len %d)", got, len(got), want, len(want))
	}
	for i := range got {
		if got[i] != want[i] {
			t.Errorf("[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestValidateProviderIDs(t *testing.T) {
	tests := []struct {
		name      string
		providers *ProvidersSpec
		wantErrs  int
		errSubstr string
	}{
		{
			name:      "empty providers",
			providers: &ProvidersSpec{},
			wantErrs:  0,
		},
		{
			name: "no collision across API types",
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{Endpoint: "https://vllm:8000"}},
					},
				},
				VectorIo: &VectorIOProvidersSpec{
					Remote: &VectorIORemoteProviders{
						Pgvector: []PgvectorProvider{{Password: SecretKeyRef{Name: "s", Key: "k"}}},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "collision via explicit ID across API types",
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "shared-id"},
							Endpoint:           "https://vllm:8000",
						}},
					},
				},
				VectorIo: &VectorIOProvidersSpec{
					Remote: &VectorIORemoteProviders{
						Pgvector: []PgvectorProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "shared-id"},
							Password:           SecretKeyRef{Name: "s", Key: "k"},
						}},
					},
				},
			},
			wantErrs:  1,
			errSubstr: "duplicate provider ID",
		},
		{
			name: "collision via custom provider derived IDs",
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						Custom: []CustomProvider{{Type: "remote::my-model"}},
					},
				},
				ToolRuntime: &ToolRuntimeProvidersSpec{
					Remote: &ToolRuntimeRemoteProviders{
						Custom: []CustomProvider{{Type: "remote::my-model"}},
					},
				},
			},
			wantErrs:  1,
			errSubstr: "duplicate provider ID",
		},
		{
			name: "safety providers included in check",
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "my-provider"},
							Endpoint:           "https://vllm:8000",
						}},
					},
				},
				Safety: &SafetyProvidersSpec{
					Remote: &SafetyRemoteProviders{
						Custom: []CustomProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "my-provider"},
							Type:               "remote::llama-guard",
						}},
					},
				},
			},
			wantErrs:  1,
			errSubstr: "duplicate provider ID",
		},
		{
			name: "multiple collisions",
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "dup1"},
							Endpoint:           "https://vllm:8000",
						}},
						OpenAI: []OpenAIProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "dup2"},
							APIKey:             SecretKeyRef{Name: "s", Key: "k"},
						}},
					},
				},
				Safety: &SafetyProvidersSpec{
					Remote: &SafetyRemoteProviders{
						Custom: []CustomProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "dup1"},
							Type:               "remote::guard",
						}},
					},
				},
				VectorIo: &VectorIOProvidersSpec{
					Remote: &VectorIORemoteProviders{
						Pgvector: []PgvectorProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "dup2"},
							Password:           SecretKeyRef{Name: "s", Key: "k"},
						}},
					},
				},
			},
			wantErrs: 2,
		},
		{
			name: "multi-instance without explicit IDs errors",
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{
							{Endpoint: "https://vllm1:8000"},
							{Endpoint: "https://vllm2:8000"},
						},
					},
				},
			},
			wantErrs:  1,
			errSubstr: "duplicate provider ID",
		},
		{
			name: "multi-instance with explicit IDs succeeds",
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{
							{RoutedProviderBase: RoutedProviderBase{ID: "vllm-gpu"}, Endpoint: "https://vllm1:8000"},
							{RoutedProviderBase: RoutedProviderBase{ID: "vllm-cpu"}, Endpoint: "https://vllm2:8000"},
						},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "singleton files providers do not need IDs",
			providers: &ProvidersSpec{
				Files: &FilesProvidersSpec{
					Remote: &FilesRemoteProviders{
						S3: &S3Provider{BucketName: "my-bucket"},
					},
					Inline: &FilesInlineProviders{
						LocalFS: &InlineLocalFSProvider{},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "inline and remote providers coexist",
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{Endpoint: "https://vllm:8000"}},
					},
					Inline: &InferenceInlineProviders{
						SentenceTransformers: []InlineSentenceTransformersProvider{{}},
					},
				},
			},
			wantErrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateProviderIDs(tt.providers)
			if len(errs) != tt.wantErrs {
				t.Errorf("validateProviderIDs() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
			if tt.errSubstr != "" && len(errs) > 0 {
				found := false
				for _, e := range errs {
					if strings.Contains(e.Detail, tt.errSubstr) {
						found = true
						break
					}
				}
				if !found {
					t.Errorf("no error contains %q; errors: %v", tt.errSubstr, errs)
				}
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
			name: "valid provider reference by derived ID",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3", Provider: "remote-vllm"}},
			},
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{Endpoint: "https://vllm:8000"}},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "unknown provider reference",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3", Provider: "nonexistent"}},
			},
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{Endpoint: "https://vllm:8000"}},
					},
				},
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
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{Endpoint: "https://vllm:8000"}},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "reference to provider with explicit ID",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3", Provider: "my-vllm"}},
			},
			providers: &ProvidersSpec{
				Inference: &InferenceProvidersSpec{
					Remote: &InferenceRemoteProviders{
						VLLM: []VLLMProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "my-vllm"},
							Endpoint:           "https://vllm:8000",
						}},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "reference to safety custom provider",
			resources: &ResourcesSpec{
				Models: []ModelConfig{{Name: "llama3", Provider: "remote-llama-guard"}},
			},
			providers: &ProvidersSpec{
				Safety: &SafetyProvidersSpec{
					Remote: &SafetyRemoteProviders{
						Custom: []CustomProvider{{Type: "remote::llama-guard"}},
					},
				},
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
						Inference: &InferenceProvidersSpec{
							Remote: &InferenceRemoteProviders{
								VLLM: []VLLMProvider{{Endpoint: "https://vllm:8000"}},
							},
						},
					},
					Resources: &ResourcesSpec{
						Models: []ModelConfig{{Name: "llama3", Provider: "remote-vllm"}},
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
						Inference: &InferenceProvidersSpec{
							Remote: &InferenceRemoteProviders{
								VLLM: []VLLMProvider{{
									RoutedProviderBase: RoutedProviderBase{ID: "dup"},
									Endpoint:           "https://vllm:8000",
								}},
							},
						},
						VectorIo: &VectorIOProvidersSpec{
							Remote: &VectorIORemoteProviders{
								Pgvector: []PgvectorProvider{{
									RoutedProviderBase: RoutedProviderBase{ID: "dup"},
									Password:           SecretKeyRef{Name: "s", Key: "k"},
								}},
							},
						},
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
