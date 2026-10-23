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

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
		{"Pgvector default", (PgvectorProvider{}).DeriveID(), "remote-pgvector"},
		{"Milvus default", (MilvusProvider{}).DeriveID(), "remote-milvus"},
		{"Qdrant default", (QdrantProvider{}).DeriveID(), "remote-qdrant"},
		{"BraveSearch default", (BraveSearchProvider{}).DeriveID(), "remote-brave-search"},
		{"TavilySearch default", (TavilySearchProvider{}).DeriveID(), "remote-tavily-search"},
		{"MCP default", (ModelContextProtocolProvider{}).DeriveID(), "remote-model-context-protocol"},
		{"FileSearch default", (InlineFileSearchProvider{}).DeriveID(), "inline-file-search"},
		{"S3 default", (S3Provider{}).DeriveID(), "remote-s3"},
		{"LocalFS default", (InlineLocalFSProvider{}).DeriveID(), "inline-localfs"},
		{"Auto FileProcessor default", (InlineAutoFileProcessorProvider{}).DeriveID(), "inline-auto"},
		{"PyPDF FileProcessor default", (InlinePyPDFFileProcessorProvider{}).DeriveID(), "inline-pypdf"},
		{"MarkItDown FileProcessor default", (InlineMarkItDownFileProcessorProvider{}).DeriveID(), "inline-markitdown"},
		{"Docling FileProcessor default", (InlineDoclingFileProcessorProvider{}).DeriveID(), "inline-docling"},
		{"DoclingServe default", (DoclingServeProvider{}).DeriveID(), "remote-docling-serve"},
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
		assertSliceEqual(t, spec.VectorIo.Remote.IDs(), []string{"remote-pgvector"})
		assertSliceEqual(t, spec.Files.Remote.IDs(), []string{"remote-s3"})

		ids := spec.IDs()
		if len(ids) != 3 {
			t.Errorf("IDs() = %v, want 3 elements", ids)
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

func TestValidateVolumeTypes(t *testing.T) {
	tests := []struct {
		name      string
		server    *OGXServer
		wantErrs  int
		errSubstr string
	}{
		{
			name: "nil workload is valid",
			server: &OGXServer{
				Spec: OGXServerSpec{Distribution: DistributionSpec{Image: "x"}},
			},
			wantErrs: 0,
		},
		{
			name: "nil overrides is valid",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload:     &WorkloadSpec{},
				},
			},
			wantErrs: 0,
		},
		{
			name: "configMap volume is allowed",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{{
							Name:         "cfg",
							VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}},
						}},
					}},
				},
			},
			wantErrs: 0,
		},
		{
			name: "secret volume is allowed",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{{
							Name:         "sec",
							VolumeSource: corev1.VolumeSource{Secret: &corev1.SecretVolumeSource{}},
						}},
					}},
				},
			},
			wantErrs: 0,
		},
		{
			name: "emptyDir volume is allowed",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{{
							Name:         "tmp",
							VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
						}},
					}},
				},
			},
			wantErrs: 0,
		},
		{
			name: "persistentVolumeClaim is allowed",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{{
							Name:         "data",
							VolumeSource: corev1.VolumeSource{PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{ClaimName: "c"}},
						}},
					}},
				},
			},
			wantErrs: 0,
		},
		{
			name: "projected volume is allowed",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{{
							Name:         "proj",
							VolumeSource: corev1.VolumeSource{Projected: &corev1.ProjectedVolumeSource{}},
						}},
					}},
				},
			},
			wantErrs: 0,
		},
		{
			name: "downwardAPI volume is allowed",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{{
							Name:         "dapi",
							VolumeSource: corev1.VolumeSource{DownwardAPI: &corev1.DownwardAPIVolumeSource{}},
						}},
					}},
				},
			},
			wantErrs: 0,
		},
		{
			name: "hostPath volume is rejected",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{{
							Name:         "bad",
							VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}},
						}},
					}},
				},
			},
			wantErrs:  1,
			errSubstr: "disallowed volume source type",
		},
		{
			name: "nfs volume is rejected",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{{
							Name:         "nfs",
							VolumeSource: corev1.VolumeSource{NFS: &corev1.NFSVolumeSource{Server: "s", Path: "/"}},
						}},
					}},
				},
			},
			wantErrs:  1,
			errSubstr: "disallowed volume source type",
		},
		{
			name: "mixed allowed and disallowed volumes",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{
							{Name: "ok", VolumeSource: corev1.VolumeSource{ConfigMap: &corev1.ConfigMapVolumeSource{}}},
							{Name: "bad", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}},
						},
					}},
				},
			},
			wantErrs: 1,
		},
		{
			name: "multiple disallowed volumes",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Workload: &WorkloadSpec{Overrides: &WorkloadOverrides{
						Volumes: []corev1.Volume{
							{Name: "hp", VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}}},
							{Name: "nfs", VolumeSource: corev1.VolumeSource{NFS: &corev1.NFSVolumeSource{Server: "s", Path: "/"}}},
						},
					}},
				},
			},
			wantErrs: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateVolumeTypes(tt.server)
			if len(errs) != tt.wantErrs {
				t.Errorf("validateVolumeTypes() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
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
				VectorIo: &VectorIOProvidersSpec{
					Remote: &VectorIORemoteProviders{
						Pgvector: []PgvectorProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "dup1"},
							Password:           SecretKeyRef{Name: "s", Key: "k"},
						}},
						Qdrant: []QdrantProvider{{
							RoutedProviderBase: RoutedProviderBase{ID: "dup2"},
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

func TestValidateAdoptionAnnotations(t *testing.T) {
	tests := []struct {
		name        string
		server      *OGXServer
		wantErrs    int
		errContains string
	}{
		{
			name: "no adoption annotations is valid",
			server: &OGXServer{
				ObjectMeta: metav1.ObjectMeta{Name: "my-server"},
			},
			wantErrs: 0,
		},
		{
			name: "adopt-storage with different name is valid",
			server: &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "new-server",
					Annotations: map[string]string{AdoptStorageAnnotation: "old-server"},
				},
			},
			wantErrs: 0,
		},
		{
			name: "adopt-storage equals CR name is rejected",
			server: &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "my-server",
					Annotations: map[string]string{AdoptStorageAnnotation: "my-server"},
				},
			},
			wantErrs:    1,
			errContains: "must not equal the CR name",
		},
		{
			name: "adopt-networking equals CR name is rejected",
			server: &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "my-server",
					Annotations: map[string]string{AdoptNetworkingAnnotation: "my-server"},
				},
			},
			wantErrs:    1,
			errContains: "must not equal the CR name",
		},
		{
			name: "both annotations equal CR name gives two errors",
			server: &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Name: "my-server",
					Annotations: map[string]string{
						AdoptStorageAnnotation:    "my-server",
						AdoptNetworkingAnnotation: "my-server",
					},
				},
			},
			wantErrs: 2,
		},
		{
			name: "adopt-networking with different name is valid",
			server: &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "new-server",
					Annotations: map[string]string{AdoptNetworkingAnnotation: "old-server"},
				},
			},
			wantErrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateAdoptionAnnotations(tt.server)
			if len(errs) != tt.wantErrs {
				t.Errorf("validateAdoptionAnnotations() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
			if tt.errContains != "" && len(errs) > 0 {
				if !strings.Contains(errs[0].Detail, tt.errContains) {
					t.Errorf("error detail %q does not contain %q", errs[0].Detail, tt.errContains)
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
		{
			name: "self-adoption annotation is rejected",
			server: &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:        "my-server",
					Annotations: map[string]string{AdoptStorageAnnotation: "my-server"},
				},
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
				},
			},
			wantErrs: 1,
		},
		{
			name: "hostPath volume is rejected",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					Workload: &WorkloadSpec{
						Overrides: &WorkloadOverrides{
							Volumes: []corev1.Volume{{
								Name:         "bad",
								VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/"}},
							}},
						},
					},
				},
			},
			wantErrs: 1,
		},
		{
			name: "external access enabled without TLS is rejected",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					Network: &NetworkSpec{
						ExternalAccess: &ExternalAccessConfig{
							Enabled:  true,
							Hostname: "ogx.example.com",
						},
					},
				},
			},
			wantErrs: 1,
		},
		{
			name: "external access with TLS and hostname is valid",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					Network: &NetworkSpec{
						ExternalAccess: &ExternalAccessConfig{
							Enabled:  true,
							Hostname: "ogx.example.com",
							TLS:      &TLSSpec{SecretName: "ogx-tls"},
						},
					},
				},
			},
			wantErrs: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			v := &OGXServerValidator{KnownDistributionNames: knownNames}
			errs := v.collectValidationErrors(tt.server)
			if len(errs) != tt.wantErrs {
				t.Errorf("collectValidationErrors() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
			}
		})
	}
}

