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
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/yaml"
)

const networkPolicyTestYAML = `
apiVersion: networking.k8s.io/v1
kind: NetworkPolicy
metadata:
  name: test-network-policy
spec:
  podSelector:
    matchLabels:
      app: ogx
  policyTypes:
  - Ingress
  ingress: []
`

func TestNetworkPolicyTransformer_Default(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		PraxisMode:        true,
		NetworkSpec:       nil, // No network spec
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	// Verify the NetworkPolicy was transformed
	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Should have pod selector with instance name
	assert.Contains(t, yamlStr, "app.kubernetes.io/instance: test-instance")

	// Default ingress allows only Praxis pods (fail-safe label) plus the operator namespace.
	assert.Contains(t, yamlStr, "app: payload-processing")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: operator-ns")

	// Must NOT allow all pods in the same namespace, nor the OpenShift router.
	assert.NotContains(t, yamlStr, "podSelector: {}")
	assert.NotContains(t, yamlStr, "network.openshift.io/policy-group: ingress")

	// Should have port rule
	assert.Contains(t, yamlStr, "port: 8321")
}

// TestNetworkPolicyTransformer_ExplicitIngressFromCR verifies user ingress rules are appended
// additively on top of the mandatory Praxis + operator peers (rather than replacing them).
func TestNetworkPolicyTransformer_ExplicitIngressFromCR(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	proto := corev1.ProtocolTCP
	ingress := []networkingv1.NetworkPolicyIngressRule{
		{
			From: []networkingv1.NetworkPolicyPeer{
				{NamespaceSelector: &metav1.LabelSelector{}},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{
					Protocol: &proto,
					Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8321},
				},
			},
		},
	}

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		PraxisMode:        true,
		NetworkSpec: &ogxiov1beta1.NetworkSpec{
			Policy: &ogxiov1beta1.NetworkPolicySpec{
				Ingress: ingress,
			},
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)
	yamlStr := string(yamlBytes)

	// User rule is present...
	assert.Contains(t, yamlStr, "namespaceSelector: {}")
	// ...and the mandatory Praxis + operator peers are still present (additive, not replacing).
	assert.Contains(t, yamlStr, "app: payload-processing")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: operator-ns")
}

func TestNetworkPolicyTransformer_CustomPort(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       9000,
		OperatorNamespace: "operator-ns",
		PraxisMode:        true,
		NetworkSpec:       nil,
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Should have custom port
	assert.Contains(t, yamlStr, "port: 9000")
}

// TestNetworkPolicyTransformer_NoRouterPeers verifies the OpenShift router peer is never
// emitted — OGX is internal-only, so no external ingress-controller traffic is admitted,
// whether or not a NetworkSpec is provided.
func TestNetworkPolicyTransformer_NoRouterPeers(t *testing.T) {
	for _, tc := range []struct {
		name    string
		network *ogxiov1beta1.NetworkSpec
	}{
		{name: "network spec provided", network: &ogxiov1beta1.NetworkSpec{}},
		{name: "network spec nil", network: nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			rf := resource.NewFactory(nil)
			res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
			require.NoError(t, err)

			rm := resmap.New()
			require.NoError(t, rm.Append(res))

			transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
				InstanceName:      "test-instance",
				ServicePort:       8321,
				OperatorNamespace: "operator-ns",
				PraxisMode:        true,
				NetworkSpec:       tc.network,
			})

			require.NoError(t, transformer.Transform(rm))

			yamlBytes, err := rm.Resources()[0].AsYAML()
			require.NoError(t, err)

			assert.NotContains(t, string(yamlBytes), "network.openshift.io/policy-group: ingress")
		})
	}
}

