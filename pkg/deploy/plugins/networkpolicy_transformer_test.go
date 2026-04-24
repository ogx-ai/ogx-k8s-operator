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

	llamav1alpha1 "github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
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
      app: llama-stack
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

	// Without AllowedTo, egress should be unrestricted (no Egress policyType, no egress rules)
	assert.NotContains(t, yamlStr, "Egress")
	assert.NotContains(t, yamlStr, "k8s-app: kube-dns")
	assert.NotContains(t, yamlStr, "cidr:")
}

func TestNetworkPolicyTransformer_AllNamespaces(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			AllowedFrom: &[]networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{},
			}},
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Should have empty namespace selector (all namespaces) from user peer
	assert.Contains(t, yamlStr, "namespaceSelector: {}")

	// Should also have default peers (same namespace + operator)
	assert.Contains(t, yamlStr, "podSelector: {}")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: operator-ns")
}

func TestNetworkPolicyTransformer_ExplicitNamespaces(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			AllowedFrom: &[]networkingv1.NetworkPolicyPeer{
				{NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ns-a"},
				}},
				{NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ns-b"},
				}},
			},
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Should have explicit namespace selectors
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: ns-a")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: ns-b")

	// Should also have operator namespace
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: operator-ns")
}

func TestNetworkPolicyTransformer_LabelSelectors(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			AllowedFrom: &[]networkingv1.NetworkPolicyPeer{
				{NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key: "myproject/lls-allowed", Operator: metav1.LabelSelectorOpExists,
					}},
				}},
				{NamespaceSelector: &metav1.LabelSelector{
					MatchExpressions: []metav1.LabelSelectorRequirement{{
						Key: "team/authorized", Operator: metav1.LabelSelectorOpExists,
					}},
				}},
			},
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Should have label selectors with Exists operator
	assert.Contains(t, yamlStr, "key: myproject/lls-allowed")
	assert.Contains(t, yamlStr, "key: team/authorized")
	assert.Contains(t, yamlStr, "operator: Exists")
}

func TestNetworkPolicyTransformer_AllowedFromWithPodSelector(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			AllowedFrom: &[]networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": "frontend-ns"},
				},
				PodSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"app": "frontend"},
				},
			}},
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: frontend-ns")
	assert.Contains(t, yamlStr, "app: frontend")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: operator-ns")
	assert.Contains(t, yamlStr, "port: 8321")
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
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			ExposeRoute: true,
		},
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

func TestNetworkPolicyTransformer_AllowedTo(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	tcp := corev1.ProtocolTCP
	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		APIServerHost:     "10.96.0.1",
		APIServerPort:     443,
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			AllowedTo: &[]networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": "ollama-dist"},
					},
				}},
				Ports: []networkingv1.NetworkPolicyPort{{
					Protocol: &tcp,
					Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 11434},
				}},
			}},
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Egress policyType should be added
	assert.Contains(t, yamlStr, "Egress")

	// DNS egress rules (vanilla Kubernetes port 53 + OpenShift port 5353)
	assert.Contains(t, yamlStr, "k8s-app: kube-dns")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: kube-system")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: openshift-dns")
	assert.Contains(t, yamlStr, "port: 53")
	assert.Contains(t, yamlStr, "port: 5353")

	// API server egress rule
	assert.Contains(t, yamlStr, "cidr: 10.96.0.1/32")
	assert.Contains(t, yamlStr, "port: 443")

	// User-specified destination
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: ollama-dist")
	assert.Contains(t, yamlStr, "port: 11434")
}

func TestNetworkPolicyTransformer_AllowedToEmpty(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	// Explicitly empty AllowedTo (non-nil pointer to empty slice):
	// egress should be locked down to DNS + API server baseline only.
	emptyRules := &[]networkingv1.NetworkPolicyEgressRule{}
	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		APIServerHost:     "10.96.0.1",
		APIServerPort:     443,
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			AllowedTo: emptyRules,
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Egress policyType should be added
	assert.Contains(t, yamlStr, "Egress")

	// DNS baseline rules should be present
	assert.Contains(t, yamlStr, "k8s-app: kube-dns")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: kube-system")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: openshift-dns")

	// API server rule should be present
	assert.Contains(t, yamlStr, "cidr: 10.96.0.1/32")

	// No user-specified destinations
	assert.NotContains(t, yamlStr, "kubernetes.io/metadata.name: model-serving")
	assert.NotContains(t, yamlStr, "kubernetes.io/metadata.name: ollama-dist")
}

func TestNetworkPolicyTransformer_NetworkSpecWithNilAllowedTo(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	// NetworkSpec is set (with AllowedFrom) but AllowedTo is nil:
	// egress should be unrestricted (no Egress policyType, no egress rules).
	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			AllowedFrom: &[]networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{"kubernetes.io/metadata.name": "my-app"},
				},
			}},
			AllowedTo: nil,
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	// Egress should NOT be restricted
	assert.NotContains(t, yamlStr, "Egress")
	assert.NotContains(t, yamlStr, "k8s-app: kube-dns")
	assert.NotContains(t, yamlStr, "cidr:")
}

func TestNetworkPolicyTransformer_AllowedToWithoutPort(t *testing.T) {
	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes([]byte(networkPolicyTestYAML))
	require.NoError(t, err)

	rm := resmap.New()
	require.NoError(t, rm.Append(res))

	transformer := CreateNetworkPolicyTransformer(NetworkPolicyTransformerConfig{
		InstanceName:      "test-instance",
		ServicePort:       8321,
		OperatorNamespace: "operator-ns",
		APIServerHost:     "10.96.0.1",
		APIServerPort:     443,
		NetworkSpec: &llamav1alpha1.NetworkSpec{
			AllowedTo: &[]networkingv1.NetworkPolicyEgressRule{{
				To: []networkingv1.NetworkPolicyPeer{{
					NamespaceSelector: &metav1.LabelSelector{
						MatchLabels: map[string]string{"kubernetes.io/metadata.name": "model-serving"},
					},
				}},
			}},
		},
	})

	err = transformer.Transform(rm)
	require.NoError(t, err)

	transformedRes := rm.Resources()[0]
	yamlBytes, err := transformedRes.AsYAML()
	require.NoError(t, err)

	yamlStr := string(yamlBytes)

	assert.Contains(t, yamlStr, "Egress")
	assert.Contains(t, yamlStr, "kubernetes.io/metadata.name: model-serving")
}
