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
