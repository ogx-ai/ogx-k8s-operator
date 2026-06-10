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
	"fmt"
	"sort"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
)

const defaultModelType = "llm"

// ExpandResources converts the ResourcesSpec into config.yaml model entries.
// It assigns providers based on the provider map (from ExpandProviders or base config).
func ExpandResources(resources *ogxiov1beta1.ResourcesSpec, providers map[string][]ConfigProvider) ([]ConfigModel, error) {
	if resources == nil {
		return nil, nil
	}

	var models []ConfigModel

	// Expand models
	for _, m := range resources.Models {
		providerID := m.Provider
		if providerID == "" {
			inferenceProviders := providers["inference"]
			if len(inferenceProviders) == 0 {
				return nil, fmt.Errorf("failed to assign provider for model %q: no explicit provider and no inference providers are configured", m.Name)
			}
			sorted := make([]ConfigProvider, len(inferenceProviders))
			copy(sorted, inferenceProviders)
			sort.Slice(sorted, func(i, j int) bool {
				return sorted[i].ProviderID < sorted[j].ProviderID
			})
			providerID = sorted[0].ProviderID
		}

		model := ConfigModel{
			ModelID:    m.Name,
			ProviderID: providerID,
		}
		if m.ModelType != "" {
			model.ModelType = m.ModelType
		} else {
			model.ModelType = defaultModelType
		}
		if m.ContextLength != nil {
			model.ContextLength = m.ContextLength
		}
		if m.Quantization != "" {
			model.Quantization = m.Quantization
		}
		models = append(models, model)
	}

	return models, nil
}
