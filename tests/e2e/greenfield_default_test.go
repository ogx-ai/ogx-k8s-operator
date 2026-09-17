//nolint:testpackage
package e2e

import (
	"context"
	"strconv"
	"strings"
	"sync"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	admissionregistrationv1 "k8s.io/api/admissionregistration/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

// RHAIENG-6602 — the 3.6 greenfield default: Praxis mode off, OGX Responses reachable.
//
// RHAIENG-7517 flipped the operator default. Responses in Praxis is not ready for 3.6, so OGX keeps
// serving /v1/responses standalone and every OGXServer CR — new and upgraded alike — stays in
// legacy mode until its author explicitly opts in. This suite is the live proof of that, against a
// deployed operator with its admission webhooks actually registered:
//
//   - a newly created CR that says nothing about spec.praxisMode comes back with it still unset
//   - no mutating webhook for ogxservers is registered in the cluster at all, so nothing can opt a
//     CR in later either — which is what makes the upgraded-CR case hold
//   - the CR keeps spec.praxisMode unset across an update, i.e. across the edits an upgrade brings
//   - a live OGX pod actually serves /v1/responses
//   - asking for external access gets an Ingress, so Responses is externally routable
//   - opting in explicitly returns the Tech Preview admission warning, and not opting in does not
//
// The inverse topology — what an explicit opt-in buys — is TestPraxisOptInSuite. The two suites
// assert opposite things about the same operator, so neither can pass on an operator that has
// stopped distinguishing the modes.
//
// What this suite does not claim: it does not reach OGX from genuinely outside the cluster. The
// HTTP probes run in-cluster and the external-path assertions are structural, same as in the
// opt-in suite. A true external reach test needs the downstream OCP/QE counterpart.

const (
	greenfieldDefaultNamespace = "ogx-greenfield-default-test"
	greenfieldDefaultCRName    = "ogx-greenfield-default"

	// techPreviewWarningFragment is the distinguishing part of the admission warning the validating
	// webhook emits on an explicit opt-in (api/v1beta1/ogxserver_webhook.go). Matching a fragment
	// rather than the whole string lets the wording be reworded without breaking the test, while
	// still failing if the warning is dropped — which is one of the ticket's "done when" clauses.
	techPreviewWarningFragment = "Tech Preview"

	// unroutedProbePath is a path OGX certainly has no handler for. Its response is the signature
	// of "this route is not registered", which is what the /v1/responses probe must NOT look like.
	unroutedProbePath = "/v1/rhaieng6602-not-a-real-api"
)

// TestGreenfieldDefaultSuite is registered in TestE2E as "greenfield-default".
func TestGreenfieldDefaultSuite(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping greenfield default suite (SkipCreation=true)")
	}

	// Cluster-wide and CR-independent, so run it before anything is created: if a mutating webhook
	// is registered, every other assertion below is about a CR that may already have been mutated.
	t.Run("no mutating webhook for ogxservers is registered in the cluster", func(t *testing.T) {
		assertNoMutatingWebhookForOGXServers(t)
	})

	t.Run("Tech Preview warning is returned on an explicit opt-in", func(t *testing.T) {
		assertTechPreviewWarningOnOptIn(t)
	})

	server := setupGreenfieldDefaultCR(t)
	target := targetService{
		Namespace: server.Namespace,
		Name:      server.Name + "-service",
		Port:      ogxServicePort,
		FQDN:      server.Name + "-service." + server.Namespace + ".svc.cluster.local",
		PodLabels: map[string]string{
			"app":                        "ogx",
			"app.kubernetes.io/instance": server.Name,
		},
	}

	t.Run("a new CR defaults to Praxis mode disabled", func(t *testing.T) {
		assertPraxisModeUnset(t, server, "a newly created CR")
	})

	t.Run("an existing CR keeps Praxis mode disabled across an update", func(t *testing.T) {
		assertPraxisModeSurvivesUpdate(t, server)
	})

	t.Run("external access is honoured, so OGX Responses is externally routable", func(t *testing.T) {
		assertExternalPathExists(t, target)
	})

	// The remaining subtests need a live OGX pod.
	requireNoErrorWithDebugging(t, TestEnv,
		WaitForPodsReady(t, TestEnv, server.Namespace, server.Name, ResourceReadyTimeout),
		"OGX pods must be ready before probing HTTP endpoints", server.Namespace, server.Name)

	// Anti-no-op control. If OGX answers nothing, "it serves /v1/responses" could not fail.
	ogxAnswers := false
	t.Run("OGX answers at all", func(t *testing.T) {
		result := httpProbe(t, "gfd-probe-models", target, "GET", "/v1/models", "")
		require.NotEqualf(t, probeTransportFailure, result.Status,
			"probe never reached OGX (curl error: %s); every probe below would be vacuous", result.Err)
		require.Equalf(t, "200", result.Status,
			"GET /v1/models should be served in the default topology; body: %s", result.Body)
		ogxAnswers = true
	})

	// Discriminator control. The serves-Responses assertion works by showing /v1/responses does not
	// look like an unrouted path, so we first have to know what an unrouted path looks like here.
	var unrouted probeResult
	t.Run("an unrouted path is recognisably unrouted", func(t *testing.T) {
		require.True(t, ogxAnswers, "the 'OGX answers at all' control did not pass")
		unrouted = httpProbe(t, "gfd-probe-unrouted", target, "GET", unroutedProbePath, "")
		require.Equalf(t, "404", unrouted.Status,
			"expected %s to be a plain 404; got %s. Without a known unrouted response the "+
				"serves-Responses assertion has nothing to contrast against. Body: %s",
			unroutedProbePath, unrouted.Status, unrouted.Body)
	})

	t.Run("OGX serves /v1/responses", func(t *testing.T) {
		require.True(t, ogxAnswers, "the 'OGX answers at all' control did not pass")
		assertResponsesIsServed(t, target, unrouted)
	})
}