func TestValidateExternalAccess(t *testing.T) {
	tests := []struct {
		name      string
		server    *OGXServer
		wantErrs  int
		errSubstr string
	}{
		{
			name: "nil network is valid",
			server: &OGXServer{
				Spec: OGXServerSpec{Distribution: DistributionSpec{Image: "x"}},
			},
			wantErrs: 0,
		},
		{
			name: "disabled external access without TLS is valid",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Network: &NetworkSpec{
						ExternalAccess: &ExternalAccessConfig{Enabled: false},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "enabled with TLS and hostname is valid",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Network: &NetworkSpec{
						ExternalAccess: &ExternalAccessConfig{
							Enabled:  true,
							Hostname: "ogx.example.com",
							TLS:      &TLSSpec{SecretName: "ogx-tls"},
						},
					},
				},
			},
			wantErrs: 0,
		},
		{
			name: "enabled without TLS is rejected",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Network: &NetworkSpec{
						ExternalAccess: &ExternalAccessConfig{
							Enabled:  true,
							Hostname: "ogx.example.com",
						},
					},
				},
			},
			wantErrs:  1,
			errSubstr: "TLS secretName is required",
		},
		{
			name: "enabled without hostname is rejected",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Network: &NetworkSpec{
						ExternalAccess: &ExternalAccessConfig{
							Enabled: true,
							TLS:     &TLSSpec{SecretName: "ogx-tls"},
						},
					},
				},
			},
			wantErrs:  1,
			errSubstr: "hostname is required",
		},
		{
			name: "enabled without hostname or TLS gives two errors",
			server: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Image: "x"},
					Network: &NetworkSpec{
						ExternalAccess: &ExternalAccessConfig{Enabled: true},
					},
				},
			},
			wantErrs: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			errs := validateExternalAccess(tt.server)
			if len(errs) != tt.wantErrs {
				t.Errorf("validateExternalAccess() returned %d errors, want %d: %v", len(errs), tt.wantErrs, errs)
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

func TestValidate_ExternalAccessWarning(t *testing.T) {
	v := &OGXServerValidator{KnownDistributionNames: []string{"starter"}}

	tests := []struct {
		name                string
		network             *NetworkSpec
		praxisMode          *PraxisModeSpec
		wantExternalWarning bool
	}{
		{
			name:                "no network spec: no external access warning",
			network:             nil,
			praxisMode:          &PraxisModeSpec{Enabled: ptr(true)},
			wantExternalWarning: false,
		},
		{
			name:                "external access disabled: no external access warning",
			network:             &NetworkSpec{ExternalAccess: &ExternalAccessConfig{Enabled: false}},
			praxisMode:          &PraxisModeSpec{Enabled: ptr(true)},
			wantExternalWarning: false,
		},
		{
			name: "external access enabled + praxis mode enabled: warns but does not reject",
			network: &NetworkSpec{ExternalAccess: &ExternalAccessConfig{
				Enabled: true, Hostname: "ogx.example.com", TLS: &TLSSpec{SecretName: "ogx-tls"},
			}},
			praxisMode:          &PraxisModeSpec{Enabled: ptr(true)},
			wantExternalWarning: true,
		},
		{
			name: "external access enabled + praxisMode unset: no warning (legacy)",
			network: &NetworkSpec{ExternalAccess: &ExternalAccessConfig{
				Enabled: true, Hostname: "ogx.example.com", TLS: &TLSSpec{SecretName: "ogx-tls"},
			}},
			praxisMode:          nil,
			wantExternalWarning: false,
		},
		{
			name: "external access enabled + praxis mode disabled: no warning (legacy honors it)",
			network: &NetworkSpec{ExternalAccess: &ExternalAccessConfig{
				Enabled: true, Hostname: "ogx.example.com", TLS: &TLSSpec{SecretName: "ogx-tls"},
			}},
			praxisMode:          &PraxisModeSpec{Enabled: ptr(false)},
			wantExternalWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					Network:      tt.network,
					PraxisMode:   tt.praxisMode,
				},
			}

			warnings, err := v.ValidateCreate(t.Context(), server)
			if err != nil {
				t.Fatalf("ValidateCreate returned unexpected error: %v", err)
			}

			got := strings.Contains(strings.Join(warnings, " "), "externalAccess")
			if got != tt.wantExternalWarning {
				t.Errorf("ValidateCreate warnings = %v, wantExternalWarning %v", warnings, tt.wantExternalWarning)
			}
		})
	}
}

