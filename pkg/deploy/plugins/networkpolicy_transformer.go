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
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/yaml"
)

const (
	networkPolicyKind = "NetworkPolicy"
	// AllNamespacesSelector is the special value to allow all namespaces.
	AllNamespacesSelector = "*"
	// Allow traffic from OpenShift router (ingress) and monitoring namespaces.
	openShiftIngressPolicyGroupLabelKey   = "network.openshift.io/policy-group"
	openShiftIngressPolicyGroupLabelValue = "ingress"
	openShiftMonitoringPolicyGroupValue   = "monitoring"
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
	// MetricsPort is the port for Prometheus scraping. 0 means monitoring is disabled.
	MetricsPort int32
	// PraxisPeer is the ingress peer identifying Praxis pods, the only application
	// workload allowed to reach OGX on the service port. When nil, a fail-safe default
	// peer (pods labeled app: payload-processing in the openshift-ingress namespace) is used.
	PraxisPeer *networkingv1.NetworkPolicyPeer
	// PraxisMode selects the internal-only, Praxis-fronted ingress posture. When true,
	// ingress is restricted to the Praxis peer plus the operator namespace and user rules
	// are appended additively. When false, the legacy behavior is preserved: same-namespace
	// and OpenShift-router peers, with user-provided ingress rules fully replacing the
	// defaults.
	PraxisMode bool
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

// applyNetworkPolicySpec configures the ingress/egress rules. The behavior depends on the
// deployment mode:
//   - Praxis mode: the mandatory Praxis + operator peers (plus monitoring) are always applied,
//     and any user-provided ingress rules are appended additively so a CR author cannot remove
//     the Praxis lock-down.
//   - Legacy mode: user-provided ingress rules fully replace the defaults (the pre-Praxis
//     behavior); otherwise the default same-namespace + operator + router peers are applied.
func (t *networkPolicyTransformer) applyNetworkPolicySpec(spec map[string]any) error {
	if !t.config.PraxisMode {
		return t.applyLegacyNetworkPolicySpec(spec)
	}
	return t.applyPraxisNetworkPolicySpec(spec)
}

// applyPraxisNetworkPolicySpec applies the mandatory Praxis + operator peers (plus monitoring)
// and appends any user-provided ingress rules additively.
func (t *networkPolicyTransformer) applyPraxisNetworkPolicySpec(spec map[string]any) error {
	ingressRules, err := t.buildIngressRules()
	if err != nil {
		return err
	}

	np := t.config.NetworkSpec
	hasPolicy := np != nil && np.Policy != nil

	if hasPolicy && len(np.Policy.Ingress) > 0 {
		userIngress, convErr := networkPolicyRulesToAnySlice(np.Policy.Ingress)
		if convErr != nil {
			return fmt.Errorf("failed to convert NetworkPolicy ingress rules: %w", convErr)
		}
		ingressRules = append(ingressRules, userIngress...)
	}
	spec["ingress"] = ingressRules

	if hasPolicy && len(np.Policy.Egress) > 0 {
		egress, convErr := networkPolicyEgressRulesToAnySlice(np.Policy.Egress)
		if convErr != nil {
			return fmt.Errorf("failed to convert NetworkPolicy egress rules: %w", convErr)
		}
		spec["egress"] = egress
		spec["policyTypes"] = []any{"Ingress", "Egress"}
	}

	return nil
}

// applyLegacyNetworkPolicySpec preserves the pre-Praxis behavior: user-provided ingress rules
// replace the defaults (along with any egress), otherwise the default peer set is used.
func (t *networkPolicyTransformer) applyLegacyNetworkPolicySpec(spec map[string]any) error {
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

	ingressRules, err := t.buildIngressRules()
	if err != nil {
		return err
	}
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

func (t *networkPolicyTransformer) buildIngressRules() ([]any, error) {
	peers, err := t.buildPeers()
	if err != nil {
		return nil, err
	}

	portRule := []any{
		map[string]any{
			"protocol": "TCP",
			"port":     t.config.ServicePort,
		},
	}

	monitoringRules := t.buildMonitoringIngressRules()
	rules := make([]any, 0, 1+len(monitoringRules))
	rules = append(rules, map[string]any{
		"from":  peers,
		"ports": portRule,
	})
	rules = append(rules, monitoringRules...)
	return rules, nil
}

// buildPeers builds the ingress peers for OGX.
//
// In Praxis mode the peers are:
//  1. Praxis pods — the only application workload allowed to reach OGX on the service port.
//  2. All pods from the operator namespace — control-plane traffic so the operator can poll
//     OGX status (/v1/providers, /v1/version), not application traffic.
//
// The broad same-namespace peer and the OpenShift router peer are intentionally omitted so
// OGX is reachable only from Praxis. In legacy mode those peers are retained (pre-Praxis
// behavior).
func (t *networkPolicyTransformer) buildPeers() ([]any, error) {
	if !t.config.PraxisMode {
		peers := t.buildDefaultPeers()
		peers = append(peers, t.buildRouterPeers()...)
		return peers, nil
	}

	praxisPeer, err := t.praxisPeerMap()
	if err != nil {
		return nil, fmt.Errorf("failed to build Praxis NetworkPolicy peer: %w", err)
	}

	return []any{
		praxisPeer,
		t.operatorNamespacePeer(),
	}, nil
}

// operatorNamespacePeer allows ingress from all pods in the operator namespace so the operator
// can poll OGX status endpoints.
func (t *networkPolicyTransformer) operatorNamespacePeer() map[string]any {
	return map[string]any{
		"namespaceSelector": map[string]any{
			"matchLabels": map[string]any{
				"kubernetes.io/metadata.name": t.config.OperatorNamespace,
			},
		},
	}
}

// buildDefaultPeers builds the legacy (pre-Praxis) NetworkPolicy peers:
//  1. All pods within the same namespace (no pod-level restriction).
//  2. All pods from the operator namespace.
func (t *networkPolicyTransformer) buildDefaultPeers() []any {
	return []any{
		// Allow from all pods in the same namespace
		map[string]any{
			"podSelector": map[string]any{},
		},
		t.operatorNamespacePeer(),
	}
}

// buildRouterPeers builds legacy NetworkPolicy peers for OpenShift ingress-controller traffic.
// Only applied in legacy mode when network configuration is present.
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

// praxisPeerMap returns the configured Praxis peer as a generic map for the ingress rule,
// falling back to the default peer (pods labeled app: payload-processing in the
// openshift-ingress namespace) when none is configured.
func (t *networkPolicyTransformer) praxisPeerMap() (map[string]any, error) {
	peer := t.config.PraxisPeer
	if peer == nil {
		peer = &networkingv1.NetworkPolicyPeer{
			PodSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ogxiov1beta1.DefaultLabelKey: ogxiov1beta1.DefaultPraxisPodLabelValue,
				},
			},
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					ogxiov1beta1.DefaultNamespaceNameLabel: ogxiov1beta1.DefaultPraxisNamespace,
				},
			},
		}
	}

	b, err := json.Marshal(peer)
	if err != nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(b, &out); err != nil {
		return nil, err
	}
	return out, nil
}

// buildMonitoringIngressRules adds an ingress rule allowing Prometheus scraping on the metrics port.
func (t *networkPolicyTransformer) buildMonitoringIngressRules() []any {
	if t.config.MetricsPort == 0 {
		return nil
	}

	return []any{
		map[string]any{
			"from": []any{
				map[string]any{
					"namespaceSelector": map[string]any{
						"matchLabels": map[string]any{
							openShiftIngressPolicyGroupLabelKey: openShiftMonitoringPolicyGroupValue,
						},
					},
				},
			},
			"ports": []any{
				map[string]any{
					"protocol": "TCP",
					"port":     t.config.MetricsPort,
				},
			},
		},
	}
}

// Config implements the resmap.TransformerPlugin interface.
func (t *networkPolicyTransformer) Config(_ *resmap.PluginHelpers, _ []byte) error {
	return nil
}