// setupGreenfieldDefaultCR creates the namespace and a CR that says nothing at all about
// spec.praxisMode — the whole point of the suite — and asks for external access, which legacy mode
// is supposed to honour.
//
// It keeps the sample's spec.overrideConfig rather than swapping in a baseConfig the way the opt-in
// suite does. overrideConfig is passed through verbatim, so what OGX serves is decided by
// config/samples/starter-config-configmap.yaml (which lists responses) and not by the generator.
// That is deliberate: this suite's job is to prove a live OGX answers /v1/responses, and the
// generator's behaviour in legacy mode is pinned deterministically in pkg/config and controllers
// unit tests instead of being inferred from a pod's HTTP responses.
func setupGreenfieldDefaultCR(t *testing.T) *ogxiov1beta1.OGXServer {
	t.Helper()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: greenfieldDefaultNamespace}}
	if err := TestEnv.Client.Create(TestEnv.Ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	server := GetSampleCRForDistribution(t, starterDistType)
	server.Name = greenfieldDefaultCRName
	server.Namespace = greenfieldDefaultNamespace

	// The CR under test must carry no spec.praxisMode whatsoever: what the operator does with an
	// unset field is exactly what is being measured.
	server.Spec.PraxisMode = nil

	if server.Spec.Network == nil {
		server.Spec.Network = &ogxiov1beta1.NetworkSpec{}
	}
	server.Spec.Network.ExternalAccess = &ogxiov1beta1.ExternalAccessConfig{Enabled: true}

	EnsureOverrideConfigMap(t, TestEnv.Client, TestEnv.Ctx, server)
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, server))

	t.Cleanup(func() {
		ctx := context.Background()
		_ = TestEnv.Client.Delete(ctx, server)
		// Wait for the Deployment to go away before the next suite competes for the single kind
		// node's memory.
		_ = EnsureResourceDeleted(t, TestEnv,
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			server.Name, greenfieldDefaultNamespace, ResourceReadyTimeout)
		_ = TestEnv.Client.Delete(ctx, ns)
	})

	return server
}

