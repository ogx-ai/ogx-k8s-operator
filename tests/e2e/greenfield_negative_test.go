//nolint:testpackage
package e2e

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strconv"
	"strings"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/deploy"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

// RHAIENG-6602 — greenfield negative tests: OGX Responses is not publicly reachable.
//
// RHAISTRAT-2277 requires that in the greenfield topology only Praxis exposes /v1/responses
// externally, and that OGX's own port 8321 is not reachable from outside the cluster. This suite
// proves the declarative half of that on a kind cluster:
//
//   - no Kubernetes object in the namespace routes external traffic to the OGX Service
//   - the OGX Service is ClusterIP with no node-port / load-balancer / external-IP exposure
//   - status advertises only the internal cluster-DNS endpoint
//   - the generated config omits the APIs Praxis serves, so OGX does not answer them at all
//   - a live OGX pod does not serve /v1/responses or /v1/conversations, while it does serve an
//     API that is meant to be enabled
//
// What this suite deliberately does NOT claim:
//
//   - It does not scan from genuinely outside the cluster. The probes run in-cluster; the
//     external-path assertions are structural.
//   - It does not prove NetworkPolicy enforcement. kind runs kindnet, which creates the policy
//     object but enforces nothing. TestNetworkPolicySuite gates that behind
//     OGX_E2E_TEST_NP_ENFORCEMENT.
//   - OpenShift Route and Gateway API HTTPRoute checks are vacuous where the CRDs are absent.
//     The suite logs this loudly rather than reporting a silent pass.
//
// Those three gaps need a downstream OCP/QE counterpart.

const (
	greenfieldNamespace = "ogx-greenfield-test"
	greenfieldCRName    = "ogx-greenfield"
	greenfieldPort      = 8321

	// curlProbeImage is pinned: an unpinned probe image is a silent-drift risk in a test whose
	// whole value is detecting drift.
	curlProbeImage = "curlimages/curl:8.11.1"

	// Sentinels delimiting the probe pod's structured output in its log. An exit code cannot
	// carry an HTTP status and a body, and wget/nc exit identically for a 404, a DNS failure,
	// and a timeout — exactly the silent no-op this suite must avoid.
	probeStatusPrefix = "OGXPROBE_STATUS="
	probeBodyBegin    = "OGXPROBE_BODY_BEGIN"
	probeBodyEnd      = "OGXPROBE_BODY_END"

	// probeTransportFailure is curl's %{http_code} when no HTTP response was received at all.
	probeTransportFailure = "000"
)

// externalPathViolation is one way external traffic could reach the OGX Service.
type externalPathViolation struct {
	Kind      string
	Namespace string
	Name      string
	Detail    string
}

func (v externalPathViolation) String() string {
	return fmt.Sprintf("%s %s/%s: %s", v.Kind, v.Namespace, v.Name, v.Detail)
}

// targetService identifies the OGX Service the scan looks for paths to.
type targetService struct {
	Namespace string
	Name      string
	Port      int32
	FQDN      string
	PodLabels map[string]string
}