func TestValidate_PolicyDisabledWarning(t *testing.T) {
	v := &OGXServerValidator{KnownDistributionNames: []string{"starter"}}

	tests := []struct {
		name              string
		network           *NetworkSpec
		praxisMode        *PraxisModeSpec
		wantPolicyWarning bool
	}{
		{
			name:              "no network spec: no policy warning",
			network:           nil,
			praxisMode:        &PraxisModeSpec{Enabled: ptr(true)},
			wantPolicyWarning: false,
		},
		{
			name:              "policy enabled + praxis mode enabled: no policy warning",
			network:           &NetworkSpec{Policy: &NetworkPolicySpec{Enabled: ptr(true)}},
			praxisMode:        &PraxisModeSpec{Enabled: ptr(true)},
			wantPolicyWarning: false,
		},
		{
			name:              "policy unset + praxis mode enabled: no policy warning",
			network:           &NetworkSpec{Policy: &NetworkPolicySpec{}},
			praxisMode:        &PraxisModeSpec{Enabled: ptr(true)},
			wantPolicyWarning: false,
		},
		{
			name:              "policy disabled + praxis mode enabled: warns but does not reject",
			network:           &NetworkSpec{Policy: &NetworkPolicySpec{Enabled: ptr(false)}},
			praxisMode:        &PraxisModeSpec{Enabled: ptr(true)},
			wantPolicyWarning: true,
		},
		{
			name:              "policy disabled + praxisMode unset: no warning (legacy)",
			network:           &NetworkSpec{Policy: &NetworkPolicySpec{Enabled: ptr(false)}},
			praxisMode:        nil,
			wantPolicyWarning: false,
		},
		{
			name:              "policy disabled + praxis mode disabled: no warning (legacy)",
			network:           &NetworkSpec{Policy: &NetworkPolicySpec{Enabled: ptr(false)}},
			praxisMode:        &PraxisModeSpec{Enabled: ptr(false)},
			wantPolicyWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					Network:      tt.network,
					PraxisMode:   tt.praxisMode,
				},
			}

			warnings, err := v.ValidateCreate(t.Context(), server)
			if err != nil {
				t.Fatalf("ValidateCreate returned unexpected error: %v", err)
			}

			got := strings.Contains(strings.Join(warnings, " "), "spec.network.policy.enabled")
			if got != tt.wantPolicyWarning {
				t.Errorf("ValidateCreate warnings = %v, wantPolicyWarning %v", warnings, tt.wantPolicyWarning)
			}
		})
	}
}