// assertPraxisModeUnset reads the CR back from the API server — after admission, after defaulting,
// after the operator has reconciled it — and requires spec.praxisMode to still be absent.
func assertPraxisModeUnset(t *testing.T, server *ogxiov1beta1.OGXServer, subject string) {
	t.Helper()

	fetched := &ogxiov1beta1.OGXServer{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx,
		types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, fetched))

	assert.Nilf(t, fetched.Spec.PraxisMode,
		"%s came back with spec.praxisMode = %+v; Praxis mode must be opt-in (RHAIENG-7517), and "+
			"defaulting it on would take /v1/responses away from an install that never asked",
		subject, fetched.Spec.PraxisMode)
}

// assertPraxisModeSurvivesUpdate covers the upgraded-CR half of the ticket. A CR that predates
// spec.praxisMode is indistinguishable from this one, and the edits an upgrade brings — a new
// image, an annotation, a resource bump — all arrive as UPDATE requests. If anything in the
// admission path or the reconciler were to fill the field in, this is where it would show.
func assertPraxisModeSurvivesUpdate(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	key := types.NamespacedName{Name: server.Name, Namespace: server.Namespace}
	fetched := &ogxiov1beta1.OGXServer{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, key, fetched))

	if fetched.Annotations == nil {
		fetched.Annotations = map[string]string{}
	}
	fetched.Annotations["ogx.io/e2e-upgrade-nudge"] = "rhaieng-6602"
	require.NoError(t, TestEnv.Client.Update(TestEnv.Ctx, fetched))

	assertPraxisModeUnset(t, server, "an existing CR after an update")
}

// assertNoMutatingWebhookForOGXServers is the live counterpart to the static guard in
// api/v1beta1/praxis_default_test.go. The static test reads what this repo generates; this one
// reads what is actually installed, which is what a stale or hand-patched deployment would differ
// on. A cluster with no mutating webhook for ogxservers cannot default spec.praxisMode on create or
// on any later update, which is what makes the upgraded-CR guarantee structural rather than
// incidental.
func assertNoMutatingWebhookForOGXServers(t *testing.T) {
	t.Helper()

	var mutating admissionregistrationv1.MutatingWebhookConfigurationList
	require.NoError(t, TestEnv.Client.List(TestEnv.Ctx, &mutating))

	for i := range mutating.Items {
		cfg := &mutating.Items[i]
		for _, wh := range cfg.Webhooks {
			for _, rule := range wh.Rules {
				if !admissionRuleCoversOGXServers(rule.APIGroups, rule.Resources) {
					continue
				}
				t.Errorf("MutatingWebhookConfiguration %s registers webhook %q for ogx.io/ogxservers "+
					"on %v; nothing may default spec.praxisMode", cfg.Name, wh.Name, rule.Operations)
			}
		}
	}

	// Without a validating webhook present the operator's admission plumbing is not deployed at
	// all, and "no mutating webhook" would be true for the wrong reason.
	var validating admissionregistrationv1.ValidatingWebhookConfigurationList
	require.NoError(t, TestEnv.Client.List(TestEnv.Ctx, &validating))

	found := false
	for i := range validating.Items {
		for _, wh := range validating.Items[i].Webhooks {
			for _, rule := range wh.Rules {
				if admissionRuleCoversOGXServers(rule.APIGroups, rule.Resources) {
					found = true
				}
			}
		}
	}
	require.True(t, found,
		"no ValidatingWebhookConfiguration for ogxservers is installed, so the operator's admission "+
			"webhooks are not deployed and the assertion above would be vacuous")
}

// admissionRuleCoversOGXServers reports whether an installed admission rule selects OGXServer.
func admissionRuleCoversOGXServers(apiGroups, resources []string) bool {
	return containsOrWildcard(apiGroups, ogxiov1beta1.GroupVersion.Group) &&
		containsOrWildcard(resources, "ogxservers")
}