// TestGreenfieldNegativeSuite is registered in TestE2E as "greenfield-negative".
func TestGreenfieldNegativeSuite(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping greenfield negative suite (SkipCreation=true)")
	}

	server := setupGreenfieldCR(t)
	target := targetService{
		Namespace: server.Namespace,
		Name:      server.Name + "-service",
		Port:      greenfieldPort,
		FQDN:      server.Name + "-service." + server.Namespace + ".svc.cluster.local",
		PodLabels: map[string]string{
			"app":                        "ogx",
			"app.kubernetes.io/instance": server.Name,
		},
	}

	// Pure-function positive control. If this fails, every live assertion below is meaningless,
	// so run it first and independently of the cluster.
	t.Run("exposure detector flags a known-exposed topology", func(t *testing.T) {
		runExposureDetectorControl(t, target)
	})

	t.Run("no Kubernetes object routes external traffic to the OGX service", func(t *testing.T) {
		violations := collectExternalPaths(t, target)
		assert.Emptyf(t, violations, "external paths to OGX found:\n  %s", strings.Join(formatViolations(violations), "\n  "))
	})

	t.Run("a decoy Ingress is detected by the live sweep", func(t *testing.T) {
		runDecoyIngressControl(t, target)
	})

	t.Run("operator deletes an adopted Ingress in Praxis mode", func(t *testing.T) {
		runAdoptedIngressDeletion(t, server, target)
	})

	t.Run("OGX service is ClusterIP with no node or load-balancer exposure", func(t *testing.T) {
		assertServiceIsInternalOnly(t, target)
	})

	t.Run("status reports internal-only endpoints", func(t *testing.T) {
		assertStatusIsInternalOnly(t, server)
	})

	t.Run("generated config omits the Praxis-served APIs", func(t *testing.T) {
		assertGeneratedConfigOmitsPraxisAPIs(t, server)
	})

	// The remaining subtests need a live OGX pod.
	requireNoErrorWithDebugging(t, TestEnv,
		WaitForPodsReady(t, TestEnv, server.Namespace, server.Name, ResourceReadyTimeout),
		"OGX pods must be ready before probing HTTP endpoints", server.Namespace, server.Name)

	// Anti-no-op control: if OGX answers nothing at all, the negative results below prove nothing.
	enabledAPIServed := false
	t.Run("OGX serves an enabled API", func(t *testing.T) {
		result := httpProbe(t, "gf-probe-models", target, "GET", "/v1/models", "")
		require.NotEqualf(t, probeTransportFailure, result.Status,
			"probe never reached OGX (curl error: %s); the negative probes below would be vacuous", result.Err)
		require.Equalf(t, "200", result.Status,
			"GET /v1/models should be served in Praxis mode; body: %s", result.Body)
		enabledAPIServed = true
	})

	responsesStatus := ""
	t.Run("OGX does not serve /v1/responses", func(t *testing.T) {
		requireControlPassed(t, enabledAPIServed)
		responsesStatus = assertAPINotServed(t, target, "gf-probe-responses", "/v1/responses",
			`{"model":"none","input":"hello"}`)
	})

	t.Run("OGX does not serve /v1/conversations", func(t *testing.T) {
		requireControlPassed(t, enabledAPIServed)
		assertAPINotServed(t, target, "gf-probe-conversations", "/v1/conversations", `{}`)
	})

	t.Run("a 410/501 guard returns an OpenAI-compatible error", func(t *testing.T) {
		assertGuardShapeIfPresent(t, target, responsesStatus)
	})
}

// requireControlPassed fails fast when the positive control did not run or did not pass: a
// not-served result only means something once we know OGX serves anything at all.
func requireControlPassed(t *testing.T, enabledAPIServed bool) {
	t.Helper()
	require.True(t, enabledAPIServed,
		"the 'OGX serves an enabled API' control did not pass, so a non-2xx result here would "+
			"prove nothing about the disable mechanism")
}

// setupGreenfieldCR creates the namespace, the base ConfigMap, and the Praxis-fronted CR, and
// registers cleanup. The CR uses spec.baseConfig rather than spec.overrideConfig: overrideConfig
// short-circuits config generation entirely, so the API-disabling path would never run.
func setupGreenfieldCR(t *testing.T) *ogxiov1beta1.OGXServer {
	t.Helper()

	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: greenfieldNamespace}}
	if err := TestEnv.Client.Create(TestEnv.Ctx, ns); err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	server := GetSampleCRForDistribution(t, starterDistType)
	server.Name = greenfieldCRName
	server.Namespace = greenfieldNamespace

	// Swap the sample's overrideConfig for a baseConfig so the operator generates the runtime
	// config, which is where the Praxis-served APIs are removed.
	server.Spec.BaseConfig = &ogxiov1beta1.ConfigMapKeyRef{Name: "greenfield-base-config", Key: "config.yaml"}
	server.Spec.OverrideConfig = nil

	// Force the Praxis-fronted posture. The deployed webhook defaults this on create, but being
	// explicit keeps the suite deterministic if the webhook is unavailable. The fail-safe Praxis
	// peer pins the openshift-ingress namespace; the probes run in the test namespace, so scope
	// the selector there.
	praxisOn := true
	server.Spec.PraxisMode = &ogxiov1beta1.PraxisModeSpec{
		Enabled: &praxisOn,
		PraxisSelector: &ogxiov1beta1.PraxisSelector{
			Namespace:   greenfieldNamespace,
			PodSelector: metav1.LabelSelector{MatchLabels: map[string]string{praxisLabelKey: praxisLabelValue}},
		},
	}

	// Ask for external access explicitly, to prove the request is ignored rather than honoured.
	if server.Spec.Network == nil {
		server.Spec.Network = &ogxiov1beta1.NetworkSpec{}
	}
	server.Spec.Network.ExternalAccess = &ogxiov1beta1.ExternalAccessConfig{Enabled: true}

	EnsureBaseConfigMap(t, TestEnv.Client, TestEnv.Ctx, server)
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, server))

	t.Cleanup(func() {
		ctx := context.Background()
		_ = TestEnv.Client.Delete(ctx, server)
		// Wait for the Deployment to actually go away before the next suite competes for the
		// single kind node's memory.
		_ = EnsureResourceDeleted(t, TestEnv,
			schema.GroupVersionKind{Group: "apps", Version: "v1", Kind: "Deployment"},
			server.Name, greenfieldNamespace, ResourceReadyTimeout)
		_ = TestEnv.Client.Delete(ctx, ns)
	})

	return server
}

