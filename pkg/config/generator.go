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
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"gopkg.in/yaml.v3"
)

// GenerateConfig orchestrates the config generation pipeline.
// It takes the OGXServer spec and resolved base config, producing a
// complete config.yaml with all provider/resource/storage expansions applied.
func GenerateConfig(spec *ogxiov1beta1.OGXServerSpec, baseConfigData []byte) (*GeneratedConfig, error) {
	// Parse base config
	baseConfig, err := ParseBaseConfig(baseConfigData)
	if err != nil {
		return nil, fmt.Errorf("failed to parse base config: %w", err)
	}

	// Expand user providers from typed spec
	userProviders, err := ExpandProviders(spec.Providers)
	if err != nil {
		return nil, fmt.Errorf("failed to expand providers: %w", err)
	}

	// Merge providers: user replaces base by API type
	mergedProviders := MergeProviders(baseConfig.Providers, userProviders)

	// Expand resources (models)
	userModels, err := ExpandResources(spec.Resources, mergedProviders)
	if err != nil {
		return nil, fmt.Errorf("failed to expand resources: %w", err)
	}

	// Apply storage configuration
	userStorage := ApplyStorage(spec.Storage)
	mergedStorage := MergeStorage(baseConfig.Storage, userStorage)

	// Determine APIs (filter disabled)
	mergedAPIs := MergeAPIs(baseConfig.APIs, spec.DisabledAPIs)

	// Build the final config.yaml structure
	finalConfig := buildFinalConfig(baseConfig, mergedProviders, userModels, mergedStorage, mergedAPIs, spec)

	// Serialize to YAML
	configYAML, err := yaml.Marshal(finalConfig)
	if err != nil {
		return nil, fmt.Errorf("failed to serialize config: %w", err)
	}

	// Compute content hash
	hash := sha256.Sum256(configYAML)
	contentHash := hex.EncodeToString(hash[:8])

	// Collect secret references for env vars
	envVars := CollectSecretRefs(spec)

	// Count providers and resources
	providerCount := countProviders(mergedProviders)
	resourceCount := len(userModels)

	// Parse config version — defaulted indicates a non-numeric version string
	configVersion, configVersionParsed := parseConfigVersion(baseConfig.Version)

	return &GeneratedConfig{
		ConfigYAML:             string(configYAML),
		ContentHash:            contentHash,
		EnvVars:                envVars,
		ProviderCount:          providerCount,
		ResourceCount:          resourceCount,
		ConfigVersion:          configVersion,
		ConfigVersionDefaulted: !configVersionParsed,
	}, nil
}

func buildFinalConfig(
	base *BaseConfig,
	providers map[string][]ConfigProvider,
	models []ConfigModel,
	storage map[string]interface{},
	apis []string,
	spec *ogxiov1beta1.OGXServerSpec,
) map[string]interface{} {
	cfg := make(map[string]interface{})

	cfg["version"] = base.Version
	switch {
	case spec.Distribution.Name != "":
		cfg["distro_name"] = spec.Distribution.Name
	case spec.Distribution.Image != "":
		cfg["distro_name"] = spec.Distribution.Image
	}
	if len(apis) > 0 {
		cfg["apis"] = apis
	}
	if len(providers) > 0 {
		cfg["providers"] = serializeProviders(providers)
	}
	cfg["registered_resources"] = buildRegisteredResources(base, models)
	if base.VectorStores != nil {
		cfg["vector_stores"] = base.VectorStores
	}
	cfg["server"] = buildServerSection(base, spec)
	buildStorageSection(cfg, storage, base)

	return cfg
}

func serializeProviders(providers map[string][]ConfigProvider) map[string]interface{} {
	section := make(map[string]interface{}, len(providers))
	for apiType, ps := range providers {
		list := make([]interface{}, 0, len(ps))
		for _, p := range ps {
			entry := map[string]interface{}{
				"provider_id":   p.ProviderID,
				"provider_type": p.ProviderType,
			}
			if p.Config != nil {
				entry["config"] = p.Config
			} else {
				entry["config"] = map[string]interface{}{}
			}
			list = append(list, entry)
		}
		section[apiType] = list
	}
	return section
}

func serializeModels(models []ConfigModel) []interface{} {
	list := make([]interface{}, 0, len(models))
	for _, m := range models {
		entry := map[string]interface{}{
			"model_id":    m.ModelID,
			"provider_id": m.ProviderID,
		}
		if m.ModelType != "" {
			entry["model_type"] = m.ModelType
		}
		if m.ContextLength != nil {
			entry["context_length"] = *m.ContextLength
		}
		if m.Quantization != "" {
			entry["quantization"] = m.Quantization
		}
		list = append(list, entry)
	}
	return list
}

func buildRegisteredResources(base *BaseConfig, userModels []ConfigModel) *RegisteredResources {
	rr := &RegisteredResources{}
	if base.RegisteredResources != nil {
		rr.Models = base.RegisteredResources.Models
		rr.VectorStores = base.RegisteredResources.VectorStores
	}
	if len(userModels) > 0 {
		rr.Models = serializeModels(userModels)
	}
	return rr
}

func buildServerSection(base *BaseConfig, spec *ogxiov1beta1.OGXServerSpec) map[string]interface{} {
	server := make(map[string]interface{})
	for k, v := range base.Server {
		server[k] = v
	}
	if spec.Network != nil && spec.Network.Port != 0 {
		server["port"] = spec.Network.Port
	} else if _, hasPort := server["port"]; !hasPort {
		server["port"] = ogxiov1beta1.DefaultServerPort
	}
	return server
}

func buildStorageSection(cfg map[string]interface{}, storage map[string]interface{}, base *BaseConfig) {
	if storage != nil {
		cfg["storage"] = storage
	} else if base.Storage != nil {
		cfg["storage"] = base.Storage
	}
}

func countProviders(providers map[string][]ConfigProvider) int {
	count := 0
	for _, ps := range providers {
		count += len(ps)
	}
	return count
}

// parseConfigVersion attempts to parse the version string as an integer.
// Returns the parsed version and true on success, or (2, false) when the
// version is non-numeric so callers can log a warning about the default.
func parseConfigVersion(version string) (int, bool) {
	v, err := strconv.Atoi(version)
	if err != nil {
		return 2, false
	}
	return v, true
}
