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

package plugins

import (
	"fmt"

	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/yaml"
)

const serviceKind = "Service"

// ServiceTransformerConfig holds the configuration for the Service transformer.
type ServiceTransformerConfig struct {
	// MetricsPort is the port for Prometheus scraping. 0 means monitoring is disabled.
	MetricsPort int32
	// ServicePort is the main API service port.
	ServicePort int32
}

// CreateServiceTransformer creates a transformer for Service resources.
func CreateServiceTransformer(config ServiceTransformerConfig) *serviceTransformer {
	return &serviceTransformer{config: config}
}

type serviceTransformer struct {
	config ServiceTransformerConfig
}

// Transform applies the Service transformation, conditionally adding a metrics port.
func (t *serviceTransformer) Transform(m resmap.ResMap) error {
	if t.config.MetricsPort == 0 || t.config.MetricsPort == t.config.ServicePort {
		return nil
	}

	for _, res := range m.Resources() {
		if res.GetKind() != serviceKind {
			continue
		}

		yamlBytes, err := res.AsYAML()
		if err != nil {
			return fmt.Errorf("failed to get Service YAML: %w", err)
		}

		var data map[string]any
		if err := yaml.Unmarshal(yamlBytes, &data); err != nil {
			return fmt.Errorf("failed to unmarshal Service YAML: %w", err)
		}

		spec, ok := data["spec"].(map[string]any)
		if !ok {
			continue
		}

		ports, ok := spec["ports"].([]any)
		if !ok {
			continue
		}

		ports = append(ports, map[string]any{
			"name":       "metrics",
			"protocol":   "TCP",
			"port":       t.config.MetricsPort,
			"targetPort": t.config.MetricsPort,
		})
		spec["ports"] = ports

		if err := updateResource(res, data); err != nil {
			return fmt.Errorf("failed to update Service resource: %w", err)
		}
	}

	return nil
}

// Config implements the resmap.TransformerPlugin interface.
func (t *serviceTransformer) Config(_ *resmap.PluginHelpers, _ []byte) error {
	return nil
}