// scanForExternalPathsToService is the detector, kept pure so it can be positive-controlled in
// memory. Each rule maps to a real regression vector.
func scanForExternalPathsToService(
	ingresses []networkingv1.Ingress,
	services []corev1.Service,
	routes []unstructured.Unstructured, // route.openshift.io/v1, empty when the CRD is absent
	httpRoutes []unstructured.Unstructured, // gateway.networking.k8s.io/v1, empty when absent
	target targetService,
) []externalPathViolation {
	var violations []externalPathViolation
	violations = append(violations, scanIngresses(ingresses, target)...)
	violations = append(violations, scanServices(services, target)...)
	violations = append(violations, scanRoutes(routes, target)...)
	violations = append(violations, scanHTTPRoutes(httpRoutes, target)...)
	return violations
}

// scanIngresses flags any Ingress with a backend naming the OGX Service, whatever the Ingress is
// called — a name-only check on "<cr>-ingress" would miss a hand-rolled one.
func scanIngresses(ingresses []networkingv1.Ingress, target targetService) []externalPathViolation {
	var violations []externalPathViolation
	for i := range ingresses {
		ing := &ingresses[i]
		for _, backend := range ingressBackendServiceNames(ing) {
			if backend == target.Name {
				violations = append(violations, externalPathViolation{
					Kind: "Ingress", Namespace: ing.Namespace, Name: ing.Name,
					Detail: "routes external HTTP traffic to service " + target.Name,
				})
				break
			}
		}
	}
	return violations
}