// TestNetworkPolicyTransformer_PraxisPeerFromConfig verifies a configured Praxis peer is used
// verbatim in place of the fail-safe default.
func TestNetworkPolicyTransformer_PraxisPeerFromConfig(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		PraxisMode:        true,
		PraxisPeer: &networkingv1.NetworkPolicyPeer{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"kubernetes.io/metadata.name": "praxis-ns"},
			},
			PodSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "custom-praxis"},
			},
		},
	})

	require.NoError(t, transformer.Transform(rm))

	yamlBytes, err := rm.Resources()[0].AsYAML()
	require.NoError(t, err)
	yamlStr := string(yamlBytes)

	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: praxis-ns")
	assert.Contains(t, yamlStr, "app: custom-praxis")
	// The fail-safe default must not appear when a peer is configured.
	assert.NotContains(t, yamlStr, "app: payload-processing")
}

func TestNetworkPolicyTransformer_MonitoringIngressWhenMetricsPortSet(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		PraxisMode:        true,
		NetworkSpec:       nil,
		MetricsPort:       9464,
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	assert.Contains(t, yamlStr, "network.openshift.io/policy-group: monitoring")
	assert.Contains(t, yamlStr, "port: 9464")
}

func TestNetworkPolicyTransformer_NoMonitoringIngressWhenMetricsPortZero(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		PraxisMode:        true,
		NetworkSpec:       nil,
		MetricsPort:       0,
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	assert.NotContains(t, yamlStr, "network.openshift.io/policy-group: monitoring")
}

// TestNetworkPolicyTransformer_LegacyMode verifies that with PraxisMode disabled the transformer
// preserves the pre-Praxis behavior: the same-namespace peer and the OpenShift router peer are
// emitted, and the Praxis peer is not.
func TestNetworkPolicyTransformer_LegacyMode(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		// NetworkSpec present so the legacy router peer is included.
		NetworkSpec: &ogxiov1beta1.NetworkSpec{},
		PraxisMode:  false,
	})

	require.NoError(t, transformer.Transform(rm))

	yamlBytes, err := rm.Resources()[0].AsYAML()
	require.NoError(t, err)
	yamlStr := string(yamlBytes)

	// Legacy peers: same-namespace and OpenShift router.
	assert.Contains(t, yamlStr, "podSelector: {}")
	assert.Contains(t, yamlStr, "network.openshift.io/policy-group: ingress")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: operator-ns")

	// The Praxis fail-safe peer must NOT appear in legacy mode.
	assert.NotContains(t, yamlStr, "app: payload-processing")
}

// renderNetworkPolicy runs the transformer over the base fixture and decodes the result into a
// typed NetworkPolicy. The string-matching assertions above cannot express "no peer anywhere
// admits the world" — that predicate needs the structure, not the text.
func renderNetworkPolicy(t *testing.T, config NetworkPolicyTransformerConfig) *networkingv1.NetworkPolicy {
	t.Helper()

	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))
	require.NoError(t, CreateNetworkPolicyTransformer(config).Transform(rm))

	yamlBytes, err := rm.Resources()[0].AsYAML()
	require.NoError(t, err)

	np := &networkingv1.NetworkPolicy{}
	require.NoError(t, yaml.Unmarshal(yamlBytes, np), "rendered NetworkPolicy:\n%s", yamlBytes)
	return np
}

// externalPeerViolations reports every way the given rules would admit traffic originating from
// outside the cluster on servicePort. Each predicate corresponds to a real regression vector:
// an omitted from:, a world IPBlock, a bare pod selector, a bare namespace selector, or the
// OpenShift router.
func externalPeerViolations(rules []networkingv1.NetworkPolicyIngressRule, servicePort int32) []string {
	var violations []string
	for i, rule := range rules {
		if !ruleOpensPort(rule, servicePort) {
			continue
		}
		if len(rule.From) == 0 {
			violations = append(violations, fmt.Sprintf("ingress[%d]: empty from: admits every source", i))
			continue
		}
		for j, peer := range rule.From {
			if v := peerViolation(peer); v != "" {
				violations = append(violations, fmt.Sprintf("ingress[%d].from[%d]: %s", i, j, v))
			}
		}
	}
	return violations
}

func peerViolation(peer networkingv1.NetworkPolicyPeer) string {
	if violation := worldIPBlockViolation(peer.IPBlock); violation != "" {
		return violation
	}
	return broadSelectorViolation(peer)
}