func TestValidate_PraxisModeWarning(t *testing.T) {
	v := &OGXServerValidator{KnownDistributionNames: []string{"starter"}}

	tests := []struct {
		name        string
		praxisMode  *PraxisModeSpec
		wantWarning bool
	}{
		{
			name:        "praxis mode enabled",
			praxisMode:  &PraxisModeSpec{Enabled: ptr(true)},
			wantWarning: true,
		},
		{
			name:        "praxis mode with enabled omitted",
			praxisMode:  &PraxisModeSpec{},
			wantWarning: true,
		},
		{
			name:        "praxis mode unset",
			praxisMode:  nil,
			wantWarning: false,
		},
		{
			name:        "praxis mode disabled",
			praxisMode:  &PraxisModeSpec{Enabled: ptr(false)},
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					PraxisMode:   tt.praxisMode,
				},
			}

			warnings, err := v.ValidateCreate(t.Context(), server)
			if err != nil {
				t.Fatalf("ValidateCreate returned unexpected error: %v", err)
			}

			got := strings.Contains(strings.Join(warnings, " "), "Praxis mode is enabled (spec.praxisMode.enabled: true). The Responses API served by Praxis is Tech Preview.")
			if got != tt.wantWarning {
				t.Errorf("ValidateCreate warnings = %v, wantTechPreviewWarning %v", warnings, tt.wantWarning)
			}
		})
	}
}