func ingressBackendServiceNames(ing *networkingv1.Ingress) []string {
	var names []string
	if ing.Spec.DefaultBackend != nil && ing.Spec.DefaultBackend.Service != nil {
		names = append(names, ing.Spec.DefaultBackend.Service.Name)
	}
	for _, rule := range ing.Spec.Rules {
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

// scanServices flags any Service that both selects the OGX pods and is externally reachable, plus
// any ExternalName Service anywhere in the cluster pointing at the OGX FQDN. The hardcoded
// service.yaml manifest does not protect against a second, differently-named Service.
func scanServices(services []corev1.Service, target targetService) []externalPathViolation {
	var violations []externalPathViolation
	for i := range services {
		svc := &services[i]

		if svc.Spec.Type == corev1.ServiceTypeExternalName && svc.Spec.ExternalName == target.FQDN {
			violations = append(violations, externalPathViolation{
				Kind: "Service", Namespace: svc.Namespace, Name: svc.Name,
				Detail: "ExternalName bridges to " + target.FQDN,
			})
			continue
		}

		// A pod selector only reaches pods in the Service's own namespace, so an identically
		// labelled workload elsewhere is not a path to this OGX.
		if svc.Namespace != target.Namespace || !selectsTargetPods(svc.Spec.Selector, target.PodLabels) {
			continue
		}
		violations = append(violations, externallyReachableServiceViolations(svc)...)
	}
	return violations
}

func externallyReachableServiceViolations(svc *corev1.Service) []externalPathViolation {
	var violations []externalPathViolation
	add := func(detail string) {
		violations = append(violations, externalPathViolation{
			Kind: "Service", Namespace: svc.Namespace, Name: svc.Name, Detail: detail,
		})
	}

	switch svc.Spec.Type {
	case corev1.ServiceTypeNodePort:
		add("type NodePort exposes OGX on every node's external interface")
	case corev1.ServiceTypeLoadBalancer:
		add("type LoadBalancer provisions an external load balancer for OGX")
	case corev1.ServiceTypeClusterIP, corev1.ServiceTypeExternalName:
	}
	for _, port := range svc.Spec.Ports {
		if port.NodePort != 0 {
			add(fmt.Sprintf("port %s is published on node port %d", port.Name, port.NodePort))
		}
	}
	if len(svc.Spec.ExternalIPs) > 0 {
		add("externalIPs " + strings.Join(svc.Spec.ExternalIPs, ",") + " bypass the service type")
	}
	if svc.Spec.LoadBalancerIP != "" {
		add("loadBalancerIP " + svc.Spec.LoadBalancerIP + " is set")
	}
	return violations
}

// selectsTargetPods reports whether selector is non-empty and matches the OGX pods — i.e. whether
// the Service fronts OGX regardless of what it is named.
func selectsTargetPods(selector, podLabels map[string]string) bool {
	if len(selector) == 0 {
		return false
	}
	for k, v := range selector {
		if podLabels[k] != v {
			return false
		}
	}
	return true
}

func scanRoutes(routes []unstructured.Unstructured, target targetService) []externalPathViolation {
	var violations []externalPathViolation
	for i := range routes {
		route := &routes[i]
		names := []string{nestedString(route.Object, "spec", "to", "name")}

		alternates, _, _ := unstructured.NestedSlice(route.Object, "spec", "alternateBackends")
		for _, alt := range alternates {
			if m, ok := alt.(map[string]interface{}); ok {
				names = append(names, nestedString(m, "name"))
			}
		}

		for _, name := range names {
			if name == target.Name {
				violations = append(violations, externalPathViolation{
					Kind: "Route", Namespace: route.GetNamespace(), Name: route.GetName(),
					Detail: "OpenShift Route exposes service " + target.Name + " externally",
				})
				break
			}
		}
	}
	return violations
}

func scanHTTPRoutes(httpRoutes []unstructured.Unstructured, target targetService) []externalPathViolation {
	var violations []externalPathViolation
	for i := range httpRoutes {
		hr := &httpRoutes[i]
		rules, _, _ := unstructured.NestedSlice(hr.Object, "spec", "rules")
		if backendRefsTarget(rules, target.Name) {
			violations = append(violations, externalPathViolation{
				Kind: "HTTPRoute", Namespace: hr.GetNamespace(), Name: hr.GetName(),
				Detail: "Gateway API HTTPRoute routes to service " + target.Name,
			})
		}
	}
	return violations
}

func backendRefsTarget(rules []interface{}, serviceName string) bool {
	for _, rule := range rules {
		ruleMap, ok := rule.(map[string]interface{})
		if !ok {
			continue
		}
		refs, _, _ := unstructured.NestedSlice(ruleMap, "backendRefs")
		for _, ref := range refs {
			refMap, isMap := ref.(map[string]interface{})
			if !isMap {
				continue
			}
			// kind defaults to Service when unset.
			kind := nestedString(refMap, "kind")
			if kind != "" && kind != "Service" {
				continue
			}
			if nestedString(refMap, "name") == serviceName {
				return true
			}
		}
	}
	return false
}

func nestedString(obj map[string]interface{}, fields ...string) string {
	s, _, _ := unstructured.NestedString(obj, fields...)
	return s
}

func formatViolations(violations []externalPathViolation) []string {
	out := make([]string, 0, len(violations))
	for _, v := range violations {
		out = append(out, v.String())
	}
	return out
}

// collectExternalPaths lists the live objects and runs the pure detector over them. ExternalName
// bridges and HTTPRoutes are cluster-scoped concerns, so Services and HTTPRoutes are listed
// across all namespaces; Ingresses and Routes can only target a Service in their own namespace.
func collectExternalPaths(t *testing.T, target targetService) []externalPathViolation {
	t.Helper()

	var ingressList networkingv1.IngressList
	require.NoError(t, TestEnv.Client.List(TestEnv.Ctx, &ingressList, client.InNamespace(target.Namespace)))

	var serviceList corev1.ServiceList
	require.NoError(t, TestEnv.Client.List(TestEnv.Ctx, &serviceList))

	routes := listIfCRDPresent(t, "routes.route.openshift.io",
		schema.GroupVersionKind{Group: "route.openshift.io", Version: "v1", Kind: "RouteList"},
		client.InNamespace(target.Namespace))

	httpRoutes := listIfCRDPresent(t, "httproutes.gateway.networking.k8s.io",
		schema.GroupVersionKind{Group: "gateway.networking.k8s.io", Version: "v1", Kind: "HTTPRouteList"})

	return scanForExternalPathsToService(ingressList.Items, serviceList.Items, routes, httpRoutes, target)
}

// listIfCRDPresent lists resources of the given kind, returning nothing if the CRD is not
// installed. It logs the vacuous case explicitly — a check that cannot fire must not read as a
// pass.
func listIfCRDPresent(t *testing.T, crdName string, gvk schema.GroupVersionKind, opts ...client.ListOption) []unstructured.Unstructured {
	t.Helper()

	present, err := deploy.CheckCRDExists(TestEnv.Ctx, TestEnv.Client, crdName)
	require.NoError(t, err, "failed to check for CRD %s", crdName)
	if !present {
		t.Logf("VACUOUS: %s is not installed on this cluster, so the %s assertion cannot fail here. "+
			"It must be re-run on a cluster where the CRD exists (see the downstream OCP/QE ticket).",
			crdName, gvk.Kind)
		return nil
	}

	list := &unstructured.UnstructuredList{}
	list.SetGroupVersionKind(gvk)
	require.NoError(t, TestEnv.Client.List(TestEnv.Ctx, list, opts...), "failed to list %s", gvk.Kind)
	return list.Items
}

// runExposureDetectorControl is the in-memory positive control. Without it, the live sweep is a
// test that can only pass.
func runExposureDetectorControl(t *testing.T, target targetService) {
	t.Helper()

	pathType := networkingv1.PathTypePrefix
	exposedIngress := networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "decoy", Namespace: target.Namespace},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path: "/v1/responses", PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: target.Name,
									Port: networkingv1.ServiceBackendPort{Number: target.Port},
								},
							},
						}},
					},
				},
			}},
		},
	}

	cleanService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: target.Name, Namespace: target.Namespace},
		Spec: corev1.ServiceSpec{
			Type:     corev1.ServiceTypeClusterIP,
			Selector: target.PodLabels,
			Ports:    []corev1.ServicePort{{Name: "http", Port: target.Port}},
		},
	}

	nodePortService := cleanService
	nodePortService.Name = "sneaky-nodeport"
	nodePortService.Spec.Type = corev1.ServiceTypeNodePort
	nodePortService.Spec.Ports = []corev1.ServicePort{{Name: "http", Port: target.Port, NodePort: 31234}}

	loadBalancerService := cleanService
	loadBalancerService.Name = "sneaky-lb"
	loadBalancerService.Spec.Type = corev1.ServiceTypeLoadBalancer

	externalIPService := cleanService
	externalIPService.Name = "sneaky-externalip"
	externalIPService.Spec.ExternalIPs = []string{"203.0.113.10"}

	externalNameService := corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: "bridge", Namespace: "other-ns"},
		Spec: corev1.ServiceSpec{
			Type:         corev1.ServiceTypeExternalName,
			ExternalName: target.FQDN,
		},
	}

	route := unstructuredObj("route.openshift.io/v1", "Route", target.Namespace, "ogx-route",
		map[string]interface{}{"to": map[string]interface{}{"name": target.Name}})

	httpRoute := unstructuredObj("gateway.networking.k8s.io/v1", "HTTPRoute", target.Namespace, "ogx-httproute",
		map[string]interface{}{"rules": []interface{}{
			map[string]interface{}{"backendRefs": []interface{}{
				map[string]interface{}{"name": target.Name, "port": int64(target.Port)},
			}},
		}})

	tests := []struct {
		name          string
		ingresses     []networkingv1.Ingress
		services      []corev1.Service
		routes        []unstructured.Unstructured
		httpRoutes    []unstructured.Unstructured
		wantViolation bool
	}{
		{name: "clean topology", services: []corev1.Service{cleanService}},
		{name: "ingress backing the OGX service", ingresses: []networkingv1.Ingress{exposedIngress}, wantViolation: true},
		{name: "NodePort service selecting OGX pods", services: []corev1.Service{nodePortService}, wantViolation: true},
		{name: "LoadBalancer service selecting OGX pods", services: []corev1.Service{loadBalancerService}, wantViolation: true},
		{name: "externalIPs on a service selecting OGX pods", services: []corev1.Service{externalIPService}, wantViolation: true},
		{name: "ExternalName bridge to the OGX FQDN", services: []corev1.Service{externalNameService}, wantViolation: true},
		{name: "OpenShift Route to the OGX service", routes: []unstructured.Unstructured{route}, wantViolation: true},
		{name: "Gateway API HTTPRoute to the OGX service", httpRoutes: []unstructured.Unstructured{httpRoute}, wantViolation: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			violations := scanForExternalPathsToService(tt.ingresses, tt.services, tt.routes, tt.httpRoutes, target)
			if tt.wantViolation {
				assert.NotEmpty(t, violations, "detector missed an externally-exposed topology")
				return
			}
			assert.Emptyf(t, violations, "detector false-positived on a clean topology: %v", formatViolations(violations))
		})
	}
}

