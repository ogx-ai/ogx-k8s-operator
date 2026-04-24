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
	"errors"
	"fmt"

	llamav1alpha1 "github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/yaml"
)

const (
	networkPolicyKind = "NetworkPolicy"
	dnsPort           = 53
	// OpenShift DNS pods listen on port 5353; the dns-default service
	// translates 53→5353. NetworkPolicy evaluates after DNAT, so we must
	// allow the actual pod port.
	openShiftDNSPort = 5353
	// Allow traffic from OpenShift router namespaces.
	openShiftIngressPolicyGroupLabelKey   = "network.openshift.io/policy-group"
	openShiftIngressPolicyGroupLabelValue = "ingress"
)

// NetworkPolicyTransformerConfig holds the configuration for the NetworkPolicy transformer.
type NetworkPolicyTransformerConfig struct {
	// InstanceName is the name of the LlamaStackDistribution instance.
	InstanceName string
	// ServicePort is the port the service is exposed on.
	ServicePort int32
	// OperatorNamespace is the namespace where the operator is running.
	OperatorNamespace string
	// APIServerHost is the Kubernetes API server ClusterIP.
	APIServerHost string
	// APIServerPort is the Kubernetes API server port.
	APIServerPort int32
	// NetworkSpec is the network configuration from the CR spec.
	NetworkSpec *llamav1alpha1.NetworkSpec
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

	ingressRules := t.buildIngressRules()
	spec["ingress"] = ingressRules

	if t.hasEgressConfig() {
		policyTypes, _ := spec["policyTypes"].([]any)
		spec["policyTypes"] = append(policyTypes, "Egress")
		spec["egress"] = t.buildEgressRules()
	}

	return updateResource(res, data)
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

	matchLabels["app"] = llamav1alpha1.DefaultLabelValue
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
	peers = append(peers, t.buildUserPeers()...)
	return peers
}

func (t *networkPolicyTransformer) buildUserPeers() []any {
	if t.config.NetworkSpec == nil || t.config.NetworkSpec.AllowedFrom == nil {
		return nil
	}

	allowedFrom := *t.config.NetworkSpec.AllowedFrom
	var peers []any
	for i := range allowedFrom {
		peerBytes, err := yaml.Marshal(allowedFrom[i])
		if err != nil {
			continue
		}
		var peerMap map[string]any
		if err := yaml.Unmarshal(peerBytes, &peerMap); err != nil {
			continue
		}
		peers = append(peers, peerMap)
	}
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
		map[string]any{
			"podSelector": map[string]any{
				"matchLabels": map[string]any{
					"control-plane": "controller-manager",
				},
			},
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

func (t *networkPolicyTransformer) hasEgressConfig() bool {
	return t.config.NetworkSpec != nil && t.config.NetworkSpec.AllowedTo != nil
}

func (t *networkPolicyTransformer) buildEgressRules() []any {
	allowedTo := *t.config.NetworkSpec.AllowedTo
	var rules []any
	rules = append(rules,
		// Vanilla Kubernetes DNS (CoreDNS in kube-system listens on port 53)
		map[string]any{
			"to": []any{
				map[string]any{
					"podSelector": map[string]any{
						"matchLabels": map[string]any{
							"k8s-app": "kube-dns",
						},
					},
					"namespaceSelector": map[string]any{
						"matchLabels": map[string]any{
							"kubernetes.io/metadata.name": "kube-system",
						},
					},
				},
			},
			"ports": []any{
				map[string]any{"protocol": "TCP", "port": dnsPort},
				map[string]any{"protocol": "UDP", "port": dnsPort},
			},
		},
		// OpenShift 4.x DNS (CoreDNS in openshift-dns listens on port 5353)
		map[string]any{
			"to": []any{
				map[string]any{
					"podSelector": map[string]any{
						"matchLabels": map[string]any{
							"dns.operator.openshift.io/daemonset-dns": "default",
						},
					},
					"namespaceSelector": map[string]any{
						"matchLabels": map[string]any{
							"kubernetes.io/metadata.name": "openshift-dns",
						},
					},
				},
			},
			"ports": []any{
				map[string]any{"protocol": "TCP", "port": openShiftDNSPort},
				map[string]any{"protocol": "UDP", "port": openShiftDNSPort},
			},
		},
		map[string]any{
			"to": []any{
				map[string]any{
					"ipBlock": map[string]any{
						"cidr": fmt.Sprintf("%s/32", t.config.APIServerHost),
					},
				},
			},
			"ports": []any{
				map[string]any{
					"protocol": "TCP",
					"port":     t.config.APIServerPort,
				},
			},
		},
	)

	for _, rule := range allowedTo {
		ruleBytes, err := yaml.Marshal(rule)
		if err != nil {
			continue
		}
		var ruleMap map[string]any
		if err := yaml.Unmarshal(ruleBytes, &ruleMap); err != nil {
			continue
		}
		rules = append(rules, ruleMap)
	}

	return rules
}
