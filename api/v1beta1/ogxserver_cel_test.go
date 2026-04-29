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
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func TestCEL_DistributionSpec(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-dist")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name:   "image only is valid",
			mutate: func(_ *OGXServer) {},
		},
		{
			name: "name only is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Distribution = DistributionSpec{Name: "starter"}
			},
		},
		{
			name: "both name and image is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Distribution = DistributionSpec{Name: "starter", Image: "test:latest"}
			},
			wantError: "only one of name or image can be specified",
		},
		{
			name: "neither name nor image is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Distribution = DistributionSpec{}
			},
			wantError: "one of name or image must be specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_ProviderConfig(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-prov")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "remote prefix is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "remote::vllm"}},
				}
			},
		},
		{
			name: "inline prefix is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "inline::builtin"}},
				}
			},
		},
		{
			name: "no prefix is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "vllm"}},
				}
			},
			wantError: "provider must have a 'remote::' or 'inline::' prefix",
		},
		{
			name: "single colon prefix is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "remote:vllm"}},
				}
			},
			wantError: "provider must have a 'remote::' or 'inline::' prefix",
		},
		{
			name: "wrong case prefix is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "Remote::vllm"}},
				}
			},
			wantError: "provider must have a 'remote::' or 'inline::' prefix",
		},
		{
			name: "bare double colon prefix is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "::vllm"}},
				}
			},
			wantError: "provider must have a 'remote::' or 'inline::' prefix",
		},
		{
			name: "explicit non-empty id is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{ID: "my-vllm", Provider: "remote::vllm"}},
				}
			},
		},
		{
			name: "id omitted is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "remote::vllm"}},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_ModelConfig(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-model")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "all optional fields populated is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Resources = &ResourcesSpec{
					Models: []ModelConfig{{
						Name:         "llama3",
						Provider:     "vllm",
						ModelType:    "llm",
						Quantization: "int8",
					}},
				}
			},
		},
		{
			name: "only required name field is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Resources = &ResourcesSpec{
					Models: []ModelConfig{{Name: "llama3"}},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_KVStorageSpec(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-kv")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "sqlite with no endpoint is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					KV: &KVStorageSpec{Type: "sqlite"},
				}
			},
		},
		{
			name: "redis with endpoint is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					KV: &KVStorageSpec{Type: "redis", Endpoint: "redis://localhost:6379"},
				}
			},
		},
		{
			name: "redis without endpoint is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					KV: &KVStorageSpec{Type: "redis"},
				}
			},
			wantError: "endpoint is required when type is redis",
		},
		{
			name: "sqlite with endpoint is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					KV: &KVStorageSpec{Type: "sqlite", Endpoint: "redis://localhost:6379"},
				}
			},
			wantError: "endpoint is only valid when type is redis",
		},
		{
			name: "sqlite with password is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					KV: &KVStorageSpec{
						Type:     "sqlite",
						Password: &SecretKeyRef{Name: "secret", Key: "password"},
					},
				}
			},
			wantError: "password is only valid when type is redis",
		},
		{
			name: "redis with password and endpoint is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					KV: &KVStorageSpec{
						Type:     "redis",
						Endpoint: "redis://localhost:6379",
						Password: &SecretKeyRef{Name: "secret", Key: "password"},
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_SQLStorageSpec(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-sql")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "sqlite with no connectionString is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					SQL: &SQLStorageSpec{Type: "sqlite"},
				}
			},
		},
		{
			name: "postgres with connectionString is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					SQL: &SQLStorageSpec{
						Type:             "postgres",
						ConnectionString: &SecretKeyRef{Name: "secret", Key: "connstr"},
					},
				}
			},
		},
		{
			name: "postgres without connectionString is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					SQL: &SQLStorageSpec{Type: "postgres"},
				}
			},
			wantError: "connectionString is required when type is postgres",
		},
		{
			name: "sqlite with connectionString is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Storage = &StateStorageSpec{
					SQL: &SQLStorageSpec{
						Type:             "sqlite",
						ConnectionString: &SecretKeyRef{Name: "secret", Key: "connstr"},
					},
				}
			},
			wantError: "connectionString is only valid when type is postgres",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_PVCStorageSpec(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-pvc")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "custom mountPath is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Workload = &WorkloadSpec{
					Storage: &PVCStorageSpec{MountPath: "/data"},
				}
			},
		},
		{
			name: "positive size is valid",
			mutate: func(o *OGXServer) {
				q := resource.MustParse("10Gi")
				o.Spec.Workload = &WorkloadSpec{
					Storage: &PVCStorageSpec{Size: &q},
				}
			},
		},
		{
			name: "zero size is invalid",
			mutate: func(o *OGXServer) {
				q := resource.MustParse("0")
				o.Spec.Workload = &WorkloadSpec{
					Storage: &PVCStorageSpec{Size: &q},
				}
			},
			wantError: "size must be a positive quantity",
		},
		{
			name: "negative size is invalid",
			mutate: func(o *OGXServer) {
				q := resource.MustParse("-1Gi")
				o.Spec.Workload = &WorkloadSpec{
					Storage: &PVCStorageSpec{Size: &q},
				}
			},
			wantError: "size must be a positive quantity",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_PodDisruptionBudgetSpec(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-pdb")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "minAvailable only is valid",
			mutate: func(o *OGXServer) {
				v := intstr.FromInt32(1)
				o.Spec.Workload = &WorkloadSpec{
					PodDisruptionBudget: &PodDisruptionBudgetSpec{MinAvailable: &v},
				}
			},
		},
		{
			name: "maxUnavailable only is valid",
			mutate: func(o *OGXServer) {
				v := intstr.FromInt32(1)
				o.Spec.Workload = &WorkloadSpec{
					PodDisruptionBudget: &PodDisruptionBudgetSpec{MaxUnavailable: &v},
				}
			},
		},
		{
			name: "both set is invalid",
			mutate: func(o *OGXServer) {
				minVal := intstr.FromInt32(1)
				maxVal := intstr.FromInt32(1)
				o.Spec.Workload = &WorkloadSpec{
					PodDisruptionBudget: &PodDisruptionBudgetSpec{
						MinAvailable:   &minVal,
						MaxUnavailable: &maxVal,
					},
				}
			},
			wantError: "minAvailable and maxUnavailable are mutually exclusive",
		},
		{
			name: "neither set is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Workload = &WorkloadSpec{
					PodDisruptionBudget: &PodDisruptionBudgetSpec{},
				}
			},
			wantError: "at least one of minAvailable or maxUnavailable must be specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_AutoscalingSpec(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-hpa")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "maxReplicas greater than minReplicas is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Workload = &WorkloadSpec{
					Autoscaling: &AutoscalingSpec{
						MinReplicas: ptr(int32(2)),
						MaxReplicas: 5,
					},
				}
			},
		},
		{
			name: "maxReplicas equal to minReplicas is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Workload = &WorkloadSpec{
					Autoscaling: &AutoscalingSpec{
						MinReplicas: ptr(int32(3)),
						MaxReplicas: 3,
					},
				}
			},
		},
		{
			name: "maxReplicas less than minReplicas is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.Workload = &WorkloadSpec{
					Autoscaling: &AutoscalingSpec{
						MinReplicas: ptr(int32(5)),
						MaxReplicas: 2,
					},
				}
			},
			wantError: "maxReplicas must be greater than or equal to minReplicas",
		},
		{
			name: "minReplicas omitted is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Workload = &WorkloadSpec{
					Autoscaling: &AutoscalingSpec{MaxReplicas: 3},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_OGXServerSpec_OverrideConfigExclusivity(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-override")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "overrideConfig alone is valid",
			mutate: func(o *OGXServer) {
				o.Spec.OverrideConfig = &OverrideConfigSpec{ConfigMapName: "my-config"}
			},
		},
		{
			name: "overrideConfig with providers is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.OverrideConfig = &OverrideConfigSpec{ConfigMapName: "my-config"}
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "remote::vllm"}},
				}
			},
			wantError: "overrideConfig and providers are mutually exclusive",
		},
		{
			name: "overrideConfig with resources is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.OverrideConfig = &OverrideConfigSpec{ConfigMapName: "my-config"}
				o.Spec.Resources = &ResourcesSpec{
					Models: []ModelConfig{{Name: "llama3"}},
				}
			},
			wantError: "overrideConfig and resources are mutually exclusive",
		},
		{
			name: "overrideConfig with storage is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.OverrideConfig = &OverrideConfigSpec{ConfigMapName: "my-config"}
				o.Spec.Storage = &StateStorageSpec{
					KV: &KVStorageSpec{Type: "sqlite"},
				}
			},
			wantError: "overrideConfig and storage are mutually exclusive",
		},
		{
			name: "overrideConfig with disabledAPIs is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.OverrideConfig = &OverrideConfigSpec{ConfigMapName: "my-config"}
				o.Spec.DisabledAPIs = []string{"inference"}
			},
			wantError: "overrideConfig and disabledAPIs are mutually exclusive",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_OGXServerSpec_DisabledAPIsProviderConflict(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-disabled")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "disabledAPIs agents with inference provider has no conflict",
			mutate: func(o *OGXServer) {
				o.Spec.DisabledAPIs = []string{"agents"}
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "remote::vllm"}},
				}
			},
		},
		{
			name: "safety has no disabledAPIs conflict rule",
			mutate: func(o *OGXServer) {
				o.Spec.DisabledAPIs = []string{"agents"}
				o.Spec.Providers = &ProvidersSpec{
					Safety: []ProviderConfig{{Provider: "remote::llama-guard"}},
				}
			},
		},
		{
			name: "inference in both disabledAPIs and providers is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.DisabledAPIs = []string{"inference"}
				o.Spec.Providers = &ProvidersSpec{
					Inference: []ProviderConfig{{Provider: "remote::vllm"}},
				}
			},
			wantError: "inference cannot be both in providers and disabledAPIs",
		},
		{
			name: "vector_io in both disabledAPIs and providers is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.DisabledAPIs = []string{"vector_io"}
				o.Spec.Providers = &ProvidersSpec{
					VectorIo: []ProviderConfig{{Provider: "remote::pgvector"}},
				}
			},
			wantError: "vector_io cannot be both in providers and disabledAPIs",
		},
		{
			name: "tool_runtime in both disabledAPIs and providers is invalid",
			mutate: func(o *OGXServer) {
				o.Spec.DisabledAPIs = []string{"tool_runtime"}
				o.Spec.Providers = &ProvidersSpec{
					ToolRuntime: []ProviderConfig{{Provider: "remote::brave-search"}},
				}
			},
			wantError: "tool_runtime cannot be both in providers and disabledAPIs",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_ExternalAccessConfig(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-extaccess")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "hostname absent is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Network = &NetworkSpec{
					ExternalAccess: &ExternalAccessConfig{Enabled: true},
				}
			},
		},
		{
			name: "hostname set is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Network = &NetworkSpec{
					ExternalAccess: &ExternalAccessConfig{
						Enabled:  true,
						Hostname: "example.com",
					},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

func TestCEL_WorkloadOverrides(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-overrides")

	tests := []struct {
		name      string
		mutate    func(*OGXServer)
		wantError string
	}{
		{
			name: "serviceAccountName absent is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Workload = &WorkloadSpec{
					Overrides: &WorkloadOverrides{},
				}
			},
		},
		{
			name: "serviceAccountName set is valid",
			mutate: func(o *OGXServer) {
				o.Spec.Workload = &WorkloadSpec{
					Overrides: &WorkloadOverrides{ServiceAccountName: "my-sa"},
				}
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			obj := validOGXServer(uniqueName(), ns)
			tt.mutate(obj)
			err := k8sClient.Create(context.Background(), obj)
			if tt.wantError == "" {
				if err != nil {
					t.Fatalf("expected success, got: %v", err)
				}
				t.Cleanup(func() { _ = k8sClient.Delete(context.Background(), obj) })
			} else {
				requireCELError(t, err, tt.wantError)
			}
		})
	}
}