func unstructuredObj(apiVersion, kind, namespace, name string, spec map[string]interface{}) unstructured.Unstructured {
	return unstructured.Unstructured{Object: map[string]interface{}{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]interface{}{"name": name, "namespace": namespace},
		"spec":       spec,
	}}
}

// runDecoyIngressControl proves the *live* sweep works — the listing and scoping, not just the
// pure function. A real Ingress is created, detected, deleted, and the sweep goes clean again.
func runDecoyIngressControl(t *testing.T, target targetService) {
	t.Helper()

	pathType := networkingv1.PathTypePrefix
	decoy := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{Name: "gf-decoy-ingress", Namespace: target.Namespace},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path: "/", PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: target.Name,
									Port: networkingv1.ServiceBackendPort{Number: target.Port},
								},
							},
						}},
					},
				},
			}},
		},
	}

	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, decoy))
	t.Cleanup(func() { _ = TestEnv.Client.Delete(context.Background(), decoy) })

	assert.NotEmpty(t, collectExternalPaths(t, target),
		"live sweep failed to detect a real Ingress backing the OGX service; the clean result "+
			"from the sweep subtest would therefore prove nothing")

	require.NoError(t, TestEnv.Client.Delete(TestEnv.Ctx, decoy))
	require.Eventually(t, func() bool {
		return len(collectExternalPaths(t, target)) == 0
	}, ResourceReadyTimeout, generalRetryInterval, "sweep should go clean once the decoy is deleted")
}

