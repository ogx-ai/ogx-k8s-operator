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

package config

import (
	"errors"
	"strings"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
)

func TestParseBaseConfig(t *testing.T) { //nolint:cyclop,gocognit // table-driven test with inline assertions
	tests := []struct {
		name    string
		input   string
		wantErr bool
		check   func(t *testing.T, cfg *BaseConfig)
	}{
		{
			name: "valid base config",
			input: `version: '2'
distro_name: starter
image_name: starter
apis:
- inference
- vector_io
providers:
  inference:
  - provider_id: remote-vllm
    provider_type: remote::vllm
    config:
      url: http://vllm:8000
registered_resources:
  models:
  - model_id: llama3
    provider_id: remote-vllm
    model_type: llm
`,
			check: func(t *testing.T, cfg *BaseConfig) {
				t.Helper()
				if cfg.Version != "2" {
					t.Errorf("expected version '2', got %q", cfg.Version)
				}
				if cfg.DistroName != "starter" {
					t.Errorf("expected distro_name 'starter', got %q", cfg.DistroName)
				}
				if cfg.ImageName != "starter" {
					t.Errorf("expected image_name 'starter', got %q", cfg.ImageName)
				}
				if len(cfg.APIs) != 2 {
					t.Fatalf("expected 2 APIs, got %d", len(cfg.APIs))
				}
				if cfg.APIs[0] != "inference" {
					t.Errorf("expected first API 'inference', got %q", cfg.APIs[0])
				}
				if len(cfg.Providers["inference"]) != 1 {
					t.Fatalf("expected 1 inference provider, got %d", len(cfg.Providers["inference"]))
				}
				if cfg.Providers["inference"][0].ProviderID != "remote-vllm" {
					t.Errorf("expected provider_id 'remote-vllm', got %q", cfg.Providers["inference"][0].ProviderID)
				}
				if cfg.RegisteredResources == nil || len(cfg.RegisteredResources.Models) != 1 {
					t.Fatalf("expected 1 registered resource model, got %+v", cfg.RegisteredResources)
				}
				firstModel, ok := cfg.RegisteredResources.Models[0].(map[string]interface{})
				if !ok || firstModel["model_id"] != "llama3" {
					t.Errorf("expected registered resource model 'llama3', got %+v", cfg.RegisteredResources.Models[0])
				}
			},
		},
		{
			name:    "missing version",
			input:   `image_name: test`,
			wantErr: true,
		},
		{
			name:    "invalid yaml",
			input:   `{invalid`,
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg, err := ParseBaseConfig([]byte(tt.input))
			if tt.wantErr {
				if err == nil {
					t.Fatal("expected error, got nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.check != nil {
				tt.check(t, cfg)
			}
		})
	}
}

func TestMergeProviders(t *testing.T) {
	base := map[string][]ConfigProvider{
		"inference": {
			{ProviderID: "base-vllm", ProviderType: "remote::vllm"},
		},
		"vector_io": {
			{ProviderID: "base-pgvector", ProviderType: "remote::pgvector"},
		},
	}

	user := map[string][]ConfigProvider{
		"inference": {
			{ProviderID: "user-openai", ProviderType: "remote::openai"},
		},
	}

	merged := MergeProviders(base, user)

	// User inference should replace base inference
	if len(merged["inference"]) != 1 || merged["inference"][0].ProviderID != "user-openai" {
		t.Errorf("expected user inference to replace base, got %+v", merged["inference"])
	}

	// Base vector_io should remain
	if len(merged["vector_io"]) != 1 || merged["vector_io"][0].ProviderID != "base-pgvector" {
		t.Errorf("expected base vector_io to remain, got %+v", merged["vector_io"])
	}
}

func TestMergeProviders_NilCases(t *testing.T) {
	providers := map[string][]ConfigProvider{
		"inference": {{ProviderID: "test"}},
	}

	// nil user returns base
	if result := MergeProviders(providers, nil); len(result) != 1 {
		t.Error("expected base returned when user is nil")
	}

	// nil base returns user
	if result := MergeProviders(nil, providers); len(result) != 1 {
		t.Error("expected user returned when base is nil")
	}
}

func TestMergeAPIs(t *testing.T) {
	base := []string{"inference", "vector_io", "tool_runtime", "agents"}
	disabled := []string{"agents", "vector_io"}

	result := MergeAPIs(base, disabled)

	expected := map[string]bool{"inference": true, "tool_runtime": true}
	if len(result) != 2 {
		t.Fatalf("expected 2 APIs, got %d: %v", len(result), result)
	}
	for _, api := range result {
		if !expected[api] {
			t.Errorf("unexpected API %q in result", api)
		}
	}
}

func TestMergeAPIs_NoDisabled(t *testing.T) {
	base := []string{"inference", "vector_io"}
	result := MergeAPIs(base, nil)
	if len(result) != 2 {
		t.Errorf("expected all base APIs when no disabled, got %v", result)
	}
}

func TestExpandResources(t *testing.T) {
	providers := map[string][]ConfigProvider{
		"inference": {
			{ProviderID: "vllm-1", ProviderType: "remote::vllm"},
		},
	}

	contextLen := 8192
	resources := &ogxiov1beta1.ResourcesSpec{
		Models: []ogxiov1beta1.ModelConfig{
			{
				Name:          "llama3",
				Provider:      "vllm-1",
				ModelType:     defaultModelType,
				ContextLength: &contextLen,
			},
			{
				Name: "llama3-embed",
			},
		},
	}

	models, err := ExpandResources(resources, providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) != 2 {
		t.Fatalf("expected 2 models, got %d", len(models))
	}

	// First model: explicit provider
	if models[0].ModelID != "llama3" {
		t.Errorf("expected model_id 'llama3', got %q", models[0].ModelID)
	}
	if models[0].ProviderID != "vllm-1" {
		t.Errorf("expected provider_id 'vllm-1', got %q", models[0].ProviderID)
	}
	if models[0].ModelType != defaultModelType {
		t.Errorf("expected model_type 'llm', got %q", models[0].ModelType)
	}
	if models[0].ContextLength == nil || *models[0].ContextLength != 8192 {
		t.Errorf("expected context_length 8192, got %v", models[0].ContextLength)
	}

	// Second model: defaults to first inference provider
	if models[1].ProviderID != "vllm-1" {
		t.Errorf("expected default provider_id 'vllm-1', got %q", models[1].ProviderID)
	}
	if models[1].ModelType != defaultModelType {
		t.Errorf("expected default model_type 'llm', got %q", models[1].ModelType)
	}
}

func TestExpandResources_NilSpec(t *testing.T) {
	models, err := ExpandResources(nil, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models != nil {
		t.Errorf("expected nil, got %v", models)
	}
}

func TestExpandResources_NoProviderAndNoInference(t *testing.T) {
	resources := &ogxiov1beta1.ResourcesSpec{
		Models: []ogxiov1beta1.ModelConfig{
			{Name: "llama3"},
		},
	}
	// No providers at all
	_, err := ExpandResources(resources, map[string][]ConfigProvider{})
	if err == nil {
		t.Fatal("expected error when no inference providers")
	}
}

func TestApplyStorage_Nil(t *testing.T) {
	result := ApplyStorage(nil)
	if result != nil {
		t.Errorf("expected nil for nil storage spec, got %v", result)
	}
}

func TestApplyStorage_SQLiteDefaults(t *testing.T) {
	storage := &ogxiov1beta1.StateStorageSpec{
		KV:  &ogxiov1beta1.KVStorageSpec{Type: "sqlite"},
		SQL: &ogxiov1beta1.SQLStorageSpec{Type: "sqlite"},
	}

	result := ApplyStorage(storage)
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	backends, ok := result["backends"].(map[string]interface{})
	if !ok {
		t.Fatal("expected backends map")
	}

	kvBackend, ok := backends["kv_default"].(map[string]interface{})
	if !ok {
		t.Fatal("expected kv_default map")
	}
	if kvBackend["type"] != "kv_sqlite" {
		t.Errorf("expected kv_sqlite, got %v", kvBackend["type"])
	}

	sqlBackend, ok := backends["sql_default"].(map[string]interface{})
	if !ok {
		t.Fatal("expected sql_default map")
	}
	if sqlBackend["type"] != "sql_sqlite" {
		t.Errorf("expected sql_sqlite, got %v", sqlBackend["type"])
	}
}

func TestApplyStorage_Redis(t *testing.T) {
	storage := &ogxiov1beta1.StateStorageSpec{
		KV: &ogxiov1beta1.KVStorageSpec{
			Type:     "redis",
			Endpoint: "redis://my-redis:6379",
			Password: &ogxiov1beta1.SecretKeyRef{
				Name: "redis-secret",
				Key:  "password",
			},
		},
	}

	result := ApplyStorage(storage)
	backends, ok := result["backends"].(map[string]interface{})
	if !ok {
		t.Fatal("expected backends to be a map")
	}
	kvBackend, ok := backends["kv_default"].(map[string]interface{})
	if !ok {
		t.Fatal("expected kv_default to be a map")
	}

	if kvBackend["type"] != "kv_redis" {
		t.Errorf("expected kv_redis, got %v", kvBackend["type"])
	}
	if kvBackend["host"] != "redis://my-redis:6379" {
		t.Errorf("expected endpoint, got %v", kvBackend["host"])
	}
}

func TestExpandProviders_Nil(t *testing.T) {
	result, err := ExpandProviders(nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil for nil providers, got %v", result)
	}
}

func TestExpandProviders_InferenceVLLM(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		Inference: &ogxiov1beta1.InferenceProvidersSpec{
			Remote: &ogxiov1beta1.InferenceRemoteProviders{
				VLLM: []ogxiov1beta1.VLLMProvider{
					{
						Endpoint: "https://vllm.example.com:8000",
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}

	inferenceProviders := result["inference"]
	if len(inferenceProviders) != 1 {
		t.Fatalf("expected 1 inference provider, got %d", len(inferenceProviders))
	}

	p := inferenceProviders[0]
	if p.ProviderType != "remote::vllm" {
		t.Errorf("expected provider_type 'remote::vllm', got %q", p.ProviderType)
	}
	if p.Config["url"] != "https://vllm.example.com:8000" {
		t.Errorf("expected url, got %v", p.Config["url"])
	}
}

func TestExpandProviders_InferenceOpenAI(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		Inference: &ogxiov1beta1.InferenceProvidersSpec{
			Remote: &ogxiov1beta1.InferenceRemoteProviders{
				OpenAI: []ogxiov1beta1.OpenAIProvider{
					{
						Endpoint: "https://api.openai.com/v1",
						APIKey: ogxiov1beta1.SecretKeyRef{
							Name: "openai-secret",
							Key:  "api-key",
						},
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	inferenceProviders := result["inference"]
	if len(inferenceProviders) != 1 {
		t.Fatalf("expected 1 inference provider, got %d", len(inferenceProviders))
	}

	p := inferenceProviders[0]
	if p.ProviderType != "remote::openai" {
		t.Errorf("expected provider_type 'remote::openai', got %q", p.ProviderType)
	}
}

func TestCollectSecretRefs_Nil(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{}
	envVars := CollectSecretRefs(spec)
	if len(envVars) != 0 {
		t.Errorf("expected no env vars for empty spec, got %d", len(envVars))
	}
}

func TestCollectSecretRefs_StorageRedis(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{
		Storage: &ogxiov1beta1.StateStorageSpec{
			KV: &ogxiov1beta1.KVStorageSpec{
				Type:     "redis",
				Endpoint: "redis://redis:6379",
				Password: &ogxiov1beta1.SecretKeyRef{
					Name: "redis-secret",
					Key:  "password",
				},
			},
		},
	}

	envVars := CollectSecretRefs(spec)
	if len(envVars) == 0 {
		t.Fatal("expected at least 1 env var for Redis password")
	}

	found := false
	for _, ev := range envVars {
		if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil {
			if ev.ValueFrom.SecretKeyRef.Name == "redis-secret" &&
				ev.ValueFrom.SecretKeyRef.Key == "password" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected env var referencing redis-secret/password")
	}
}

func TestCollectSecretRefs_StoragePostgres(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{
		Storage: &ogxiov1beta1.StateStorageSpec{
			SQL: &ogxiov1beta1.SQLStorageSpec{
				Type: "postgres",
				ConnectionString: &ogxiov1beta1.SecretKeyRef{
					Name: "pg-secret",
					Key:  "conn-string",
				},
			},
		},
	}

	envVars := CollectSecretRefs(spec)
	if len(envVars) == 0 {
		t.Fatal("expected at least 1 env var for Postgres connection string")
	}

	found := false
	for _, ev := range envVars {
		if ev.ValueFrom != nil && ev.ValueFrom.SecretKeyRef != nil {
			if ev.ValueFrom.SecretKeyRef.Name == "pg-secret" &&
				ev.ValueFrom.SecretKeyRef.Key == "conn-string" {
				found = true
				break
			}
		}
	}
	if !found {
		t.Error("expected env var referencing pg-secret/conn-string")
	}
}

func TestGenerateConfig_Basic(t *testing.T) {
	baseConfig := `version: '2'
distro_name: remote-vllm
image_name: remote-vllm
apis:
- inference
- vector_io
providers:
  inference:
  - provider_id: default-vllm
    provider_type: remote::vllm
    config:
      url: http://default:8000
models:
- model_id: default-model
  provider_id: default-vllm
  model_type: llm
vector_stores:
  default_provider_id: faiss
`

	spec := &ogxiov1beta1.OGXServerSpec{
		Distribution: ogxiov1beta1.DistributionSpec{Name: "remote-vllm"},
		Providers: &ogxiov1beta1.ProvidersSpec{
			Inference: &ogxiov1beta1.InferenceProvidersSpec{
				Remote: &ogxiov1beta1.InferenceRemoteProviders{
					VLLM: []ogxiov1beta1.VLLMProvider{
						{
							Endpoint: "https://my-vllm:8000",
						},
					},
				},
			},
		},
		Resources: &ogxiov1beta1.ResourcesSpec{
			Models: []ogxiov1beta1.ModelConfig{
				{Name: "llama3.2-8b"},
			},
		},
	}

	generated, err := GenerateConfig(spec, []byte(baseConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if generated.ConfigYAML == "" {
		t.Error("expected non-empty ConfigYAML")
	}
	if generated.ContentHash == "" {
		t.Error("expected non-empty ContentHash")
	}
	if generated.ProviderCount < 1 {
		t.Errorf("expected at least 1 provider, got %d", generated.ProviderCount)
	}
	if generated.ResourceCount < 1 {
		t.Errorf("expected at least 1 resource, got %d", generated.ResourceCount)
	}
	if generated.ConfigVersion != 2 {
		t.Errorf("expected config version 2, got %d", generated.ConfigVersion)
	}
	if !strings.Contains(generated.ConfigYAML, "distro_name: remote-vllm") {
		t.Errorf("expected generated config to preserve distro_name, got:\n%s", generated.ConfigYAML)
	}
	if !strings.Contains(generated.ConfigYAML, "registered_resources:") {
		t.Errorf("expected generated config to use registered_resources, got:\n%s", generated.ConfigYAML)
	}
	if !strings.Contains(generated.ConfigYAML, "vector_stores:") {
		t.Errorf("expected generated config to preserve unrelated top-level sections, got:\n%s", generated.ConfigYAML)
	}
}

func TestGenerateConfig_DisabledAPIs(t *testing.T) {
	baseConfig := `version: '2'
apis:
- inference
- vector_io
- tool_runtime
providers:
  inference:
  - provider_id: vllm
    provider_type: remote::vllm
    config: {}
`

	spec := &ogxiov1beta1.OGXServerSpec{
		DisabledAPIs: []string{"vector_io", "tool_runtime"},
	}

	generated, err := GenerateConfig(spec, []byte(baseConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if generated.ConfigYAML == "" {
		t.Error("expected non-empty ConfigYAML")
	}
}

func TestGenerateConfig_DisabledAPIs_AllRemoved(t *testing.T) {
	baseConfig := `version: '2'
apis:
- inference
`

	spec := &ogxiov1beta1.OGXServerSpec{
		DisabledAPIs: []string{"inference"},
	}

	generated, err := GenerateConfig(spec, []byte(baseConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if strings.Contains(generated.ConfigYAML, "apis:") {
		t.Errorf("expected apis section to be removed when all APIs are disabled, got:\n%s", generated.ConfigYAML)
	}
}

func TestGenerateConfig_ContentHashDeterministic(t *testing.T) {
	baseConfig := `version: '2'
providers:
  inference:
  - provider_id: vllm
    provider_type: remote::vllm
    config: {}
`
	spec := &ogxiov1beta1.OGXServerSpec{
		DisabledAPIs: []string{"vector_io"},
	}

	g1, err := GenerateConfig(spec, []byte(baseConfig))
	if err != nil {
		t.Fatal(err)
	}
	g2, err := GenerateConfig(spec, []byte(baseConfig))
	if err != nil {
		t.Fatal(err)
	}

	if g1.ContentHash != g2.ContentHash {
		t.Errorf("expected same hash for same input, got %q and %q", g1.ContentHash, g2.ContentHash)
	}
	if len(g1.ContentHash) != 16 {
		t.Errorf("expected 16-char content hash, got %q", g1.ContentHash)
	}
}

func TestDefaultConfigResolver_EmptyImage(t *testing.T) {
	resolver := NewDefaultConfigResolver(nil)
	_, err := resolver.Resolve("", "starter")
	if err == nil {
		t.Error("expected error for empty image reference")
	}
}

func TestDefaultConfigResolver_NoFetcher(t *testing.T) {
	resolver := NewDefaultConfigResolver(nil)
	_, err := resolver.Resolve("starter:latest", "starter")
	if err == nil {
		t.Error("expected error when OCI fetcher is not configured")
	}
}

func TestDefaultConfigResolver_NoImageNoDistribution(t *testing.T) {
	resolver := NewDefaultConfigResolver(nil)
	_, err := resolver.Resolve("", "")
	if err == nil {
		t.Error("expected error when image is empty")
	}
}

func TestDefaultConfigResolver_OCILabelUsed(t *testing.T) {
	expectedConfig := "version: '2'\nimage_name: from-oci\n"
	fetcher := func(imageRef string) (map[string]string, error) {
		return map[string]string{
			OCIDefaultConfigLabel:                "config.yaml",
			OCIConfigLabelPrefix + "config.yaml": "dmVyc2lvbjogJzInCmltYWdlX25hbWU6IGZyb20tb2NpCg==", // base64 of expectedConfig
		}, nil
	}

	resolver := NewDefaultConfigResolver(fetcher)
	data, err := resolver.Resolve("my-image:latest", "starter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != expectedConfig {
		t.Errorf("expected OCI config %q, got %q", expectedConfig, string(data))
	}
}

func TestDefaultConfigResolver_OCIFetchFails(t *testing.T) {
	fetcher := func(imageRef string) (map[string]string, error) {
		return nil, errors.New("failed to connect: network error")
	}

	resolver := NewDefaultConfigResolver(fetcher)
	_, err := resolver.Resolve("unreachable-image:v1", "starter")
	if err != nil {
		if !strings.Contains(err.Error(), "failed to connect") {
			t.Fatalf("expected OCI fetch failure, got: %v", err)
		}
		return
	}
	t.Fatal("expected error when OCI fetch fails")
}

func TestDefaultConfigResolver_OCILabelMissing(t *testing.T) {
	fetcher := func(imageRef string) (map[string]string, error) {
		return map[string]string{"unrelated-label": "value"}, nil
	}

	resolver := NewDefaultConfigResolver(fetcher)
	_, err := resolver.Resolve("my-image:latest", "starter")
	if err == nil {
		t.Fatal("expected error when OCI labels are missing")
	}
	if !strings.Contains(err.Error(), OCIDefaultConfigLabel) {
		t.Fatalf("expected missing default-config label error, got %v", err)
	}
}

func TestMergeStorage(t *testing.T) {
	base := map[string]interface{}{
		"backends": map[string]interface{}{
			"kv_default": map[string]interface{}{"type": "kv_sqlite"},
		},
	}

	user := map[string]interface{}{
		"backends": map[string]interface{}{
			"kv_default": map[string]interface{}{"type": "kv_redis", "host": "redis:6379"},
		},
	}

	result := MergeStorage(base, user)
	backends, ok := result["backends"].(map[string]interface{})
	if !ok {
		t.Fatal("expected backends to be a map")
	}
	kv, ok := backends["kv_default"].(map[string]interface{})
	if !ok {
		t.Fatal("expected kv_default to be a map")
	}
	if kv["type"] != "kv_redis" {
		t.Errorf("expected user storage to override, got %v", kv["type"])
	}
}

func TestValidateSecretRefEnvVarNames_ValidWithNormalization(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{
		Providers: &ogxiov1beta1.ProvidersSpec{
			Inference: &ogxiov1beta1.InferenceProvidersSpec{
				Remote: &ogxiov1beta1.InferenceRemoteProviders{
					Custom: []ogxiov1beta1.CustomProvider{
						{
							RoutedProviderBase: ogxiov1beta1.RoutedProviderBase{ID: "team.a/provider-1"},
							Type:               "remote::foo",
							SecretRefs: map[string]ogxiov1beta1.SecretKeyRef{
								"api.key": {Name: "s1", Key: "k1"},
							},
						},
					},
				},
			},
		},
	}

	if err := ValidateSecretRefEnvVarNames(spec); err != nil {
		t.Fatalf("expected valid env names after normalization, got: %v", err)
	}

	envVars := CollectSecretRefs(spec)
	if len(envVars) != 1 {
		t.Fatalf("expected one env var, got %d", len(envVars))
	}
	if strings.Contains(envVars[0].Name, ".") || strings.Contains(envVars[0].Name, "/") || strings.Contains(envVars[0].Name, "-") {
		t.Fatalf("expected normalized env var name, got %q", envVars[0].Name)
	}
}

func TestValidateSecretRefEnvVarNames_DetectsCollision(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{
		Providers: &ogxiov1beta1.ProvidersSpec{
			Inference: &ogxiov1beta1.InferenceProvidersSpec{
				Remote: &ogxiov1beta1.InferenceRemoteProviders{
					Custom: []ogxiov1beta1.CustomProvider{
						{
							RoutedProviderBase: ogxiov1beta1.RoutedProviderBase{ID: "provider-a"},
							Type:               "remote::foo",
							SecretRefs: map[string]ogxiov1beta1.SecretKeyRef{
								"api-key": {Name: "secret-a", Key: "token"},
							},
						},
						{
							RoutedProviderBase: ogxiov1beta1.RoutedProviderBase{ID: "provider_a"},
							Type:               "remote::foo",
							SecretRefs: map[string]ogxiov1beta1.SecretKeyRef{
								"api.key": {Name: "secret-b", Key: "token"},
							},
						},
					},
				},
			},
		},
	}

	if err := ValidateSecretRefEnvVarNames(spec); err == nil {
		t.Fatal("expected collision validation error, got nil")
	}
}

func TestExpandCustomProvider_SecretRefSettingsConflict(t *testing.T) {
	p := ogxiov1beta1.CustomProvider{
		RoutedProviderBase: ogxiov1beta1.RoutedProviderBase{ID: "my-provider"},
		Type:               "remote::custom",
		SecretRefs: map[string]ogxiov1beta1.SecretKeyRef{
			"api_key": {Name: "my-secret", Key: "key"},
		},
		Settings: &apiextensionsv1.JSON{
			Raw: []byte(`{"api_key":"plaintext-value"}`),
		},
	}

	_, err := expandCustomProvider(p)
	if err == nil {
		t.Fatal("expected error for secretRefs/settings key conflict, got nil")
	}
	if !strings.Contains(err.Error(), "api_key") {
		t.Errorf("expected error to mention conflicting key, got: %v", err)
	}
}

func TestExpandCustomProvider_NoConflict(t *testing.T) {
	p := ogxiov1beta1.CustomProvider{
		RoutedProviderBase: ogxiov1beta1.RoutedProviderBase{ID: "my-provider"},
		Type:               "remote::custom",
		SecretRefs: map[string]ogxiov1beta1.SecretKeyRef{
			"api_key": {Name: "my-secret", Key: "key"},
		},
		Settings: &apiextensionsv1.JSON{
			Raw: []byte(`{"endpoint":"https://example.com"}`),
		},
	}

	cp, err := expandCustomProvider(p)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cp.ProviderType != "remote::custom" {
		t.Errorf("expected provider type 'remote::custom', got %q", cp.ProviderType)
	}
	if cp.Config["endpoint"] != "https://example.com" {
		t.Errorf("expected endpoint in config, got %v", cp.Config["endpoint"])
	}
}
