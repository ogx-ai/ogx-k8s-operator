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

// RHAIENG-6602 — the 3.6 default topology: Praxis mode off, OGX Responses served.
//
// RHAIENG-7517 flipped the default, so a CR with no spec.praxisMode — what a greenfield install
// creates and what an upgraded CR keeps — must get the legacy topology: /v1/responses left in the
// generated config, externalAccess honoured, and the router admitted by the NetworkPolicy.
//
// Every predicate here is the negation of one in external_exposure_test.go. That is deliberate:
// each file is the other's positive control, so neither can pass by accident on a reconciler that
// has stopped distinguishing the two modes.
//
// These run against envtest, which installs no admission webhooks. The fixtures therefore set
// spec.praxisMode explicitly (or leave it out explicitly) rather than relying on defaulting — what
// admission does with an unset field is pinned statically in api/v1beta1/praxis_default_test.go and
// live against a deployed webhook in tests/e2e/greenfield_default_test.go.

package controllers_test

import (
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	controllers "github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// TestDefaultCR_GeneratedConfigKeepsResponses is the core anti-regression assertion of the
// rescoped RHAIENG-6602: with no spec.praxisMode, nothing may strip the APIs Praxis would
// otherwise serve. Responses in Praxis is not ready for 3.6, so OGX has to keep answering.
//
// The fixture carries declarative config so that the generator actually runs; it must then leave
// responses alone. A legacy CR without declarative config takes a different path entirely — the
// operator generates nothing — which TestDefaultCR_WithoutDeclarativeConfigGeneratesNoConfig covers.
func TestDefaultCR_GeneratedConfigKeepsResponses(t *testing.T) {
	instance := reconciledPraxisFixture(t, "praxis-default-config", praxisFixtureOptions{
		mode:              praxisModeUnset,
		declarativeConfig: true,
	})

	apis := generatedConfigAPIs(t, instance)

	assert.Contains(t, apis, "responses",
		"a CR with no spec.praxisMode must keep serving /v1/responses; Praxis does not serve "+
			"Responses in 3.6, so stripping it here takes the API away with nothing behind it")
	assert.Contains(t, apis, "conversations",
		"a CR with no spec.praxisMode must keep serving /v1/conversations")
	assert.Contains(t, apis, "inference", "the base config's other APIs must survive untouched")
}

// TestExplicitOptOutCR_GeneratedConfigKeepsResponses covers the other way to be in legacy mode.
// spec.praxisMode.enabled: false and an absent spec.praxisMode must behave identically; a
// reconciler that only special-cased nil would pass the test above and still break every CR that
// opted out explicitly.
func TestExplicitOptOutCR_GeneratedConfigKeepsResponses(t *testing.T) {
	instance := reconciledPraxisFixture(t, "praxis-optout-config", praxisFixtureOptions{
		mode:              praxisModeOptOut,
		declarativeConfig: true,
	})

	apis := generatedConfigAPIs(t, instance)

	assert.Contains(t, apis, "responses",
		"spec.praxisMode.enabled: false must keep /v1/responses served, exactly as an absent "+
			"spec.praxisMode does")
	assert.Contains(t, apis, "conversations")
}

// generatedConfigAPIs returns the apis: list from the ConfigMap the Deployment will mount.
func generatedConfigAPIs(t *testing.T, instance *ogxiov1beta1.OGXServer) []string {
	t.Helper()

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
	require.NotEmptyf(t, cfg.APIs,
		"generated config declares no apis: list, so the assertions on it would be vacuous. Config:\n%s", raw)

	return cfg.APIs
}

// TestDefaultCR_WithoutDeclarativeConfigGeneratesNoConfig pins the shape of the plainest greenfield
// CR there is: spec.distribution and nothing else. shouldGenerateConfig gates generation on
// declarative config *or* Praxis mode, so with both absent the operator generates nothing and the
// distribution image's own config is what runs — which is how OGX ends up serving Responses by
// default without the operator having to put it back.
//
// This is worth asserting rather than assuming: if generation ever started running unconditionally
// on this path, GeneratePraxisDefaultConfig — which unconditionally strips the Praxis-served APIs —
// is the function it would reach for, and Responses would vanish from every greenfield install.
func TestDefaultCR_WithoutDeclarativeConfigGeneratesNoConfig(t *testing.T) {
	t.Setenv("OPERATOR_NAMESPACE", testOperatorNamespace)
	namespace := createTestNamespace(t, "praxis-default-bare")

	instance := NewOGXServerBuilder().
		WithName("praxis-default-bare").
		WithNamespace(namespace.Name).
		WithDistribution("starter").
		Build()
	require.Nil(t, instance.Spec.PraxisMode, "fixture must carry no spec.praxisMode")

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

	ReconcileOGXServer(t, instance)

	var configMaps corev1.ConfigMapList
	require.NoError(t, k8sClient.List(t.Context(), &configMaps,
		client.InNamespace(instance.Namespace),
		client.MatchingLabels{"ogx.io/generated-config": "true"}))
	assert.Emptyf(t, configMaps.Items,
		"a CR with neither declarative config nor Praxis mode must run the distribution's own "+
			"config verbatim; the operator generated %d ConfigMap(s) instead", len(configMaps.Items))
}

// TestDefaultCR_ExternalAccessCreatesIngress is the exposure half of the rescope. In the 3.6
// default topology externalAccess.enabled is honoured again, so a CR asking for external access
// gets an Ingress routing to the OGX Service — the very object
// TestPraxisOptIn_ExternalAccessEnabledStillCreatesNoExposure requires be absent once a CR opts in.
func TestDefaultCR_ExternalAccessCreatesIngress(t *testing.T) {
	instance := reconciledPraxisFixture(t, "praxis-default-ingress", praxisFixtureOptions{
		mode:           praxisModeUnset,
		externalAccess: true,
	})

	var ingress networkingv1.Ingress
	waitForResource(t, k8sClient, instance.Namespace, instance.Name+controllers.IngressNameSuffix, &ingress)

	backends := ingressServiceBackends(&ingress)
	assert.Containsf(t, backends, instance.Name+serviceNameSuffix,
		"the Ingress must route to the OGX Service; backends were %v", backends)
}

// TestDefaultCR_NoExternalAccessCreatesNoIngress keeps the assertion above honest. Legacy mode
// honours externalAccess; it does not expose unconditionally. Without this, an operator that
// created an Ingress for every CR would satisfy the test above and silently publish OGX for users
// who never asked.
func TestDefaultCR_NoExternalAccessCreatesNoIngress(t *testing.T) {
	instance := reconciledPraxisFixture(t, "praxis-default-noingress", praxisFixtureOptions{mode: praxisModeUnset})

	var ingresses networkingv1.IngressList
	require.NoError(t, k8sClient.List(t.Context(), &ingresses, client.InNamespace(instance.Namespace)))
	for i := range ingresses.Items {
		for _, backend := range ingressServiceBackends(&ingresses.Items[i]) {
			assert.NotEqualf(t, instance.Name+serviceNameSuffix, backend,
				"Ingress %s exposes OGX although spec.network.externalAccess.enabled is unset",
				ingresses.Items[i].Name)
		}
	}
}

// TestDefaultCR_NetworkPolicyAdmitsTheRouter asserts the legacy NetworkPolicy peers on the object
// the reconciler actually persisted. The OpenShift router peer is exactly what
// TestPraxisOptIn_NetworkPolicyAdmitsNoExternalPeers reports as a violation, so the two tests
// cannot both pass unless the reconciler really does branch on the mode.
func TestDefaultCR_NetworkPolicyAdmitsTheRouter(t *testing.T) {
	instance := reconciledPraxisFixture(t, "praxis-default-netpol", praxisFixtureOptions{
		mode:           praxisModeUnset,
		externalAccess: true,
	})

	var np networkingv1.NetworkPolicy
	waitForResource(t, k8sClient, instance.Namespace, instance.Name+networkPolicyNameSuffix, &np)

	require.NotEmpty(t, np.Spec.Ingress, "no ingress rules at all would make this test vacuous")

	violations := externalIngressPeerViolations(np.Spec.Ingress, ogxiov1beta1.DefaultServerPort)
	assert.NotEmptyf(t, violations,
		"the legacy NetworkPolicy must admit the OpenShift router on port %d so externally routed "+
			"traffic can reach OGX Responses. Finding none means either the policy silently kept "+
			"the Praxis lock-down, or the detector shared with the opt-in tests has stopped firing "+
			"— and in the latter case those tests are vacuous too. Rules: %+v",
		ogxiov1beta1.DefaultServerPort, np.Spec.Ingress)
}

// TestUpgradedCR_KeepsPraxisModeUnsetAcrossUpdates covers the upgrade case in the ticket's scope:
// a CR created before spec.praxisMode existed must not acquire it later. The mutating webhook that
// would have done so is gone (api/v1beta1/praxis_default_test.go asserts it is not registered
// anywhere), so the remaining risk is the reconciler writing the field back itself during a
// status or spec update. envtest runs no webhooks, which makes it the right place to isolate that:
// anything that sets spec.praxisMode here was set by the operator, not by admission.
func TestUpgradedCR_KeepsPraxisModeUnsetAcrossUpdates(t *testing.T) {
	instance := reconciledPraxisFixture(t, "praxis-upgraded", praxisFixtureOptions{
		mode:              praxisModeUnset,
		declarativeConfig: true,
	})
	key := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}

	// Stand in for the post-upgrade edits a real CR sees: a spec change, then another reconcile.
	fetched := &ogxiov1beta1.OGXServer{}
	require.NoError(t, k8sClient.Get(t.Context(), key, fetched))
	require.Nil(t, fetched.Spec.PraxisMode,
		"spec.praxisMode was already set after the first reconcile")

	if fetched.Annotations == nil {
		fetched.Annotations = map[string]string{}
	}
	fetched.Annotations["ogx.io/test-upgrade-nudge"] = "rhaieng-6602"
	require.NoError(t, k8sClient.Update(t.Context(), fetched))

	ReconcileOGXServer(t, instance)

	after := &ogxiov1beta1.OGXServer{}
	require.NoError(t, k8sClient.Get(t.Context(), key, after))
	assert.Nilf(t, after.Spec.PraxisMode,
		"spec.praxisMode became %+v after an update and reconcile; an existing CR must stay in "+
			"legacy mode until its author opts in", after.Spec.PraxisMode)

	// And the effective behaviour, not just the field: Responses is still served.
	assert.Contains(t, generatedConfigAPIs(t, instance), "responses",
		"an upgraded CR must still serve /v1/responses after reconciling")
}
