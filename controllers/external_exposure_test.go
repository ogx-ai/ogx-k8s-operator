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

// Negative tests for RHAIENG-6602: in the greenfield (Praxis-fronted) topology the operator must
// create no Kubernetes object that routes traffic from outside the cluster to OGX, and the
// NetworkPolicy it does create must admit no externally-reachable peer on the service port.
//
// These run against envtest — a real API server, no CNI and no workloads. They therefore prove
// the *declarative* topology, not packet-level enforcement. Reachability from a genuinely
// external client is the e2e suite's and downstream OCP QE's job.

package controllers_test

import (
	"fmt"
	"os"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	controllers "github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	// openShiftIngressPolicyGroupLabel is the namespace label OpenShift puts on the namespaces
	// hosting the ingress controllers that terminate external traffic.
	openShiftIngressPolicyGroupLabel = "network.openshift.io/policy-group"
	serviceNameSuffix                = "-service"
	networkPolicyNameSuffix          = "-network-policy"
)

// greenfieldBaseConfig is a minimal distribution default. It declares responses and conversations
// so the Praxis filter has something to remove, and inference so an emptied list is
// distinguishable from a deleted one.
const greenfieldBaseConfig = `version: '2'
apis:
- inference
- responses
- conversations
- vector_io
server:
  port: 8321
`

// greenfieldInstance creates and reconciles a Praxis-fronted OGXServer in a fresh namespace,
// returning it. externalAccess mirrors spec.network.externalAccess.enabled — set it true to prove
// the operator ignores the request rather than honouring it.
//
// The CR carries spec.baseConfig rather than spec.overrideConfig: overrideConfig short-circuits
// config generation entirely, and baseConfig keeps the real generation path in play while
// avoiding the OCI-label fetch that envtest cannot perform.
func greenfieldInstance(t *testing.T, name string, externalAccess bool) *ogxiov1beta1.OGXServer {
	t.Helper()

	t.Setenv("OPERATOR_NAMESPACE", testOperatorNamespace)

	namespace := createTestNamespace(t, "greenfield")

	baseConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: name + "-base-config", Namespace: namespace.Name},
		Data:       map[string]string{"config.yaml": greenfieldBaseConfig},
	}
	require.NoError(t, k8sClient.Create(t.Context(), baseConfigMap))

	instance := NewOGXServerBuilder().
		WithName(name).
		WithNamespace(namespace.Name).
		WithDistribution("starter").
		WithBaseConfig(baseConfigMap.Name, "config.yaml").
		WithPraxisMode(true). // the webhook that defaults this does not run in envtest
		Build()
	if externalAccess {
		instance.Spec.Network = &ogxiov1beta1.NetworkSpec{
			ExternalAccess: &ogxiov1beta1.ExternalAccessConfig{Enabled: true},
		}
	}

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

	ReconcileOGXServer(t, instance)
	return instance
}

// assertNoExternalExposurePrimitive is the shared predicate: after reconcile, nothing in the
// instance's namespace routes external traffic to OGX.
func assertNoExternalExposurePrimitive(t *testing.T, instance *ogxiov1beta1.OGXServer) {
	t.Helper()
	ctx := t.Context()

	// The Ingress the operator would create in legacy mode must not exist...
	var ingress networkingv1.Ingress
	err := k8sClient.Get(ctx,
		types.NamespacedName{Name: instance.Name + controllers.IngressNameSuffix, Namespace: instance.Namespace},
		&ingress)
	assert.Truef(t, apierrors.IsNotFound(err),
		"Ingress %s%s must not exist in Praxis mode; got err=%v",
		instance.Name, controllers.IngressNameSuffix, err)

	// ...and neither must any *other* Ingress backing the OGX Service, which a name-only check
	// would miss.
	serviceName := instance.Name + serviceNameSuffix
	var ingresses networkingv1.IngressList
	require.NoError(t, k8sClient.List(ctx, &ingresses, client.InNamespace(instance.Namespace)))
	for i := range ingresses.Items {
		for _, backend := range ingressServiceBackends(&ingresses.Items[i]) {
			assert.NotEqualf(t, serviceName, backend,
				"Ingress %s routes external traffic to the OGX service", ingresses.Items[i].Name)
		}
	}

	// The Service itself must be cluster-internal only.
	var service corev1.Service
	waitForResource(t, k8sClient, instance.Namespace, serviceName, &service)
	assert.Equal(t, corev1.ServiceTypeClusterIP, service.Spec.Type,
		"OGX Service must be ClusterIP; NodePort and LoadBalancer are externally reachable")
	assert.Empty(t, service.Spec.ExternalIPs, "externalIPs bypass the service type and reach the node")
	assert.Empty(t, service.Spec.LoadBalancerIP)
	assert.Empty(t, service.Spec.ExternalName)
	for _, port := range service.Spec.Ports {
		assert.Zerof(t, port.NodePort, "port %s is published on node port %d", port.Name, port.NodePort)
	}
}