func worldIPBlockViolation(block *networkingv1.IPBlock) string {
	if block == nil {
		return ""
	}
	if block.CIDR == "0.0.0.0/0" || block.CIDR == "::/0" {
		return "ipBlock " + block.CIDR + " admits the whole internet"
	}
	return ""
}

func broadSelectorViolation(peer networkingv1.NetworkPolicyPeer) string {
	if peer.NamespaceSelector == nil {
		if peer.PodSelector != nil && isEmptySelector(peer.PodSelector) {
			return "empty podSelector with no namespaceSelector admits the whole namespace"
		}
		return ""
	}
	if isEmptySelector(peer.NamespaceSelector) {
		return "empty namespaceSelector admits every namespace"
	}
	if peer.NamespaceSelector.MatchLabels[openShiftIngressPolicyGroupLabelKey] == openShiftIngressPolicyGroupLabelValue {
		return "admits the OpenShift router, which fronts external traffic"
	}
	return ""
}

func isEmptySelector(sel *metav1.LabelSelector) bool {
	return len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0
}

// ruleOpensPort reports whether rule admits traffic to port. A nil/empty Ports list opens every
// port, which necessarily includes the service port.
func ruleOpensPort(rule networkingv1.NetworkPolicyIngressRule, port int32) bool {
	if len(rule.Ports) == 0 {
		return true
	}
	for _, p := range rule.Ports {
		if p.Port == nil || p.Port.IntValue() == int(port) {
			return true
		}
	}
	return false
}

// TestNetworkPolicyTransformer_PraxisModeAdmitsNoExternalPeers is the core negative assertion for
// RHAIENG-6602: in Praxis-fronted mode no ingress rule opening the OGX service port may admit a
// peer reachable from outside the cluster.
func TestNetworkPolicyTransformer_PraxisModeAdmitsNoExternalPeers(t *testing.T) {
	const servicePort int32 = 8321
	proto := corev1.ProtocolTCP

	// A realistically-scoped user rule. A wide-open user rule would legitimately trip these
	// predicates — Praxis-mode user rules are additive, which is a known gap tracked separately
	// and characterized by TestNetworkPolicyTransformer_ExplicitIngressFromCR.
	scopedUserIngress := []networkingv1.NetworkPolicyIngressRule{
		{
			From: []networkingv1.NetworkPolicyPeer{
				{NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": "team-ns"},
				}},
			},
			Ports: []networkingv1.NetworkPolicyPort{
				{Protocol: &proto, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: servicePort}},
			},
		},
	}

	tests := []struct {
		name   string
		config NetworkPolicyTransformerConfig
	}{
		{
			name: "no user rules",
			config: NetworkPolicyTransformerConfig{
				InstanceName: "test-instance", ServicePort: servicePort,
				OperatorNamespace: "operator-ns", PraxisMode: true,
			},
		},
		{
			name: "empty network spec",
			config: NetworkPolicyTransformerConfig{
				InstanceName: "test-instance", ServicePort: servicePort,
				OperatorNamespace: "operator-ns", PraxisMode: true,
				NetworkSpec: &ogxiov1beta1.NetworkSpec{},
			},
		},
		{
			name: "scoped user ingress rules",
			config: NetworkPolicyTransformerConfig{
				InstanceName: "test-instance", ServicePort: servicePort,
				OperatorNamespace: "operator-ns", PraxisMode: true,
				NetworkSpec: &ogxiov1beta1.NetworkSpec{
					Policy: &ogxiov1beta1.NetworkPolicySpec{Ingress: scopedUserIngress},
				},
			},
		},
		{
			name: "monitoring enabled",
			config: NetworkPolicyTransformerConfig{
				InstanceName: "test-instance", ServicePort: servicePort,
				OperatorNamespace: "operator-ns", PraxisMode: true,
				MetricsPort: 9464,
			},
		},
		{
			name: "custom Praxis peer",
			config: NetworkPolicyTransformerConfig{
				InstanceName: "test-instance", ServicePort: servicePort,
				OperatorNamespace: "operator-ns", PraxisMode: true,
				PraxisPeer: &networkingv1.NetworkPolicyPeer{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": "praxis-ns"},
					},
					PodSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"app": "custom-praxis"},
					},
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			np := renderNetworkPolicy(t, tt.config)

			require.NotEmpty(t, np.Spec.Ingress, "transformer emitted no ingress rules at all")
			assert.Empty(t, externalPeerViolations(np.Spec.Ingress, servicePort),
				"Praxis-mode NetworkPolicy admits externally-reachable peers on port %d", servicePort)
		})
	}
}