// runAdoptedIngressDeletion exercises enforceInternalOnlyIngress against a real API server: an
// Ingress owned by the CR must be removed, not merely left uncreated.
func runAdoptedIngressDeletion(t *testing.T, server *ogxiov1beta1.OGXServer, target targetService) {
	t.Helper()

	fetched := &ogxiov1beta1.OGXServer{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx,
		types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, fetched))

	pathType := networkingv1.PathTypePrefix
	controller := true
	adopted := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      server.Name + "-ingress",
			Namespace: server.Namespace,
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ogxiov1beta1.GroupVersion.String(),
				Kind:       "OGXServer",
				Name:       fetched.Name,
				UID:        fetched.UID,
				Controller: &controller,
			}},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path: "/", PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: target.Name,
									Port: networkingv1.ServiceBackendPort{Number: target.Port},
								},
							},
						}},
					},
				},
			}},
		},
	}
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, adopted))
	t.Cleanup(func() { _ = TestEnv.Client.Delete(context.Background(), adopted) })

	// Nudge the CR so the operator reconciles promptly rather than waiting for the resync.
	if fetched.Annotations == nil {
		fetched.Annotations = map[string]string{}
	}
	fetched.Annotations["ogx.io/e2e-reconcile-nudge"] = "greenfield-negative"
	require.NoError(t, TestEnv.Client.Update(TestEnv.Ctx, fetched))

	require.NoError(t, EnsureResourceDeleted(t, TestEnv,
		schema.GroupVersionKind{Group: "networking.k8s.io", Version: "v1", Kind: "Ingress"},
		adopted.Name, adopted.Namespace, ResourceReadyTimeout),
		"operator must delete an Ingress it owns while in Praxis mode")
}

func assertServiceIsInternalOnly(t *testing.T, target targetService) {
	t.Helper()

	svc := &corev1.Service{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx,
		types.NamespacedName{Name: target.Name, Namespace: target.Namespace}, svc))

	assert.Equal(t, corev1.ServiceTypeClusterIP, svc.Spec.Type,
		"OGX Service must be ClusterIP; NodePort and LoadBalancer are externally reachable")
	assert.Empty(t, svc.Spec.ExternalIPs, "externalIPs bypass the service type and reach the node")
	assert.Empty(t, svc.Spec.LoadBalancerIP)
	assert.Empty(t, svc.Spec.ExternalName)
	for _, port := range svc.Spec.Ports {
		assert.Zerof(t, port.NodePort, "port %s is published on node port %d", port.Name, port.NodePort)
	}
}

func assertStatusIsInternalOnly(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	err := wait.PollUntilContextTimeout(TestEnv.Ctx, generalRetryInterval, ResourceReadyTimeout, true,
		func(ctx context.Context) (bool, error) {
			fetched := &ogxiov1beta1.OGXServer{}
			if getErr := TestEnv.Client.Get(ctx,
				types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, fetched); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					return false, nil
				}
				return false, getErr
			}
			return fetched.Status.ServiceURL != "", nil
		})
	require.NoError(t, err, "status.serviceURL should be populated with the internal endpoint")

	fetched := &ogxiov1beta1.OGXServer{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx,
		types.NamespacedName{Name: server.Name, Namespace: server.Namespace}, fetched))
	assert.Contains(t, fetched.Status.ServiceURL, ".svc.cluster.local",
		"serviceURL should be the internal cluster DNS endpoint")
	assert.Nil(t, fetched.Status.ExternalURL, "externalURL must be empty for internal-only OGX")
}

