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

import corev1 "k8s.io/api/core/v1"

// GeneratedConfig is the output of the config generation pipeline.
type GeneratedConfig struct {
	// ConfigYAML is the final config.yaml content.
	ConfigYAML string
	// ContentHash is a short SHA-256 prefix of ConfigYAML (8 bytes / 16 hex characters).
	ContentHash string
	// EnvVars are environment variable definitions to set on the Deployment container.
	EnvVars []corev1.EnvVar
	// ProviderCount is the number of configured providers.
	ProviderCount int
	// ResourceCount is the number of registered model resources.
	ResourceCount int
	// ConfigVersion is the detected config.yaml schema version.
	ConfigVersion int
	// ConfigVersionDefaulted is true when the version string was non-numeric
	// and ConfigVersion was defaulted to 2. Callers should log a warning.
	ConfigVersionDefaulted bool
}

// ConfigProvider represents a provider in config.yaml format.
type ConfigProvider struct {
	ProviderID   string                 `yaml:"provider_id"`
	ProviderType string                 `yaml:"provider_type"`
	Config       map[string]interface{} `yaml:"config"`
}

// ConfigModel represents a model in config.yaml format.
type ConfigModel struct {
	ModelID       string `yaml:"model_id"`
	ProviderID    string `yaml:"provider_id"`
	ModelType     string `yaml:"model_type,omitempty"`
	ContextLength *int   `yaml:"context_length,omitempty"`
	Quantization  string `yaml:"quantization,omitempty"`
}

// RegisteredResources represents the registered_resources section of config.yaml.
type RegisteredResources struct {
	Models       []interface{} `yaml:"models,omitempty"`
	VectorStores []interface{} `yaml:"vector_stores,omitempty"`
}

// BaseConfig represents the parsed base config.yaml structure.
type BaseConfig struct {
	Version             string                      `yaml:"version"`
	DistroName          string                      `yaml:"distro_name,omitempty"`
	ImageName           string                      `yaml:"image_name,omitempty"`
	APIs                []string                    `yaml:"apis,omitempty"`
	Providers           map[string][]ConfigProvider `yaml:"providers,omitempty"`
	RegisteredResources *RegisteredResources        `yaml:"registered_resources,omitempty"`
	VectorStores        map[string]interface{}      `yaml:"vector_stores,omitempty"`
	Server              map[string]interface{}      `yaml:"server,omitempty"`
	Storage             map[string]interface{}      `yaml:"storage,omitempty"`
}