// TestNetworkPolicyTransformer_PraxisModeRestrictsToServicePortOnly verifies that every emitted
// rule scopes itself to a port. A rule with no ports opens the entire pod.
func TestNetworkPolicyTransformer_PraxisModeRestrictsToServicePortOnly(t *testing.T) {
	np := renderNetworkPolicy(t, NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		PraxisMode:        true,
		MetricsPort:       9464,
	})

	require.Len(t, np.Spec.Ingress, 2, "expected the Praxis rule plus the monitoring rule")

	for i, rule := range np.Spec.Ingress {
		require.NotEmpty(t, rule.Ports, "ingress[%d] has no ports, which opens every port", i)
		for j, port := range rule.Ports {
			require.NotNil(t, port.Port, "ingress[%d].ports[%d] has no port, which opens every port", i, j)
			require.NotNil(t, port.Protocol, "ingress[%d].ports[%d] has no protocol", i, j)
			assert.Equal(t, corev1.ProtocolTCP, *port.Protocol)
		}
	}

	praxisRule := np.Spec.Ingress[0]
	require.Len(t, praxisRule.Ports, 1)
	assert.Equal(t, 8321, praxisRule.Ports[0].Port.IntValue())
	assert.Len(t, praxisRule.From, 2, "expected exactly the Praxis peer and the operator namespace peer")
}

// TestNetworkPolicyTransformer_LegacyModeStillAdmitsRouter keeps the negative tests above honest:
// the predicates must be capable of firing, so they must flag the legacy posture they were
// written to exclude.
func TestNetworkPolicyTransformer_LegacyModeStillAdmitsRouter(t *testing.T) {
	np := renderNetworkPolicy(t, NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec:       &ogxiov1beta1.NetworkSpec{},
		PraxisMode:        false,
	})

	violations := externalPeerViolations(np.Spec.Ingress, 8321)
	assert.NotEmpty(t, violations,
		"legacy mode admits the OpenShift router and the whole namespace; if this passes, the "+
			"Praxis-mode negative assertions are vacuous")
}

// TestNetworkPolicyTransformer_LegacyModeUserIngressReplaces verifies that in legacy mode
// user-provided ingress rules fully replace the defaults (pre-Praxis semantics).
func TestNetworkPolicyTransformer_LegacyModeUserIngressReplaces(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	proto := corev1.ProtocolTCP
	ingress := []networkingv1.NetworkPolicyIngressRule{
		{
			From:  []networkingv1.NetworkPolicyPeer{{NamespaceSelector: &metav1.LabelSelector{}}},
			Ports: []networkingv1.NetworkPolicyPort{{Protocol: &proto, Port: &intstr.IntOrString{Type: intstr.Int, IntVal: 8321}}},
		},
	}

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec: &ogxiov1beta1.NetworkSpec{
			Policy: &ogxiov1beta1.NetworkPolicySpec{Ingress: ingress},
		},
		PraxisMode: false,
	})

	require.NoError(t, transformer.Transform(rm))

	yamlBytes, err := rm.Resources()[0].AsYAML()
	require.NoError(t, err)
	yamlStr := string(yamlBytes)

	// User rule present; defaults (operator namespace peer) replaced, not appended.
	assert.Contains(t, yamlStr, "namespaceSelector: {}")
	assert.NotContains(t, yamlStr, "kubernetes.io/metadata.name: operator-ns")
	assert.NotContains(t, yamlStr, "app: payload-processing")
}