// assertGeneratedConfigOmitsPraxisAPIs reads the config the Deployment actually mounts and checks
// the apis: list. This is deterministic and needs no running pod, so it localizes a failure the
// HTTP probes would only report as "OGX answered".
func assertGeneratedConfigOmitsPraxisAPIs(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	deployment, err := GetDeployment(TestEnv.Client, TestEnv.Ctx, server.Name, server.Namespace)
	require.NoError(t, err, "failed to get the OGX Deployment")

	configMapName := mountedConfigMapName(deployment.Spec.Template.Spec.Volumes)
	require.NotEmpty(t, configMapName, "Deployment mounts no ConfigMap holding config.yaml")

	cm := &corev1.ConfigMap{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx,
		types.NamespacedName{Name: configMapName, Namespace: server.Namespace}, cm))

	raw, ok := cm.Data["config.yaml"]
	require.Truef(t, ok, "ConfigMap %s has no config.yaml key", configMapName)

	var cfg struct {
		APIs []string `json:"apis"`
	}
	require.NoError(t, yaml.Unmarshal([]byte(raw), &cfg), "failed to parse the mounted config.yaml")

	require.NotNilf(t, cfg.APIs,
		"config.yaml declares no apis: list, which means OGX serves every API it has providers "+
			"for — the Praxis disabling would be silently undone. Config:\n%s", raw)
	assert.NotContains(t, cfg.APIs, "responses", "OGX must not serve /v1/responses in Praxis mode")
	assert.NotContains(t, cfg.APIs, "conversations", "OGX must not serve /v1/conversations in Praxis mode")
	assert.Contains(t, cfg.APIs, "inference", "the filter must not empty the apis list wholesale")
}

// mountedConfigMapName returns the name of the ConfigMap volume holding config.yaml.
func mountedConfigMapName(volumes []corev1.Volume) string {
	for _, volume := range volumes {
		if volume.ConfigMap == nil {
			continue
		}
		if len(volume.ConfigMap.Items) == 0 {
			// Whole-ConfigMap mount: the config volume is the only one named for the config.
			if strings.Contains(volume.Name, "config") {
				return volume.ConfigMap.Name
			}
			continue
		}
		for _, item := range volume.ConfigMap.Items {
			if item.Key == "config.yaml" {
				return volume.ConfigMap.Name
			}
		}
	}
	return ""
}

// probeResult is the parsed output of an HTTP probe pod.
type probeResult struct {
	Status string // curl's %{http_code}; "000" means no HTTP response was received
	Body   string
	Err    string
}

// assertAPINotServed probes an endpoint that must be disabled and asserts the response is neither
// a success nor a valid Responses-style payload. The assertion is deliberately mechanism-agnostic:
// the operator disables APIs by omitting them from the config's apis: list, and the exact status
// upstream OGX returns for an unregistered route is not contractual. An allow-set rather than
// "!= 200" so that an unexpected 500 also fails and forces a conscious decision. Returns the
// observed status so the guard-shape subtest can key off it.
func assertAPINotServed(t *testing.T, target targetService, probeName, path, body string) string {
	t.Helper()

	post := httpProbe(t, probeName+"-post", target, "POST", path, body)
	require.NotEqualf(t, probeTransportFailure, post.Status,
		"probe never reached OGX (curl error: %s); a transport failure does not prove the API is "+
			"disabled", post.Err)

	get := httpProbe(t, probeName+"-get", target, "GET", path, "")

	for _, result := range []struct {
		method string
		res    probeResult
	}{{"POST", post}, {"GET", get}} {
		if result.res.Status == probeTransportFailure {
			t.Logf("%s %s: no HTTP response received (curl error: %s); skipping the status "+
				"assertion for this method rather than reading a transport failure as a pass",
				result.method, path, result.res.Err)
			continue
		}
		code, convErr := strconv.Atoi(result.res.Status)
		require.NoErrorf(t, convErr, "unparseable status %q from %s %s", result.res.Status, result.method, path)

		t.Logf("%s %s -> HTTP %d", result.method, path, code)
		assert.Falsef(t, code >= 200 && code < 300,
			"%s %s returned %d: OGX is still serving an API that Praxis owns. Body: %s",
			result.method, path, code, result.res.Body)
		assert.Containsf(t, []int{404, 405, 410, 501}, code,
			"%s %s returned an unexpected %d. Not-served is expected to surface as 404/405 (route "+
				"omitted) or 410/501 (explicit guard); anything else needs a deliberate decision "+
				"rather than a silent pass. Body: %s", result.method, path, code, result.res.Body)
	}

	return post.Status
}

