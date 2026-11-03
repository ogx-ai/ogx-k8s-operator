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
	"fmt"

	"gopkg.in/yaml.v3"
)

// ParseBaseConfig parses the raw YAML into a BaseConfig struct.
func ParseBaseConfig(data []byte) (*BaseConfig, error) {
	var cfg BaseConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("failed to parse base config: %w", err)
	}
	if cfg.Version == "" {
		return nil, errors.New("failed to validate base config: missing required 'version' field")
	}
	return &cfg, nil
}

// MergeProviders merges user-configured providers over the base config.
// Strategy: replace entire API type block when user specifies it.
func MergeProviders(base map[string][]ConfigProvider, user map[string][]ConfigProvider) map[string][]ConfigProvider {
	if user == nil {
		return base
	}
	if base == nil {
		return user
	}

	merged := make(map[string][]ConfigProvider, len(base))
	for k, v := range base {
		merged[k] = v
	}
	// User providers replace base providers by API type
	for k, v := range user {
		merged[k] = v
	}
	return merged
}

// MergeStorage overlays explicit CR storage onto a clone of the distribution base.
// Nil user storage leaves the base content unchanged. Explicit backends replace the
// complete corresponding backend object; omitted KV/SQL inherit from the base, with
// SQLite used only when both omit the backend. Extra base backends, store mappings,
// and other storage fields are preserved. Default store mappings are filled only
// when absent; explicit null entries are left in place.
func MergeStorage(base map[string]interface{}, user map[string]interface{}) map[string]interface{} {
	if user == nil {
		return cloneStorageMap(base)
	}
	return overlayStorage(base, user)
}

// MergeAPIs filters the API list based on disabled APIs.
func MergeAPIs(baseAPIs []string, disabledAPIs []string) []string {
	if len(disabledAPIs) == 0 {
		return baseAPIs
	}
	disabled := make(map[string]bool, len(disabledAPIs))
	for _, d := range disabledAPIs {
		disabled[d] = true
	}
	var apis []string
	for _, api := range baseAPIs {
		if !disabled[api] {
			apis = append(apis, api)
		}
	}
	return apis
}
