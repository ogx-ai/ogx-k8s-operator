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

const llama3ModelID = "llama3"

func requireProviderAt(t *testing.T, result map[string][]ConfigProvider, api string, idx int) ConfigProvider {
	t.Helper()

	providers := result[api]
	if len(providers) <= idx {
		t.Fatalf("expected %s provider at index %d, got %d providers", api, idx, len(providers))
	}
	return providers[idx]
}

func requireConfigMap(t *testing.T, cfg map[string]interface{}, key string) map[string]interface{} {
	t.Helper()

	value, ok := cfg[key].(map[string]interface{})
	if !ok {
		t.Fatalf("expected %s map, got %v", key, cfg[key])
	}
	return value
}

func requireStringMap(t *testing.T, cfg map[string]interface{}, key string) map[string]string {
	t.Helper()

	value, ok := cfg[key].(map[string]string)
	if !ok {
		t.Fatalf("expected %s map, got %v", key, cfg[key])
	}
	return value
}

func assertProviderIdentity(t *testing.T, provider ConfigProvider, wantID, wantType string) {
	t.Helper()

	if provider.ProviderID != wantID {
		t.Errorf("expected provider_id %q, got %q", wantID, provider.ProviderID)
	}
	if wantType != "" && provider.ProviderType != wantType {
		t.Errorf("expected provider_type %q, got %q", wantType, provider.ProviderType)
	}
}

func assertConfigValue(t *testing.T, cfg map[string]interface{}, key string, want interface{}) {
	t.Helper()

	got, ok := cfg[key]
	if !ok {
		t.Errorf("expected %s, but key was missing", key)
		return
	}
	if got != want {
		t.Errorf("expected %s %v, got %v", key, want, got)
	}
}

func assertConfigValues(t *testing.T, cfg map[string]interface{}, want map[string]interface{}) {
	t.Helper()

	for key, value := range want {
		assertConfigValue(t, cfg, key, value)
	}
}