// assertGuardShapeIfPresent runs only when the disable mechanism turned out to be an explicit
// 410/501 guard. The ticket requires such a guard to return an OpenAI-compatible error body. With
// the route-omission mechanism in use today there is no guard, which this logs rather than
// skipping silently; the check activates automatically if upstream OGX ever adds one.
func assertGuardShapeIfPresent(t *testing.T, target targetService, responsesStatus string) {
	t.Helper()

	if responsesStatus != "410" && responsesStatus != "501" {
		t.Logf("No 410/501 guard in use (observed status %q for /v1/responses). The operator "+
			"disables APIs by omitting them from the config's apis: list, so there is no guard "+
			"body to shape-check. This check activates automatically if OGX adds one.", responsesStatus)
		return
	}

	result := httpProbe(t, "gf-probe-guard", target, "POST", "/v1/responses", `{"model":"none","input":"hello"}`)

	var payload struct {
		Error struct {
			Message string `json:"message"`
			Type    string `json:"type"`
		} `json:"error"`
	}
	require.NoErrorf(t, json.Unmarshal([]byte(result.Body), &payload),
		"guard response is not JSON: %s", result.Body)
	assert.NotEmpty(t, payload.Error.Message, "OpenAI-compatible errors carry error.message; got: %s", result.Body)
	assert.NotEmpty(t, payload.Error.Type, "OpenAI-compatible errors carry error.type; got: %s", result.Body)
}

// httpProbe runs a short-lived curl pod against the OGX Service and parses its sentinel-delimited
// output. The container always exits 0 so a non-2xx response is reported as data rather than as a
// pod failure.
//
// The probe pod carries the Praxis label so it is also admitted on a policy-enforcing CNI, and
// sends x-user-id / x-tenant-id because Praxis-mode configs install upstream_header auth.
func httpProbe(t *testing.T, name string, target targetService, method, path, body string) probeResult {
	t.Helper()

	url := "http://" + net.JoinHostPort(target.FQDN, strconv.Itoa(int(target.Port))) + path
	args := []string{
		"-s", "-S", "--max-time", "20",
		"-o", "/tmp/body", "-w", "%{http_code}",
		"-X", method,
		"-H", "x-user-id: e2e-greenfield",
		"-H", "x-tenant-id: e2e-tenant",
	}
	if body != "" {
		args = append(args, "-H", "Content-Type: application/json", "-d", body)
	}
	args = append(args, url)

	script := fmt.Sprintf(
		"status=$(curl %s 2>/tmp/err); "+
			"echo \"%s${status:-%s}\"; "+
			"echo %s; cat /tmp/body 2>/dev/null; echo; echo %s; "+
			"echo \"OGXPROBE_ERR=$(cat /tmp/err 2>/dev/null)\"; "+
			"exit 0",
		shellQuoteAll(args), probeStatusPrefix, probeTransportFailure, probeBodyBegin, probeBodyEnd)

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: target.Namespace,
			Labels:    map[string]string{praxisLabelKey: praxisLabelValue},
		},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{{
				Name:    "probe",
				Image:   curlProbeImage,
				Command: []string{"sh", "-c", script},
			}},
		},
	}

	_ = TestEnv.Client.Delete(TestEnv.Ctx, pod)
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, pod))
	t.Cleanup(func() { _ = TestEnv.Client.Delete(context.Background(), pod) })

	waitForProbePodTerminated(t, target.Namespace, name)

	logs, err := GetPodLogs(t, TestEnv, target.Namespace, name)
	require.NoError(t, err, "failed to read probe pod logs; the probe result is unknown")

	result := parseProbeOutput(logs)
	t.Logf("probe %s: %s %s -> status=%s err=%q", name, method, url, result.Status, result.Err)
	return result
}

// shellQuoteAll single-quotes each argument for safe interpolation into the probe's sh -c script.
func shellQuoteAll(args []string) string {
	quoted := make([]string, 0, len(args))
	for _, arg := range args {
		quoted = append(quoted, "'"+strings.ReplaceAll(arg, "'", `'\''`)+"'")
	}
	return strings.Join(quoted, " ")
}

func parseProbeOutput(logs string) probeResult {
	var result probeResult
	inBody := false
	var bodyLines []string

	for _, line := range strings.Split(logs, "\n") {
		switch {
		case strings.HasPrefix(line, probeStatusPrefix):
			result.Status = strings.TrimSpace(strings.TrimPrefix(line, probeStatusPrefix))
		case strings.TrimSpace(line) == probeBodyBegin:
			inBody = true
		case strings.TrimSpace(line) == probeBodyEnd:
			inBody = false
		case strings.HasPrefix(line, "OGXPROBE_ERR="):
			result.Err = strings.TrimSpace(strings.TrimPrefix(line, "OGXPROBE_ERR="))
		case inBody:
			bodyLines = append(bodyLines, line)
		}
	}

	result.Body = strings.TrimSpace(strings.Join(bodyLines, "\n"))
	if result.Status == "" {
		result.Status = probeTransportFailure
	}
	return result
}