// TestCEL_EmptyStringViaUnstructured tests CEL rules of the form
// `!has(self.X) || self.X.size() > 0` that cannot be triggered via typed Go
// structs because `omitempty` strips empty strings from the JSON payload.
// These rules guard against raw JSON/YAML submissions (e.g. kubectl apply)
// that explicitly set an optional string field to "".
func TestCEL_EmptyStringViaUnstructured(t *testing.T) {
	ns := createCELTestNamespace(t, "cel-unstr")

	tests := []struct {
		name      string
		mutate    func(map[string]any)
		wantError string
	}{
		{
			name: "provider config with empty id is invalid",
			mutate: func(raw map[string]any) {
				setNestedField(raw, []any{
					map[string]any{"id": "", "provider": "remote::vllm"},
				}, "spec", "providers", "inference")
			},
			wantError: "id must not be empty if specified",
		},
		{
			name: "model config with empty provider is invalid",
			mutate: func(raw map[string]any) {
				setNestedField(raw, []any{
					map[string]any{"name": "llama3", "provider": ""},
				}, "spec", "resources", "models")
			},
			wantError: "provider must not be empty if specified",
		},
		{
			name: "model config with empty modelType is invalid",
			mutate: func(raw map[string]any) {
				setNestedField(raw, []any{
					map[string]any{"name": "llama3", "modelType": ""},
				}, "spec", "resources", "models")
			},
			wantError: "modelType must not be empty if specified",
		},
		{
			name: "model config with empty quantization is invalid",
			mutate: func(raw map[string]any) {
				setNestedField(raw, []any{
					map[string]any{"name": "llama3", "quantization": ""},
				}, "spec", "resources", "models")
			},
			wantError: "quantization must not be empty if specified",
		},
		{
			name: "external access with empty hostname is invalid",
			mutate: func(raw map[string]any) {
				setNestedField(raw, map[string]any{
					"enabled": true, "hostname": "",
				}, "spec", "network", "externalAccess")
			},
			wantError: "hostname must not be empty if specified",
		},
		{
			name: "pvc storage with empty mountPath is invalid",
			mutate: func(raw map[string]any) {
				setNestedField(raw, map[string]any{
					"mountPath": "",
				}, "spec", "workload", "storage")
			},
			wantError: "mountPath must not be empty if specified",
		},
		{
			name: "workload overrides with empty serviceAccountName is invalid",
			mutate: func(raw map[string]any) {
				setNestedField(raw, map[string]any{
					"serviceAccountName": "",
				}, "spec", "workload", "overrides")
			},
			wantError: "serviceAccountName must not be empty if specified",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			raw := validUnstructuredOGXServer(t, uniqueName(), ns)
			tt.mutate(raw)
			err := createUnstructured(t, raw)
			requireAPIError(t, err, tt.wantError)
		})
	}
}