func containsOrWildcard(values []string, want string) bool {
	for _, v := range values {
		if v == "*" || v == want || strings.HasPrefix(v, want+"/") {
			return true
		}
	}
	return false
}

// assertExternalPathExists is the inverse of the opt-in suite's clean-sweep assertion, run through
// the same detector. In the default topology a CR that asks for external access gets an Ingress, so
// /v1/responses is routable from outside — which is the whole reason OGX stays the Responses
// entrypoint in 3.6.
//
// Sharing the detector with TestPraxisOptInSuite is what makes both sides meaningful: a detector
// that had stopped finding anything would fail here before it could silently pass there.
func assertExternalPathExists(t *testing.T, target targetService) {
	t.Helper()

	var violations []externalPathViolation
	require.Eventuallyf(t, func() bool {
		violations = collectExternalPaths(t, target)
		return len(violations) > 0
	}, ResourceReadyTimeout, generalRetryInterval,
		"spec.network.externalAccess.enabled is set and Praxis mode is off, so the operator must "+
			"create an external path to the OGX service; the sweep found none")

	t.Logf("external paths to OGX in the default topology (expected): %s",
		strings.Join(formatViolations(violations), "; "))
}

// assertResponsesIsServed proves a live OGX registers a route for /v1/responses.
//
// It cannot simply assert a 2xx: a POST with no usable model is a legitimate error whatever the
// routing looks like. So it distinguishes "route registered, request rejected" from "route not
// registered" by contrasting against unrouted, the response to a path OGX certainly does not
// handle. A GET on /v1/responses is also probed: OGX registers POST and GET-by-id there, so an
// unmatched GET on the collection path yields 405 Method Not Allowed when the router exists and the
// same plain 404 as unrouted when it does not.
func assertResponsesIsServed(t *testing.T, target targetService, unrouted probeResult) {
	t.Helper()

	post := httpProbe(t, "gfd-probe-responses-post", target, "POST", "/v1/responses",
		`{"model":"none","input":"hello"}`)
	require.NotEqualf(t, probeTransportFailure, post.Status,
		"probe never reached OGX (curl error: %s); a transport failure proves nothing about "+
			"whether the route exists", post.Err)

	code, err := strconv.Atoi(post.Status)
	require.NoErrorf(t, err, "unparseable status %q from POST /v1/responses", post.Status)
	t.Logf("POST /v1/responses -> HTTP %d, body: %s", code, post.Body)

	// 404 with the same body as a path OGX has no handler for means the router was never
	// registered — i.e. something stripped responses from the served API surface.
	sameAsUnrouted := post.Status == unrouted.Status && post.Body == unrouted.Body
	assert.Falsef(t, sameAsUnrouted,
		"POST /v1/responses returned exactly what %s returned (HTTP %s, body %s), so OGX has no "+
			"route for it. In the default topology Praxis mode is off and OGX must keep serving "+
			"Responses — Praxis does not serve it in 3.6, so this leaves the API with nothing "+
			"behind it", unroutedProbePath, post.Status, post.Body)

	// Independent corroboration through a different mechanism: a registered router makes the
	// collection path known, so an unsupported method on it is rejected as a method error rather
	// than as an unknown path.
	get := httpProbe(t, "gfd-probe-responses-get", target, "GET", "/v1/responses", "")
	t.Logf("GET /v1/responses -> HTTP %s, body: %s", get.Status, get.Body)
	if get.Status == "405" {
		return
	}
	assert.NotEqualf(t, unrouted.Body, get.Body,
		"GET /v1/responses returned neither 405 nor a body distinguishable from %s (HTTP %s); "+
			"both readings say the route is absent", unroutedProbePath, get.Status)
}