func assertConfigAbsent(t *testing.T, cfg map[string]interface{}, key string) {
	t.Helper()

	if _, ok := cfg[key]; ok {
		t.Errorf("%s should not appear in config", key)
	}
}

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
				if !ok || firstModel["model_id"] != llama3ModelID {
					t.Errorf("expected registered resource model %q, got %+v", llama3ModelID, cfg.RegisteredResources.Models[0])
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
				Name:          llama3ModelID,
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
	if models[0].ModelID != llama3ModelID {
		t.Errorf("expected model_id %q, got %q", llama3ModelID, models[0].ModelID)
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

func TestExpandResources_DefaultProviderIsDeterministic(t *testing.T) {
	providers := map[string][]ConfigProvider{
		"inference": {
			{ProviderID: "openai-1", ProviderType: "remote::openai"},
			{ProviderID: "azure-1", ProviderType: "remote::azure"},
			{ProviderID: "vllm-1", ProviderType: "remote::vllm"},
		},
	}
	resources := &ogxiov1beta1.ResourcesSpec{
		Models: []ogxiov1beta1.ModelConfig{{Name: llama3ModelID}},
	}

	models, err := ExpandResources(resources, providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models[0].ProviderID != "azure-1" {
		t.Errorf("expected lexicographically first provider 'azure-1', got %q", models[0].ProviderID)
	}

	// Reverse the input order — result should be the same
	providers["inference"] = []ConfigProvider{
		{ProviderID: "vllm-1", ProviderType: "remote::vllm"},
		{ProviderID: "openai-1", ProviderType: "remote::openai"},
		{ProviderID: "azure-1", ProviderType: "remote::azure"},
	}
	models2, err := ExpandResources(resources, providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if models2[0].ProviderID != "azure-1" {
		t.Errorf("expected same provider regardless of input order, got %q", models2[0].ProviderID)
	}
}

func TestExpandResources_NoProviderAndNoInference(t *testing.T) {
	resources := &ogxiov1beta1.ResourcesSpec{
		Models: []ogxiov1beta1.ModelConfig{
			{Name: llama3ModelID},
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
	if p.Config["base_url"] != "https://vllm.example.com:8000" {
		t.Errorf("expected base_url, got %v", p.Config["base_url"])
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

func TestExpandProviders_FileProcessorsAllProviders(t *testing.T) {
	chunkSize := 1000
	chunkOverlap := 500
	extractMeta := true
	cleanText := false
	doOCR := false
	doclingChunk := 2048

	providers := &ogxiov1beta1.ProvidersSpec{
		FileProcessors: &ogxiov1beta1.FileProcessorsProvidersSpec{
			Remote: &ogxiov1beta1.FileProcessorsRemoteProviders{
				DoclingServe: &ogxiov1beta1.DoclingServeProvider{
					BaseURL:                "http://docling.example.com:5001/v1",
					APIKey:                 &ogxiov1beta1.SecretKeyRef{Name: "docling-secret", Key: "api-key"},
					DefaultChunkSizeTokens: &doclingChunk,
				},
			},
			Inline: &ogxiov1beta1.FileProcessorsInlineProviders{
				Auto: &ogxiov1beta1.InlineAutoFileProcessorProvider{
					FileProcessorChunkConfig: ogxiov1beta1.FileProcessorChunkConfig{
						DefaultChunkSizeTokens:    &chunkSize,
						DefaultChunkOverlapTokens: &chunkOverlap,
					},
					ExtractMetadata: &extractMeta,
					CleanText:       &cleanText,
				},
				MarkItDown: &ogxiov1beta1.InlineMarkItDownFileProcessorProvider{},
				Docling: &ogxiov1beta1.InlineDoclingFileProcessorProvider{
					DoOCR: &doOCR,
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	fps := result["file_processors"]
	if len(fps) != 4 {
		t.Fatalf("expected 4 file_processors providers, got %d", len(fps))
	}

	assertProviderIdentity(t, fps[0], "remote-docling-serve", "remote::docling-serve")
	assertConfigValues(t, fps[0].Config, map[string]interface{}{
		"base_url":                  "http://docling.example.com:5001/v1",
		"default_chunk_size_tokens": 2048,
	})

	assertProviderIdentity(t, fps[1], "inline-auto", "inline::auto")
	assertConfigValues(t, fps[1].Config, map[string]interface{}{
		"default_chunk_size_tokens": 1000,
		"extract_metadata":          true,
		"clean_text":                false,
	})

	assertProviderIdentity(t, fps[2], "inline-markitdown", "")
	if len(fps[2].Config) != 0 {
		t.Errorf("expected empty config for markitdown, got %v", fps[2].Config)
	}

	assertProviderIdentity(t, fps[3], "inline-docling", "")
	assertConfigValue(t, fps[3].Config, "do_ocr", false)
}

func TestCollectSecretRefs_FileProcessorsDoclingServe(t *testing.T) {
	spec := &ogxiov1beta1.OGXServerSpec{
		Providers: &ogxiov1beta1.ProvidersSpec{
			FileProcessors: &ogxiov1beta1.FileProcessorsProvidersSpec{
				Remote: &ogxiov1beta1.FileProcessorsRemoteProviders{
					DoclingServe: &ogxiov1beta1.DoclingServeProvider{
						BaseURL: "http://docling.example.com:5001/v1",
						APIKey:  &ogxiov1beta1.SecretKeyRef{Name: "docling-secret", Key: "api-key"},
					},
				},
			},
		},
	}

	envVars := CollectSecretRefs(spec)
	if len(envVars) != 1 {
		t.Fatalf("expected 1 env var, got %d", len(envVars))
	}
	if envVars[0].Name != "OGX_REMOTE_DOCLING_SERVE_API_KEY" {
		t.Errorf("expected env var name 'OGX_REMOTE_DOCLING_SERVE_API_KEY', got %q", envVars[0].Name)
	}
	if envVars[0].ValueFrom.SecretKeyRef.Name != "docling-secret" {
		t.Errorf("expected secret name 'docling-secret', got %q", envVars[0].ValueFrom.SecretKeyRef.Name)
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

func TestExpandProviders_VLLMWithCommonInferenceConfig(t *testing.T) {
	refreshModels := true
	providers := &ogxiov1beta1.ProvidersSpec{
		Inference: &ogxiov1beta1.InferenceProvidersSpec{
			Remote: &ogxiov1beta1.InferenceRemoteProviders{
				VLLM: []ogxiov1beta1.VLLMProvider{
					{
						Endpoint: "https://vllm.example.com:8000",
						RemoteInferenceCommonConfig: ogxiov1beta1.RemoteInferenceCommonConfig{
							RefreshModels: &refreshModels,
							AllowedModels: []string{llama3ModelID, "mistral"},
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

	p := result["inference"][0]
	if p.Config["base_url"] != "https://vllm.example.com:8000" {
		t.Errorf("expected base_url, got %v", p.Config["base_url"])
	}
	if p.Config["refresh_models"] != true {
		t.Errorf("expected refresh_models true, got %v", p.Config["refresh_models"])
	}
	allowedModels, ok := p.Config["allowed_models"].([]string)
	if !ok || len(allowedModels) != 2 || allowedModels[0] != llama3ModelID {
		t.Errorf("expected allowed_models [%s mistral], got %v", llama3ModelID, p.Config["allowed_models"])
	}
}

func TestExpandProviders_AzureBaseURL(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		Inference: &ogxiov1beta1.InferenceProvidersSpec{
			Remote: &ogxiov1beta1.InferenceRemoteProviders{
				Azure: []ogxiov1beta1.AzureProvider{
					{
						Endpoint: "https://my-resource.openai.azure.com/openai/v1",
						APIKey:   ogxiov1beta1.SecretKeyRef{Name: "azure-secret", Key: "key"},
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := result["inference"][0]
	if p.Config["base_url"] != "https://my-resource.openai.azure.com/openai/v1" {
		t.Errorf("expected base_url, got %v", p.Config["base_url"])
	}
	if _, hasOldKey := p.Config["url"]; hasOldKey {
		t.Error("config should not contain legacy 'url' key")
	}
}

func TestExpandProviders_BedrockRegionName(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		Inference: &ogxiov1beta1.InferenceProvidersSpec{
			Remote: &ogxiov1beta1.InferenceRemoteProviders{
				Bedrock: []ogxiov1beta1.BedrockProvider{
					{
						Region: "us-east-1",
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := result["inference"][0]
	if p.Config["region_name"] != "us-east-1" {
		t.Errorf("expected region_name 'us-east-1', got %v", p.Config["region_name"])
	}
	if _, hasOldKey := p.Config["region"]; hasOldKey {
		t.Error("config should not contain legacy 'region' key")
	}
}

func TestExpandProviders_WatsonxBaseURL(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		Inference: &ogxiov1beta1.InferenceProvidersSpec{
			Remote: &ogxiov1beta1.InferenceRemoteProviders{
				Watsonx: []ogxiov1beta1.WatsonxProvider{
					{
						Endpoint: "https://watsonx.example.com",
						APIKey:   ogxiov1beta1.SecretKeyRef{Name: "wx-secret", Key: "key"},
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := result["inference"][0]
	if p.Config["base_url"] != "https://watsonx.example.com" {
		t.Errorf("expected base_url, got %v", p.Config["base_url"])
	}
	if _, hasOldKey := p.Config["url"]; hasOldKey {
		t.Error("config should not contain legacy 'url' key")
	}
}

func TestExpandProviders_NetworkConfigNested(t *testing.T) {
	verify := false
	connectTimeout := 30
	providers := &ogxiov1beta1.ProvidersSpec{
		Inference: &ogxiov1beta1.InferenceProvidersSpec{
			Remote: &ogxiov1beta1.InferenceRemoteProviders{
				VLLM: []ogxiov1beta1.VLLMProvider{
					{
						Endpoint: "https://vllm.example.com:8000",
						RemoteInferenceCommonConfig: ogxiov1beta1.RemoteInferenceCommonConfig{
							Network: &ogxiov1beta1.NetworkConfig{
								TLS: &ogxiov1beta1.TLSConfig{
									Verify: &verify,
								},
								Timeout: &ogxiov1beta1.TimeoutConfig{
									Connect: &connectTimeout,
								},
								Headers: map[string]string{
									"X-Custom": "value",
								},
							},
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

	p := requireProviderAt(t, result, "inference", 0)
	network := requireConfigMap(t, p.Config, "network")
	assertConfigValue(t, requireConfigMap(t, network, "tls"), "verify", false)
	assertConfigValue(t, requireConfigMap(t, network, "timeout"), "connect", 30)

	headers := requireStringMap(t, network, "headers")
	if headers["X-Custom"] != "value" {
		t.Errorf("expected X-Custom header, got %v", headers["X-Custom"])
	}

	assertConfigAbsent(t, p.Config, "tls")
	assertConfigAbsent(t, p.Config, "timeout")
	assertConfigAbsent(t, p.Config, "headers")
}

func TestExpandProviders_NetworkConfigWithProxy(t *testing.T) {
	proxyURL := "http://proxy.example.com:8080"
	proxyHTTPS := "http://proxy.example.com:8443"
	caCert := "/etc/ssl/proxy-ca.crt"
	providers := &ogxiov1beta1.ProvidersSpec{
		Inference: &ogxiov1beta1.InferenceProvidersSpec{
			Remote: &ogxiov1beta1.InferenceRemoteProviders{
				VLLM: []ogxiov1beta1.VLLMProvider{
					{
						Endpoint: "https://vllm.example.com:8000",
						RemoteInferenceCommonConfig: ogxiov1beta1.RemoteInferenceCommonConfig{
							Network: &ogxiov1beta1.NetworkConfig{
								Proxy: &ogxiov1beta1.ProxyConfig{
									URL:     &proxyURL,
									HTTPS:   &proxyHTTPS,
									CACert:  &caCert,
									NoProxy: []string{"localhost", "10.0.0.0/8"},
								},
							},
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

	p := result["inference"][0]
	network, ok := p.Config["network"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected nested 'network' map, got %v", p.Config["network"])
	}
	proxy, ok := network["proxy"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected proxy map inside network, got %v", network["proxy"])
	}
	if proxy["url"] != proxyURL {
		t.Errorf("expected proxy url %q, got %v", proxyURL, proxy["url"])
	}
	if proxy["https"] != proxyHTTPS {
		t.Errorf("expected proxy https %q, got %v", proxyHTTPS, proxy["https"])
	}
	if proxy["ca_cert"] != caCert {
		t.Errorf("expected proxy ca_cert %q, got %v", caCert, proxy["ca_cert"])
	}
	noProxy, ok := proxy["no_proxy"].([]string)
	if !ok || len(noProxy) != 2 {
		t.Errorf("expected 2 no_proxy entries, got %v", proxy["no_proxy"])
	}
}

func TestExpandProviders_NetworkConfigNilIsOmitted(t *testing.T) {
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

	p := result["inference"][0]
	if _, hasNetwork := p.Config["network"]; hasNetwork {
		t.Error("expected no 'network' key when network config is nil")
	}
}

func TestExpandProviders_QdrantAllFields(t *testing.T) {
	port := 6333
	grpcPort := 6334
	preferGRPC := true
	https := true
	timeout := 30
	providers := &ogxiov1beta1.ProvidersSpec{
		VectorIo: &ogxiov1beta1.VectorIOProvidersSpec{
			Remote: &ogxiov1beta1.VectorIORemoteProviders{
				Qdrant: []ogxiov1beta1.QdrantProvider{
					{
						URL:        "http://qdrant:6333",
						Host:       "qdrant.example.com",
						Port:       &port,
						APIKey:     &ogxiov1beta1.SecretKeyRef{Name: "qdrant-secret", Key: "api-key"},
						Location:   "us-east-1",
						GRPCPort:   &grpcPort,
						PreferGRPC: &preferGRPC,
						HTTPS:      &https,
						Prefix:     "/v1",
						Timeout:    &timeout,
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := requireProviderAt(t, result, "vector_io", 0)
	assertProviderIdentity(t, p, "remote-qdrant", "remote::qdrant")
	assertConfigValues(t, p.Config, map[string]interface{}{
		"url":         "http://qdrant:6333",
		"host":        "qdrant.example.com",
		"port":        6333,
		"location":    "us-east-1",
		"grpc_port":   6334,
		"prefer_grpc": true,
		"https":       true,
		"prefix":      "/v1",
		"timeout":     30,
	})
}

func TestExpandProviders_QdrantMinimalWithHost(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		VectorIo: &ogxiov1beta1.VectorIOProvidersSpec{
			Remote: &ogxiov1beta1.VectorIORemoteProviders{
				Qdrant: []ogxiov1beta1.QdrantProvider{
					{
						Host: "qdrant.example.com",
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := result["vector_io"][0]
	if p.Config["host"] != "qdrant.example.com" {
		t.Errorf("expected host, got %v", p.Config["host"])
	}
	if _, hasURL := p.Config["url"]; hasURL {
		t.Error("url should not be set when only host is provided")
	}
}

func TestExpandProviders_PgvectorWithVectorIndex(t *testing.T) {
	m := 16
	efConstruction := 200
	efSearch := 100
	providers := &ogxiov1beta1.ProvidersSpec{
		VectorIo: &ogxiov1beta1.VectorIOProvidersSpec{
			Remote: &ogxiov1beta1.VectorIORemoteProviders{
				Pgvector: []ogxiov1beta1.PgvectorProvider{
					{
						Host:           "pg.example.com",
						Password:       ogxiov1beta1.SecretKeyRef{Name: "pg-secret", Key: "password"},
						DistanceMetric: "COSINE",
						VectorIndex: &ogxiov1beta1.VectorIndexConfig{
							HNSW: &ogxiov1beta1.HNSWConfig{
								M:              &m,
								EfConstruction: &efConstruction,
								EfSearch:       &efSearch,
							},
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

	p := result["vector_io"][0]
	if p.ProviderType != "remote::pgvector" {
		t.Errorf("expected provider_type 'remote::pgvector', got %q", p.ProviderType)
	}
	if p.Config["distance_metric"] != "COSINE" {
		t.Errorf("expected distance_metric 'COSINE', got %v", p.Config["distance_metric"])
	}

	vi, ok := p.Config["vector_index"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected vector_index map, got %v", p.Config["vector_index"])
	}
	hnsw, ok := vi["hnsw"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected hnsw map, got %v", vi["hnsw"])
	}
	if hnsw["m"] != 16 {
		t.Errorf("expected hnsw m=16, got %v", hnsw["m"])
	}
	if hnsw["ef_construction"] != 200 {
		t.Errorf("expected hnsw ef_construction=200, got %v", hnsw["ef_construction"])
	}
	if hnsw["ef_search"] != 100 {
		t.Errorf("expected hnsw ef_search=100, got %v", hnsw["ef_search"])
	}
}

func TestExpandProviders_PgvectorWithIVFFlat(t *testing.T) {
	nlist := 128
	nprobe := 10
	providers := &ogxiov1beta1.ProvidersSpec{
		VectorIo: &ogxiov1beta1.VectorIOProvidersSpec{
			Remote: &ogxiov1beta1.VectorIORemoteProviders{
				Pgvector: []ogxiov1beta1.PgvectorProvider{
					{
						Password: ogxiov1beta1.SecretKeyRef{Name: "pg-secret", Key: "password"},
						VectorIndex: &ogxiov1beta1.VectorIndexConfig{
							IVFFlat: &ogxiov1beta1.IVFFlatConfig{
								Nlist:  &nlist,
								Nprobe: &nprobe,
							},
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

	vi, ok := result["vector_io"][0].Config["vector_index"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected vector_index map, got %v", result["vector_io"][0].Config["vector_index"])
	}
	ivf, ok := vi["ivf_flat"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected ivf_flat map, got %v", vi["ivf_flat"])
	}
	if ivf["nlist"] != 128 {
		t.Errorf("expected nlist=128, got %v", ivf["nlist"])
	}
	if ivf["nprobe"] != 10 {
		t.Errorf("expected nprobe=10, got %v", ivf["nprobe"])
	}
}

func TestExpandProviders_MilvusWithConsistencyLevel(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		VectorIo: &ogxiov1beta1.VectorIOProvidersSpec{
			Remote: &ogxiov1beta1.VectorIORemoteProviders{
				Milvus: []ogxiov1beta1.MilvusProvider{
					{
						URI:              "http://milvus:19530",
						ConsistencyLevel: "Strong",
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := result["vector_io"][0]
	if p.ProviderType != "remote::milvus" {
		t.Errorf("expected provider_type 'remote::milvus', got %q", p.ProviderType)
	}
	if p.Config["consistency_level"] != "Strong" {
		t.Errorf("expected consistency_level 'Strong', got %v", p.Config["consistency_level"])
	}
}

func TestExpandProviders_BatchReference(t *testing.T) {
	maxBatches := 5
	maxRequestsPerBatch := 100
	providers := &ogxiov1beta1.ProvidersSpec{
		Batches: &ogxiov1beta1.BatchesProvidersSpec{
			Inline: &ogxiov1beta1.BatchesInlineProviders{
				Reference: &ogxiov1beta1.InlineReferenceProvider{
					MaxConcurrentBatches:          &maxBatches,
					MaxConcurrentRequestsPerBatch: &maxRequestsPerBatch,
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := result["batches"][0]
	if p.ProviderType != "inline::batch-reference" {
		t.Errorf("expected provider_type 'inline::batch-reference', got %q", p.ProviderType)
	}
	if p.Config["max_concurrent_batches"] != 5 {
		t.Errorf("expected max_concurrent_batches=5, got %v", p.Config["max_concurrent_batches"])
	}
	if p.Config["max_concurrent_requests_per_batch"] != 100 {
		t.Errorf("expected max_concurrent_requests_per_batch=100, got %v", p.Config["max_concurrent_requests_per_batch"])
	}
}

func TestExpandProviders_BatchReferenceEmpty(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		Batches: &ogxiov1beta1.BatchesProvidersSpec{
			Inline: &ogxiov1beta1.BatchesInlineProviders{
				Reference: &ogxiov1beta1.InlineReferenceProvider{},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := result["batches"][0]
	if len(p.Config) != 0 {
		t.Errorf("expected empty config for default reference provider, got %v", p.Config)
	}
}

func TestExpandProviders_InlineFileSearchWithVectorStoresConfig(t *testing.T) {
	embeddingDim := 768
	providers := &ogxiov1beta1.ProvidersSpec{
		ToolRuntime: &ogxiov1beta1.ToolRuntimeProvidersSpec{
			Inline: &ogxiov1beta1.ToolRuntimeInlineProviders{
				FileSearch: []ogxiov1beta1.InlineFileSearchProvider{
					{
						VectorStoresConfig: &ogxiov1beta1.VectorStoresConfig{
							DefaultProviderID: "pgvector-1",
							DefaultEmbeddingModel: &ogxiov1beta1.QualifiedModel{
								ProviderID:          "embed-provider",
								ModelID:             "all-MiniLM-L6-v2",
								EmbeddingDimensions: &embeddingDim,
							},
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

	p := result["tool_runtime"][0]
	if p.ProviderType != "inline::rag-runtime" {
		t.Errorf("expected provider_type 'inline::rag-runtime', got %q", p.ProviderType)
	}
	vs, ok := p.Config["vector_stores_config"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected vector_stores_config map, got %v", p.Config["vector_stores_config"])
	}
	if vs["default_provider_id"] != "pgvector-1" {
		t.Errorf("expected default_provider_id, got %v", vs["default_provider_id"])
	}
	emb, ok := vs["default_embedding_model"].(map[string]interface{})
	if !ok {
		t.Fatalf("expected default_embedding_model map, got %v", vs["default_embedding_model"])
	}
	if emb["provider_id"] != "embed-provider" {
		t.Errorf("expected provider_id 'embed-provider', got %v", emb["provider_id"])
	}
	if emb["model_id"] != "all-MiniLM-L6-v2" {
		t.Errorf("expected model_id, got %v", emb["model_id"])
	}
	if emb["embedding_dimensions"] != 768 {
		t.Errorf("expected embedding_dimensions 768, got %v", emb["embedding_dimensions"])
	}
}

func TestExpandProviders_BuiltinResponsesWithVectorStoresAndCompaction(t *testing.T) {
	compactThreshold := 4096
	chunkSize := 512
	chunkOverlap := 50
	maxTokens := 256
	temperature := "0.7"
	enableAnnotations := true
	chunkMultiplier := 3
	maxTokensInContext := 2048
	rerankerStrategy := "rrf"
	rrfFactor := "60.0"
	searchAlpha := "0.5"
	searchMode := "hybrid"
	maxFilesPerBatch := 10
	batchChunkSize := 5
	cleanupInterval := 3600
	contextTimeout := 30
	contextConcurrency := 4
	maxDocTokens := 8000

	providers := &ogxiov1beta1.ProvidersSpec{
		Responses: &ogxiov1beta1.ResponsesProvidersSpec{
			Inline: &ogxiov1beta1.ResponsesInlineProviders{
				Builtin: &ogxiov1beta1.InlineBuiltinResponsesProvider{
					VectorStoresConfig: &ogxiov1beta1.VectorStoresConfig{
						DefaultProviderID: "qdrant-1",
						DefaultRerankerModel: &ogxiov1beta1.RerankerModel{
							ProviderID: "reranker-provider",
							ModelID:    "bge-reranker-v2",
						},
						RewriteQueryParams: &ogxiov1beta1.RewriteQueryParams{
							Model: &ogxiov1beta1.QualifiedModel{
								ProviderID: "llm-provider",
								ModelID:    llama3ModelID,
							},
							Prompt:      "Rewrite: {query}",
							MaxTokens:   &maxTokens,
							Temperature: &temperature,
						},
						FileSearchParams: &ogxiov1beta1.FileSearchDisplayParams{
							HeaderTemplate: "=== Results ===",
							FooterTemplate: "=== End ===",
						},
						ContextPromptParams: &ogxiov1beta1.ContextPromptParams{
							ChunkAnnotationTemplate: "Chunk: {{content}}",
							ContextTemplate:         "Context: {{chunks}}",
						},
						AnnotationPromptParams: &ogxiov1beta1.AnnotationPromptParams{
							EnableAnnotations:             &enableAnnotations,
							AnnotationInstructionTemplate: "Cite sources",
							ChunkAnnotationTemplate:       "Source: {{source}}",
						},
						FileIngestionParams: &ogxiov1beta1.FileIngestionParams{
							DefaultChunkSizeTokens:    &chunkSize,
							DefaultChunkOverlapTokens: &chunkOverlap,
						},
						ChunkRetrievalParams: &ogxiov1beta1.ChunkRetrievalParams{
							ChunkMultiplier:         &chunkMultiplier,
							MaxTokensInContext:      &maxTokensInContext,
							DefaultRerankerStrategy: &rerankerStrategy,
							RRFImpactFactor:         &rrfFactor,
							WeightedSearchAlpha:     &searchAlpha,
							DefaultSearchMode:       &searchMode,
						},
						FileBatchParams: &ogxiov1beta1.FileBatchParams{
							MaxConcurrentFilesPerBatch: &maxFilesPerBatch,
							FileBatchChunkSize:         &batchChunkSize,
							CleanupIntervalSeconds:     &cleanupInterval,
						},
						ContextualRetrievalParams: &ogxiov1beta1.ContextualRetrievalParams{
							Model: &ogxiov1beta1.QualifiedModel{
								ProviderID: "ctx-provider",
								ModelID:    "ctx-model",
							},
							DefaultTimeoutSeconds: &contextTimeout,
							DefaultMaxConcurrency: &contextConcurrency,
							MaxDocumentTokens:     &maxDocTokens,
						},
					},
					CompactionConfig: &ogxiov1beta1.CompactionConfig{
						SummarizationPrompt:     "Summarize this conversation",
						SummaryPrefix:           "Previous context: ",
						SummarizationModel:      "llama3-8b",
						DefaultCompactThreshold: &compactThreshold,
						TokenizerEncoding:       "cl100k_base",
					},
				},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := requireProviderAt(t, result, "responses", 0)
	assertProviderIdentity(t, p, "inline-builtin", "inline::responses")

	vs := requireConfigMap(t, p.Config, "vector_stores_config")
	assertConfigValue(t, vs, "default_provider_id", "qdrant-1")
	assertConfigValues(t, requireConfigMap(t, vs, "default_reranker_model"), map[string]interface{}{
		"provider_id": "reranker-provider",
		"model_id":    "bge-reranker-v2",
	})
	assertConfigValues(t, requireConfigMap(t, vs, "rewrite_query_params"), map[string]interface{}{
		"prompt":      "Rewrite: {query}",
		"max_tokens":  256,
		"temperature": "0.7",
	})
	assertConfigValues(t, requireConfigMap(t, vs, "file_search_params"), map[string]interface{}{
		"header_template": "=== Results ===",
		"footer_template": "=== End ===",
	})
	assertConfigValues(t, requireConfigMap(t, vs, "context_prompt_params"), map[string]interface{}{
		"chunk_annotation_template": "Chunk: {{content}}",
		"context_template":          "Context: {{chunks}}",
	})
	assertConfigValue(t, requireConfigMap(t, vs, "annotation_prompt_params"), "enable_annotations", true)
	assertConfigValues(t, requireConfigMap(t, vs, "file_ingestion_params"), map[string]interface{}{
		"default_chunk_size_tokens":    512,
		"default_chunk_overlap_tokens": 50,
	})
	assertConfigValues(t, requireConfigMap(t, vs, "chunk_retrieval_params"), map[string]interface{}{
		"chunk_multiplier":          3,
		"default_reranker_strategy": "rrf",
		"default_search_mode":       "hybrid",
	})
	assertConfigValue(t, requireConfigMap(t, vs, "file_batch_params"), "max_concurrent_files_per_batch", 10)
	assertConfigValues(t, requireConfigMap(t, vs, "contextual_retrieval_params"), map[string]interface{}{
		"default_timeout_seconds": 30,
		"max_document_tokens":     8000,
	})

	assertConfigValues(t, requireConfigMap(t, p.Config, "compaction_config"), map[string]interface{}{
		"summarization_prompt":      "Summarize this conversation",
		"summary_prefix":            "Previous context: ",
		"summarization_model":       "llama3-8b",
		"default_compact_threshold": 4096,
		"tokenizer_encoding":        "cl100k_base",
	})
}

func TestExpandProviders_BuiltinResponsesEmpty(t *testing.T) {
	providers := &ogxiov1beta1.ProvidersSpec{
		Responses: &ogxiov1beta1.ResponsesProvidersSpec{
			Inline: &ogxiov1beta1.ResponsesInlineProviders{
				Builtin: &ogxiov1beta1.InlineBuiltinResponsesProvider{},
			},
		},
	}

	result, err := ExpandProviders(providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	p := result["responses"][0]
	if len(p.Config) != 0 {
		t.Errorf("expected empty config for default builtin responses, got %v", p.Config)
	}
}

func TestExpandResources_WithQuantization(t *testing.T) {
	providers := map[string][]ConfigProvider{
		"inference": {
			{ProviderID: "vllm-1", ProviderType: "remote::vllm"},
		},
	}

	resources := &ogxiov1beta1.ResourcesSpec{
		Models: []ogxiov1beta1.ModelConfig{
			{
				Name:         "llama3-8b-int8",
				Provider:     "vllm-1",
				Quantization: "int8",
			},
		},
	}

	models, err := ExpandResources(resources, providers)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(models) != 1 {
		t.Fatalf("expected 1 model, got %d", len(models))
	}
	if models[0].Quantization != "int8" {
		t.Errorf("expected quantization 'int8', got %q", models[0].Quantization)
	}
}

func TestGenerateConfig_ModelQuantizationInYAML(t *testing.T) {
	baseConfig := `version: '2'
providers:
  inference:
  - provider_id: vllm
    provider_type: remote::vllm
    config: {}
`

	spec := &ogxiov1beta1.OGXServerSpec{
		Providers: &ogxiov1beta1.ProvidersSpec{
			Inference: &ogxiov1beta1.InferenceProvidersSpec{
				Remote: &ogxiov1beta1.InferenceRemoteProviders{
					VLLM: []ogxiov1beta1.VLLMProvider{
						{Endpoint: "https://vllm:8000"},
					},
				},
			},
		},
		Resources: &ogxiov1beta1.ResourcesSpec{
			Models: []ogxiov1beta1.ModelConfig{
				{
					Name:         "llama3-8b-int4",
					Quantization: "int4",
				},
			},
		},
	}

	generated, err := GenerateConfig(spec, []byte(baseConfig))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !strings.Contains(generated.ConfigYAML, "quantization: int4") {
		t.Errorf("expected quantization in generated YAML, got:\n%s", generated.ConfigYAML)
	}
}
