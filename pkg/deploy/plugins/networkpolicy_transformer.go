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
	"encoding/json"
	"errors"
	"fmt"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/yaml"
)

const (
	networkPolicyKind = "NetworkPolicy"
	// AllNamespacesSelector is the special value to allow all namespaces.
	AllNamespacesSelector = "*"
	// Allow traffic from OpenShift router namespaces.
	openShiftIngressPolicyGroupLabelKey   = "network.openshift.io/policy-group"
	openShiftIngressPolicyGroupLabelValue = "ingress"
)

// NetworkPolicyTransformerConfig holds the configuration for the NetworkPolicy transformer.
type NetworkPolicyTransformerConfig struct {
	// InstanceName is the name of the OGXServer instance.
	InstanceName string
	// ServicePort is the port the service is exposed on.
	ServicePort int32
	// OperatorNamespace is the namespace where the operator is running.
	OperatorNamespace string
	// NetworkSpec is the network configuration from the CR spec.
	NetworkSpec *ogxiov1beta1.NetworkSpec
}

// CreateNetworkPolicyTransformer creates a transformer for NetworkPolicy resources.
func CreateNetworkPolicyTransformer(config NetworkPolicyTransformerConfig) *networkPolicyTransformer {
	return &networkPolicyTransformer{config: config}
}

type networkPolicyTransformer struct {
	config NetworkPolicyTransformerConfig
}

// Transform applies the NetworkPolicy transformation.
func (t *networkPolicyTransformer) Transform(m resmap.ResMap) error {
	for _, res := range m.Resources() {
		if res.GetKind() != networkPolicyKind {
			continue
		}

		if err := t.transformNetworkPolicy(res); err != nil {
			return fmt.Errorf("failed to transform NetworkPolicy: %w", err)
		}
	}
	return nil
}

func (t *networkPolicyTransformer) transformNetworkPolicy(res *resource.Resource) error {
	yamlBytes, err := res.AsYAML()
	if err != nil {
		return fmt.Errorf("failed to get YAML: %w", err)
	}

	var data map[string]any
	if unmarshalErr := yaml.Unmarshal(yamlBytes, &data); unmarshalErr != nil {
		return fmt.Errorf("failed to unmarshal YAML: %w", unmarshalErr)
	}

	spec, ok := data["spec"].(map[string]any)
	if !ok {
		return errors.New("failed to find spec in NetworkPolicy")
	}

	// Update pod selector with instance name
	if err := t.updatePodSelector(spec); err != nil {
		return err
	}

	if err := t.applyNetworkPolicySpec(spec); err != nil {
		return err
	}

	return updateResource(res, data)
}

// applyNetworkPolicySpec sets ingress/egress from the CR when explicitly provided; otherwise uses defaults.
func (t *networkPolicyTransformer) applyNetworkPolicySpec(spec map[string]any) error {
	np := t.config.NetworkSpec
	if np != nil && np.Policy != nil && len(np.Policy.Ingress) > 0 {
		ingress, err := networkPolicyRulesToAnySlice(np.Policy.Ingress)
		if err != nil {
			return fmt.Errorf("failed to convert NetworkPolicy ingress rules: %w", err)
		}
		spec["ingress"] = ingress
		policyTypes := []any{"Ingress"}
		if len(np.Policy.Egress) > 0 {
			egress, err := networkPolicyEgressRulesToAnySlice(np.Policy.Egress)
			if err != nil {
				return fmt.Errorf("failed to convert NetworkPolicy egress rules: %w", err)
			}
			spec["egress"] = egress
			policyTypes = append(policyTypes, "Egress")
		}
		spec["policyTypes"] = policyTypes
		return nil
	}

	ingressRules := t.buildIngressRules()
	spec["ingress"] = ingressRules
	return nil
}

func networkPolicyRulesToAnySlice(rules []networkingv1.NetworkPolicyIngressRule) ([]any, error) {
	b, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	var out []any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func networkPolicyEgressRulesToAnySlice(rules []networkingv1.NetworkPolicyEgressRule) ([]any, error) {
	b, err := json.Marshal(rules)
	if err != nil {
		return nil, err
	}
	var out []any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (t *networkPolicyTransformer) updatePodSelector(spec map[string]any) error {
	podSelector, ok := spec["podSelector"].(map[string]any)
	if !ok {
		podSelector = make(map[string]any)
		spec["podSelector"] = podSelector
	}

	matchLabels, ok := podSelector["matchLabels"].(map[string]any)
	if !ok {
		matchLabels = make(map[string]any)
		podSelector["matchLabels"] = matchLabels
	}

	matchLabels["app"] = ogxiov1beta1.DefaultLabelValue
	matchLabels["app.kubernetes.io/instance"] = t.config.InstanceName

	return nil
}

func (t *networkPolicyTransformer) buildIngressRules() []any {
	peers := t.buildPeers()

	portRule := []any{
		map[string]any{
			"protocol": "TCP",
			"port":     t.config.ServicePort,
		},
	}

	return []any{
		map[string]any{
			"from":  peers,
			"ports": portRule,
		},
	}
}

func (t *networkPolicyTransformer) buildPeers() []any {
	peers := t.buildDefaultPeers()
	peers = append(peers, t.buildRouterPeers()...)
	return peers
}

// buildDefaultPeers builds the default NetworkPolicy peers:
// 1. All pods within the same namespace (no pod-level restriction).
// 2. All pods from the operator namespace.
func (t *networkPolicyTransformer) buildDefaultPeers() []any {
	return []any{
		// Allow from all pods in the same namespace
		map[string]any{
			"podSelector": map[string]any{},
		},
		// Allow from operator namespace (no podSelector to allow all pods in the namespace)
		map[string]any{
			"namespaceSelector": map[string]any{
				"matchLabels": map[string]any{
					"kubernetes.io/metadata.name": t.config.OperatorNamespace,
				},
			},
		},
	}
}

// buildRouterPeers builds NetworkPolicy peers for ingress controller traffic.
func (t *networkPolicyTransformer) buildRouterPeers() []any {
	if t.config.NetworkSpec == nil {
		return nil
	}

	// Allow traffic from OpenShift router namespaces using label selection.
	return []any{
		map[string]any{
			"namespaceSelector": map[string]any{
				"matchLabels": map[string]any{
					openShiftIngressPolicyGroupLabelKey: openShiftIngressPolicyGroupLabelValue,
				},
			},
		},
	}
}

// Config implements the resmap.TransformerPlugin interface.
func (t *networkPolicyTransformer) Config(_ *resmap.PluginHelpers, _ []byte) error {
	return nil
}