// assertTechPreviewWarningOnOptIn covers the ticket's last clause: the operator must warn that
// Praxis-served Responses is Tech Preview when a user opts in. The warning is an admission warning,
// so it only exists on the wire — it is invisible to a client that discards warning headers, which
// the suite's shared client does. This builds a second client with a capturing warning handler.
//
// It is paired with a control that a CR which does not opt in gets no such warning, so the test
// cannot pass by matching a warning the webhook emits unconditionally.
//
// The creates are dry-run. Admission — and therefore the warning — runs in full on a dry-run
// request, but nothing is persisted, so three CRs' worth of OGX Deployments never get scheduled
// onto the single kind node just to read a response header.
func assertTechPreviewWarningOnOptIn(t *testing.T) {
	t.Helper()

	warnings, warningClient := newWarningCapturingClient(t)

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: greenfieldDefaultNamespace}}
	if err := TestEnv.Client.Create(TestEnv.Ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	tests := []struct {
		name        string
		crName      string
		praxisMode  *ogxiov1beta1.PraxisModeSpec
		wantWarning bool
	}{
		{
			name:        "explicit opt-in warns",
			crName:      "gfd-warning-optin",
			praxisMode:  &ogxiov1beta1.PraxisModeSpec{Enabled: ptrTo(true)},
			wantWarning: true,
		},
		{
			name:        "no praxisMode does not warn",
			crName:      "gfd-warning-default",
			praxisMode:  nil,
			wantWarning: false,
		},
		{
			name:        "explicit opt-out does not warn",
			crName:      "gfd-warning-optout",
			praxisMode:  &ogxiov1beta1.PraxisModeSpec{Enabled: ptrTo(false)},
			wantWarning: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := GetSampleCRForDistribution(t, starterDistType)
			server.Name = tt.crName
			server.Namespace = greenfieldDefaultNamespace
			server.Spec.PraxisMode = tt.praxisMode

			EnsureOverrideConfigMap(t, TestEnv.Client, TestEnv.Ctx, server)

			warnings.reset()
			require.NoError(t, warningClient.Create(TestEnv.Ctx, server, client.DryRunAll))

			got := warnings.contains(techPreviewWarningFragment)
			if tt.wantWarning {
				assert.Truef(t, got,
					"creating a CR with spec.praxisMode.enabled: true returned no %q warning. "+
						"Warnings seen: %v", techPreviewWarningFragment, warnings.all())
				return
			}
			assert.Falsef(t, got,
				"creating a CR that did not opt in returned a %q warning; the warning must "+
					"identify the opt-in, not fire unconditionally. Warnings seen: %v",
				techPreviewWarningFragment, warnings.all())
		})
	}
}

// capturedWarnings collects admission warning headers. The apiserver delivers them on the
// goroutine issuing the request, but the handler is shared across subtests, so guard it.
type capturedWarnings struct {
	mu       sync.Mutex
	messages []string
}

func (c *capturedWarnings) HandleWarningHeader(_ int, _ string, message string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, message)
}

func (c *capturedWarnings) reset() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = nil
}

func (c *capturedWarnings) all() []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.messages...)
}

func (c *capturedWarnings) contains(fragment string) bool {
	for _, message := range c.all() {
		if strings.Contains(message, fragment) {
			return true
		}
	}
	return false
}

// newWarningCapturingClient builds a client that records admission warnings instead of discarding
// them. controller-runtime's default client installs a logging warning handler, so the warnings
// never surface as test-visible data.
func newWarningCapturingClient(t *testing.T) (*capturedWarnings, client.Client) {
	t.Helper()

	cfg, err := config.GetConfig()
	require.NoError(t, err, "failed to load the cluster config for the warning-capturing client")

	warnings := &capturedWarnings{}
	cfg = rest.CopyConfig(cfg)
	cfg.WarningHandler = warnings

	cl, err := client.New(cfg, client.Options{Scheme: TestEnv.Client.Scheme()})
	require.NoError(t, err, "failed to build the warning-capturing client")

	return warnings, cl
}

func ptrTo[T any](v T) *T { return &v }