func TestValidateUpdate_SoftRollbackWarning(t *testing.T) {
	v := &OGXServerValidator{KnownDistributionNames: []string{"starter"}}

	migrationJob := &MigrationJobSpec{
		Enabled: ptr(true),
		TargetConnectionString: &SecretKeyRef{
			Name: "praxis-pg",
			Key:  "url",
		},
	}
	requested := &OGXServer{
		Spec: OGXServerSpec{
			Distribution: DistributionSpec{Name: "starter"},
			PraxisMode: &PraxisModeSpec{
				Enabled:      ptr(true),
				MigrationJob: migrationJob,
			},
		},
	}

	tests := []struct {
		name        string
		oldObj      *OGXServer
		newObj      *OGXServer
		wantWarning bool
	}{
		{
			name:        "unchanged requested spec does not warn",
			oldObj:      requested,
			newObj:      requested,
			wantWarning: false,
		},
		{
			name:   "praxisMode.enabled true to false warns",
			oldObj: requested,
			newObj: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					PraxisMode: &PraxisModeSpec{
						Enabled:      ptr(false),
						MigrationJob: migrationJob,
					},
				},
			},
			wantWarning: true,
		},
		{
			name:   "migrationJob.enabled true to false warns",
			oldObj: requested,
			newObj: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					PraxisMode: &PraxisModeSpec{
						Enabled: ptr(true),
						MigrationJob: &MigrationJobSpec{
							Enabled:                ptr(false),
							TargetConnectionString: migrationJob.TargetConnectionString,
						},
					},
				},
			},
			wantWarning: true,
		},
		{
			name:   "removing migrationJob warns",
			oldObj: requested,
			newObj: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					PraxisMode:   &PraxisModeSpec{Enabled: ptr(true)},
				},
			},
			wantWarning: true,
		},
		{
			name: "never requested does not warn",
			oldObj: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					PraxisMode:   &PraxisModeSpec{Enabled: ptr(true)},
				},
			},
			newObj: &OGXServer{
				Spec: OGXServerSpec{
					Distribution: DistributionSpec{Name: "starter"},
					PraxisMode:   &PraxisModeSpec{Enabled: ptr(false)},
				},
			},
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			warnings, err := v.ValidateUpdate(t.Context(), tt.oldObj, tt.newObj)
			if err != nil {
				t.Fatalf("ValidateUpdate returned unexpected error: %v", err)
			}
			got := false
			for _, w := range warnings {
				if strings.Contains(w, "soft rollback") {
					got = true
				}
			}
			if got != tt.wantWarning {
				t.Errorf("ValidateUpdate rollback warning = %v, wantWarning %v; warnings=%v", got, tt.wantWarning, warnings)
			}
		})
	}
}