// ingressServiceBackends returns every Service name the Ingress routes to.
func ingressServiceBackends(ingress *networkingv1.Ingress) []string {
	var names []string
	if ingress.Spec.DefaultBackend != nil && ingress.Spec.DefaultBackend.Service != nil {
		names = append(names, ingress.Spec.DefaultBackend.Service.Name)
	}
	for _, rule := range ingress.Spec.Rules {
		if rule.HTTP == nil {
			continue
		}
		for _, path := range rule.HTTP.Paths {
			if path.Backend.Service != nil {
				names = append(names, path.Backend.Service.Name)
			}
		}
	}
	return names
}

func TestGreenfieldPraxisCR_CreatesNoExternalExposurePrimitive(t *testing.T) {
	instance := greenfieldInstance(t, "greenfield-exposure", false)
	assertNoExternalExposurePrimitive(t, instance)
}

// TestGreenfieldPraxisCR_ExternalAccessEnabledStillCreatesNoExposure covers the regression that
// matters most: a CR author asking for external access in Praxis mode must be ignored, not
// honoured. network_resources_test.go covers reconcileIngress in isolation with a fake client;
// this drives the real reconcile loop against a real API server.
func TestGreenfieldPraxisCR_ExternalAccessEnabledStillCreatesNoExposure(t *testing.T) {
	instance := greenfieldInstance(t, "greenfield-extaccess", true)
	assertNoExternalExposurePrimitive(t, instance)
}

// TestGreenfieldPraxisCR_DeletesAdoptedIngress verifies that an Ingress left behind by a legacy
// instance is removed once the CR flips to Praxis mode, rather than merely not being recreated.
func TestGreenfieldPraxisCR_DeletesAdoptedIngress(t *testing.T) {
	instance := greenfieldInstance(t, "greenfield-adopted", false)

	pathType := networkingv1.PathTypePrefix
	stale := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + controllers.IngressNameSuffix,
			Namespace: instance.Namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: instance.Name + serviceNameSuffix,
									Port: networkingv1.ServiceBackendPort{Number: 8321},
								},
							},
						}},
					},
				},
			}},
		},
	}
	require.NoError(t, ctrl.SetControllerReference(instance, stale, k8sClient.Scheme()))
	require.NoError(t, k8sClient.Create(t.Context(), stale))

	ReconcileOGXServer(t, instance)

	require.Eventually(t, func() bool {
		var got networkingv1.Ingress
		err := k8sClient.Get(t.Context(),
			types.NamespacedName{Name: stale.Name, Namespace: stale.Namespace}, &got)
		return apierrors.IsNotFound(err)
	}, testTimeout, testInterval, "operator must delete an owned Ingress in Praxis mode")
}

// TestGreenfieldPraxisCR_NetworkPolicyAdmitsNoExternalPeers asserts the same structural predicates
// as the transformer unit tests, but against the object the reconciler actually persisted. The
// transformer can be correct while the controller passes it the wrong config.
func TestGreenfieldPraxisCR_NetworkPolicyAdmitsNoExternalPeers(t *testing.T) {
	instance := greenfieldInstance(t, "greenfield-netpol", true)

	var np networkingv1.NetworkPolicy
	waitForResource(t, k8sClient, instance.Namespace, instance.Name+networkPolicyNameSuffix, &np)

	require.NotEmpty(t, np.Spec.Ingress, "no ingress rules at all would make this test vacuous")
	require.Equal(t, []networkingv1.PolicyType{networkingv1.PolicyTypeIngress}, np.Spec.PolicyTypes)

	violations := externalIngressPeerViolations(np.Spec.Ingress, ogxiov1beta1.DefaultServerPort)
	assert.Emptyf(t, violations,
		"persisted NetworkPolicy admits externally-reachable peers on port %d:\n  %v",
		ogxiov1beta1.DefaultServerPort, violations)
}

// externalIngressPeerViolations reports every way the rules would admit traffic originating
// outside the cluster on port. Each predicate is a real regression vector: an omitted from:
// (admits everything), a world IPBlock, a bare selector, or the OpenShift router.
func externalIngressPeerViolations(rules []networkingv1.NetworkPolicyIngressRule, port int32) []string {
	var violations []string
	for i, rule := range rules {
		if !ingressRuleOpensPort(rule, port) {
			continue
		}
		if len(rule.From) == 0 {
			violations = append(violations, fmt.Sprintf("ingress[%d]: empty from: admits every source", i))
			continue
		}
		for j, peer := range rule.From {
			if detail := externalPeerDetail(peer); detail != "" {
				violations = append(violations, fmt.Sprintf("ingress[%d].from[%d]: %s", i, j, detail))
			}
		}
	}
	return violations
}

func externalPeerDetail(peer networkingv1.NetworkPolicyPeer) string {
	if detail := worldIPBlockDetail(peer.IPBlock); detail != "" {
		return detail
	}
	return overlyBroadSelectorDetail(peer)
}

func worldIPBlockDetail(block *networkingv1.IPBlock) string {
	if block == nil {
		return ""
	}
	if block.CIDR == "0.0.0.0/0" || block.CIDR == "::/0" {
		return "ipBlock " + block.CIDR + " admits the whole internet"
	}
	return ""
}

