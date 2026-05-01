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

	// Should have ingress rules with default peers
	assert.Contains(t, yamlStr, "podSelector: {}")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: operator-ns")

	// Should have port rule
	assert.Contains(t, yamlStr, "port: 8321")
}

// TestNetworkPolicyTransformer_ExplicitIngressFromCR uses v1beta1 NetworkPolicySpec.Ingress verbatim.
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

	assert.Contains(t, yamlStr, "namespaceSelector: {}")
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

func TestNetworkPolicyTransformer_RouterPeersWhenNetworkSpecProvided(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec:       &ogxiov1beta1.NetworkSpec{},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Should have OpenShift router namespace selector when network spec is provided
	assert.Contains(t, yamlStr, "network.openshift.io/policy-group: ingress")
}

func TestNetworkPolicyTransformer_NoRouterPeersWhenNetworkSpecNil(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec:       nil,
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Should NOT have OpenShift router namespace selector when network spec is nil
	assert.NotContains(t, yamlStr, "network.openshift.io/policy-group: ingress")
}