func TestValidateUpdate_ExternalAccessOnlyWhenChanged(t *testing.T) {
	v := &OGXServerValidator{}

	server := func(ea *ExternalAccessConfig, replicas int32) *OGXServer {
		return &OGXServer{
			Spec: OGXServerSpec{
				Distribution: DistributionSpec{Image: "x"},
				Network:      &NetworkSpec{ExternalAccess: ea},
				Workload:     &WorkloadSpec{Replicas: ptr(replicas)},
			},
		}
	}
	noTLS := &ExternalAccessConfig{Enabled: true}

	tests := []struct {
		name      string
		oldObj    *OGXServer
		newObj    *OGXServer
		wantErr   bool
		errSubstr string
	}{
		{
			name:    "unrelated edit to a server with external access and no TLS is allowed",
			oldObj:  server(noTLS, 1),
			newObj:  server(&ExternalAccessConfig{Enabled: true}, 2),
			wantErr: false,
		},
		{
			name:      "setting a hostname without TLS is rejected",
			oldObj:    server(noTLS, 1),
			newObj:    server(&ExternalAccessConfig{Enabled: true, Hostname: "ogx.example.com"}, 1),
			wantErr:   true,
			errSubstr: "TLS secretName is required",
		},
		{
			name:    "disabling external access is allowed",
			oldObj:  server(noTLS, 1),
			newObj:  server(&ExternalAccessConfig{Enabled: false}, 1),
			wantErr: false,
		},
		{
			name:    "adding hostname and TLS is allowed",
			oldObj:  server(noTLS, 1),
			newObj:  server(&ExternalAccessConfig{Enabled: true, Hostname: "ogx.example.com", TLS: &TLSSpec{SecretName: "ogx-tls"}}, 1),
			wantErr: false,
		},
		{
			name:    "unrelated edit with no network spec is allowed",
			oldObj:  &OGXServer{Spec: OGXServerSpec{Distribution: DistributionSpec{Image: "x"}}},
			newObj:  &OGXServer{Spec: OGXServerSpec{Distribution: DistributionSpec{Image: "y"}}},
			wantErr: false,
		},
		{
			name:      "enabling external access without TLS is rejected",
			oldObj:    server(nil, 1),
			newObj:    server(noTLS, 1),
			wantErr:   true,
			errSubstr: "hostname is required",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateUpdate(t.Context(), tt.oldObj, tt.newObj)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateUpdate error = %v, wantErr %v", err, tt.wantErr)
			}
			if tt.errSubstr != "" && (err == nil || !strings.Contains(err.Error(), tt.errSubstr)) {
				t.Errorf("ValidateUpdate error %q does not contain %q", err, tt.errSubstr)
			}
		})
	}
}

func TestValidateUpdate_VolumeTypesOnlyWhenChanged(t *testing.T) {
	v := &OGXServerValidator{}

	hostPath := corev1.Volume{
		Name:         "host",
		VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/data"}},
	}
	server := func(replicas int32, vols ...corev1.Volume) *OGXServer {
		return &OGXServer{
			Spec: OGXServerSpec{
				Distribution: DistributionSpec{Image: "x"},
				Workload: &WorkloadSpec{
					Replicas:  ptr(replicas),
					Overrides: &WorkloadOverrides{Volumes: vols},
				},
			},
		}
	}
	changedHostPath := hostPath
	changedHostPath.HostPath = &corev1.HostPathVolumeSource{Path: "/other"}
	renamedHostPath := hostPath
	renamedHostPath.Name = "host2"
	otherHostPath := corev1.Volume{
		Name:         "other",
		VolumeSource: corev1.VolumeSource{HostPath: &corev1.HostPathVolumeSource{Path: "/tmp"}},
	}

	tests := []struct {
		name    string
		oldObj  *OGXServer
		newObj  *OGXServer
		wantErr bool
	}{
		{
			name:    "unrelated edit keeps an existing disallowed volume",
			oldObj:  server(1, hostPath),
			newObj:  server(2, hostPath),
			wantErr: false,
		},
		{
			name:    "adding a disallowed volume is rejected",
			oldObj:  server(1),
			newObj:  server(1, hostPath),
			wantErr: true,
		},
		{
			name:    "renaming an existing disallowed volume is rejected",
			oldObj:  server(1, hostPath),
			newObj:  server(1, renamedHostPath),
			wantErr: true,
		},
		{
			name:    "reordering existing disallowed volumes is allowed",
			oldObj:  server(1, hostPath, otherHostPath),
			newObj:  server(1, otherHostPath, hostPath),
			wantErr: false,
		},
		{
			name:    "changing an existing disallowed volume is rejected",
			oldObj:  server(1, hostPath),
			newObj:  server(1, changedHostPath),
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := v.ValidateUpdate(t.Context(), tt.oldObj, tt.newObj)
			if (err != nil) != tt.wantErr {
				t.Fatalf("ValidateUpdate error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