func overlyBroadSelectorDetail(peer networkingv1.NetworkPolicyPeer) string {
	if peer.NamespaceSelector == nil {
		if peer.PodSelector != nil && isEmptyLabelSelector(peer.PodSelector) {
			return "empty podSelector with no namespaceSelector admits the whole namespace"
		}
		return ""
	}
	if isEmptyLabelSelector(peer.NamespaceSelector) {
		return "empty namespaceSelector admits every namespace"
	}
	if peer.NamespaceSelector.MatchLabels[openShiftIngressPolicyGroupLabel] == "ingress" {
		return "admits the OpenShift router, which fronts external traffic"
	}
	return ""
}

func isEmptyLabelSelector(sel *metav1.LabelSelector) bool {
	return len(sel.MatchLabels) == 0 && len(sel.MatchExpressions) == 0
}

// ingressRuleOpensPort reports whether rule admits traffic to port. An empty Ports list opens
// every port, which necessarily includes the service port.
func ingressRuleOpensPort(rule networkingv1.NetworkPolicyIngressRule, port int32) bool {
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

// TestGreenfieldPraxisCR_GeneratedConfigOmitsPraxisServedAPIs asserts the disable mechanism end to
// end through the reconciler: the ConfigMap the Deployment will mount must not list the APIs
// Praxis serves. pkg/config covers the generator in isolation; this covers the wiring.
func TestGreenfieldPraxisCR_GeneratedConfigOmitsPraxisServedAPIs(t *testing.T) {
	instance := greenfieldInstance(t, "greenfield-config", false)

	var configMaps corev1.ConfigMapList
	require.NoError(t, k8sClient.List(t.Context(), &configMaps,
		client.InNamespace(instance.Namespace),
		client.MatchingLabels{"ogx.io/generated-config": "true"}))
	require.Len(t, configMaps.Items, 1, "expected exactly one operator-generated ConfigMap")

	raw, ok := configMaps.Items[0].Data["config.yaml"]
	require.True(t, ok, "generated ConfigMap has no config.yaml key")

	var cfg struct {
		APIs []string `json:"apis"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg))

	require.NotNil(t, cfg.APIs,
		"an absent apis: list means OGX serves everything, which would silently undo the disabling")
	assert.NotContains(t, cfg.APIs, "responses", "OGX must not serve /v1/responses in Praxis mode")
	assert.NotContains(t, cfg.APIs, "conversations", "OGX must not serve /v1/conversations in Praxis mode")
	assert.Contains(t, cfg.APIs, "inference", "the filter must not empty the list wholesale")
	assert.Contains(t, cfg.APIs, "vector_io")
}

// TestOperatorRBACGrantsNoExternalExposurePrimitives is the cheapest durable guard in this ticket:
// an operator with no permission to create a Route or an HTTPRoute cannot regress into creating
// one. It is a static read of the generated ClusterRole, so it runs in milliseconds and cannot
// flake. It fails at PR time on the `make manifests` diff, before any cluster is involved.
func TestOperatorRBACGrantsNoExternalExposurePrimitives(t *testing.T) {
	data, err := os.ReadFile("../config/rbac/role.yaml")
	require.NoError(t, err, "failed to read the generated operator ClusterRole")

	var role rbacv1.ClusterRole
	require.NoError(t, yaml.Unmarshal(data, &role))
	require.NotEmpty(t, role.Rules, "parsed an empty ClusterRole; the assertions below would be vacuous")

	// Groups the operator must hold no permission in at all. Each defines an
	// externally-reachable routing primitive.
	forbiddenGroups := []string{
		"route.openshift.io",        // OpenShift Route
		"gateway.networking.k8s.io", // Gateway API Gateway / HTTPRoute
		"networking.istio.io",       // Istio Gateway / VirtualService
		"projectcontour.io",         // Contour HTTPProxy
		"traefik.containo.us",       // Traefik IngressRoute
		"elbv2.k8s.aws",             // AWS load-balancer TargetGroupBinding
		"networking.gke.io",         // GKE MultiClusterIngress / ServiceAttachment
	}

	// networking.k8s.io is needed for NetworkPolicy and for deleting legacy Ingresses, so it is
	// allow-listed by resource rather than forbidden outright.
	allowedNetworkingResources := map[string]bool{"ingresses": true, "networkpolicies": true}

	for _, rule := range role.Rules {
		for _, group := range rule.APIGroups {
			for _, forbidden := range forbiddenGroups {
				assert.NotEqualf(t, forbidden, group,
					"operator is granted %v on %s/%v; OGX is internal-only and must not be able to "+
						"create external routing objects", rule.Verbs, group, rule.Resources)
			}
			if group != "networking.k8s.io" {
				continue
			}
			for _, res := range rule.Resources {
				assert.Truef(t, allowedNetworkingResources[res],
					"operator is granted %v on networking.k8s.io/%s, which is outside the "+
						"NetworkPolicy + legacy-Ingress allow-list", rule.Verbs, res)
			}
		}
	}
}
