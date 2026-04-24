//nolint:testpackage
package e2e

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
	"github.com/llamastack/llama-stack-k8s-operator/pkg/featureflags"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/remotecommand"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
)

const npTestTimeout = 5 * time.Minute

func isOpenShiftCluster(t *testing.T) bool {
	t.Helper()
	cfg, err := config.GetConfig()
	if err != nil {
		t.Logf("Failed to get REST config for OpenShift detection: %v", err)
		return false
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		t.Logf("Failed to create clientset for OpenShift detection: %v", err)
		return false
	}
	_, resources, err := clientset.Discovery().ServerGroupsAndResources()
	if err != nil {
		t.Logf("Partial error during API discovery: %v", err)
	}
	for _, rl := range resources {
		if strings.HasPrefix(rl.GroupVersion, "security.openshift.io/") {
			return true
		}
	}
	return false
}

// TestNetworkPolicySuite runs NetworkPolicy e2e tests.
func TestNetworkPolicySuite(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping NetworkPolicy test suite")
	}
	t.Run("should verify NetworkPolicy structure", testNetworkPolicyStructure)
	t.Run("should resolve DNS on OpenShift with egress restrictions", testDNSResolutionOnOpenShift)
}

func createNetworkPolicyTestCR(name, namespace string, extraEgress ...networkingv1.NetworkPolicyEgressRule) *v1alpha1.LlamaStackDistribution {
	tcp := corev1.ProtocolTCP
	rules := []networkingv1.NetworkPolicyEgressRule{
		{
			To: []networkingv1.NetworkPolicyPeer{{
				NamespaceSelector: &metav1.LabelSelector{
					MatchLabels: map[string]string{
						"kubernetes.io/metadata.name": "ollama-dist",
					},
				},
			}},
			Ports: []networkingv1.NetworkPolicyPort{{
				Protocol: &tcp,
				Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 11434},
			}},
		},
	}
	rules = append(rules, extraEgress...)
	return &v1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: v1alpha1.LlamaStackDistributionSpec{
			Replicas: 1,
			Server: v1alpha1.ServerSpec{
				Distribution: v1alpha1.DistributionType{Name: starterDistType},
				ContainerSpec: v1alpha1.ContainerSpec{
					Name: "llama-stack",
					Env: []corev1.EnvVar{
						{Name: "OLLAMA_INFERENCE_MODEL", Value: "llama3.2:1b"},
						{Name: "OLLAMA_URL", Value: "http://ollama-server-service.ollama-dist.svc.cluster.local:11434"},
					},
				},
			},
			Network: &v1alpha1.NetworkSpec{AllowedTo: &rules},
		},
	}
}

func testNetworkPolicyStructure(t *testing.T) {
	restore := enableNetworkPolicyFeatureFlag(t)
	defer restore()

	ns, crName := "llama-stack-np-test", "np-test"
	createTestNamespace(t, ns)
	t.Cleanup(func() { cleanupNetworkPolicyTest(t, ns, crName) })

	tcp := corev1.ProtocolTCP
	cr := createNetworkPolicyTestCR(crName, ns, networkingv1.NetworkPolicyEgressRule{
		To: []networkingv1.NetworkPolicyPeer{{
			NamespaceSelector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"kubernetes.io/metadata.name": "test-egress-target",
				},
			},
		}},
		Ports: []networkingv1.NetworkPolicyPort{{
			Protocol: &tcp,
			Port:     &intstr.IntOrString{Type: intstr.Int, IntVal: 8080},
		}},
	})
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, cr))

	np := &networkingv1.NetworkPolicy{}
	require.NoError(t, waitForNetworkPolicy(t, ns, crName+"-network-policy", np))

	// Pod selector
	require.NotNil(t, np.Spec.PodSelector.MatchLabels)
	assert.Equal(t, v1alpha1.DefaultLabelValue, np.Spec.PodSelector.MatchLabels["app"])
	assert.Equal(t, crName, np.Spec.PodSelector.MatchLabels["app.kubernetes.io/instance"])

	// Policy types
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeIngress)
	assert.Contains(t, np.Spec.PolicyTypes, networkingv1.PolicyTypeEgress)

	// Ingress: same-namespace + operator-namespace peers
	require.NotEmpty(t, np.Spec.Ingress)
	verifyIngressPeers(t, np.Spec.Ingress[0])

	// Egress: DNS + API server + user destination
	require.NotEmpty(t, np.Spec.Egress)
	verifyEgressRules(t, np.Spec.Egress)
}

func testDNSResolutionOnOpenShift(t *testing.T) {
	if !isOpenShiftCluster(t) {
		t.Skip("OpenShift-only test: DNS resolution via openshift-dns")
	}

	restore := enableNetworkPolicyFeatureFlag(t)
	defer restore()

	ns, crName := "llama-stack-dns-test", "dns-test"
	createTestNamespace(t, ns)
	t.Cleanup(func() { cleanupNetworkPolicyTest(t, ns, crName) })

	cr := createNetworkPolicyTestCR(crName, ns)
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, cr))

	require.NoError(t, EnsureResourceReady(t, TestEnv, schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	}, crName, ns, npTestTimeout, isDeploymentReady))
	require.NoError(t, WaitForPodsReady(t, TestEnv, ns, crName, npTestTimeout))

	podList, err := GetPodsForDeployment(TestEnv, TestEnv.Ctx, ns, crName)
	require.NoError(t, err)
	require.NotEmpty(t, podList.Items)

	stdout, stderr, err := execInPod(t, ns, podList.Items[0].Name, "llama-stack",
		[]string{"nslookup", "kubernetes.default.svc.cluster.local"})
	t.Logf("DNS stdout: %s", stdout)
	if stderr != "" {
		t.Logf("DNS stderr: %s", stderr)
	}
	require.NoError(t, err, "DNS lookup failed; openshift-dns egress rule may be broken")
	assert.Contains(t, stdout, "kubernetes.default.svc.cluster.local")
}

// --- verification helpers ---

func verifyIngressPeers(t *testing.T, rule networkingv1.NetworkPolicyIngressRule) {
	t.Helper()
	foundSameNS, foundOperatorNS := false, false
	for _, peer := range rule.From {
		if peer.PodSelector != nil && peer.NamespaceSelector == nil {
			foundSameNS = true
		}
		if peer.NamespaceSelector != nil && peer.PodSelector != nil {
			nsLabels := peer.NamespaceSelector.MatchLabels
			podLabels := peer.PodSelector.MatchLabels
			if nsLabels["kubernetes.io/metadata.name"] == TestOpts.OperatorNS &&
				podLabels["control-plane"] == "controller-manager" {
				foundOperatorNS = true
			}
		}
	}
	assert.True(t, foundSameNS, "Should have same-namespace ingress peer")
	assert.True(t, foundOperatorNS, "Should have operator-namespace ingress peer")
}

func verifyEgressRules(t *testing.T, rules []networkingv1.NetworkPolicyEgressRule) {
	t.Helper()
	foundDNS, foundAPI, foundUser := false, false, false
	for _, rule := range rules {
		for _, peer := range rule.To {
			if peer.NamespaceSelector != nil {
				nsName := peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"]
				if nsName == "kube-system" || nsName == "openshift-dns" {
					foundDNS = true
				}
				if nsName == "test-egress-target" {
					foundUser = true
				}
			}
			if peer.IPBlock != nil && strings.HasSuffix(peer.IPBlock.CIDR, "/32") {
				foundAPI = true
			}
		}
	}
	assert.True(t, foundDNS, "Egress should include DNS peers")
	assert.True(t, foundAPI, "Egress should include API server")
	assert.True(t, foundUser, "Egress should include user-specified destination")
}

// --- helpers ---

func createTestNamespace(t *testing.T, name string) {
	t.Helper()
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name}}
	err := TestEnv.Client.Create(TestEnv.Ctx, ns)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
}

func enableNetworkPolicyFeatureFlag(t *testing.T) func() {
	t.Helper()
	cm := &corev1.ConfigMap{}
	key := client.ObjectKey{Namespace: TestOpts.OperatorNS, Name: "llama-stack-operator-config"}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, key, cm))

	original := cm.Data[featureflags.FeatureFlagsKey]
	if cm.Data == nil {
		cm.Data = make(map[string]string)
	}
	cm.Data[featureflags.FeatureFlagsKey] = "enableNetworkPolicy:\n  enabled: true\n"
	require.NoError(t, TestEnv.Client.Update(TestEnv.Ctx, cm))
	t.Log("Enabled NetworkPolicy feature flag")

	return func() {
		cm := &corev1.ConfigMap{}
		if err := TestEnv.Client.Get(TestEnv.Ctx, key, cm); err != nil {
			t.Logf("WARNING: failed to restore ConfigMap: %v", err)
			return
		}
		if original == "" {
			delete(cm.Data, featureflags.FeatureFlagsKey)
		} else {
			cm.Data[featureflags.FeatureFlagsKey] = original
		}
		if err := TestEnv.Client.Update(TestEnv.Ctx, cm); err != nil {
			t.Logf("WARNING: failed to restore ConfigMap: %v", err)
		}
	}
}

func waitForNetworkPolicy(t *testing.T, namespace, name string, np *networkingv1.NetworkPolicy) error {
	t.Helper()
	return wait.PollUntilContextTimeout(TestEnv.Ctx, pollInterval, npTestTimeout, true, func(ctx context.Context) (bool, error) {
		err := TestEnv.Client.Get(ctx, client.ObjectKey{Namespace: namespace, Name: name}, np)
		if k8serrors.IsNotFound(err) {
			return false, nil
		}
		return err == nil, err
	})
}

func execInPod(t *testing.T, namespace, podName, containerName string, command []string) (string, string, error) {
	t.Helper()
	cfg, err := config.GetConfig()
	if err != nil {
		return "", "", fmt.Errorf("failed to get REST config: %w", err)
	}
	clientset, err := kubernetes.NewForConfig(cfg)
	if err != nil {
		return "", "", fmt.Errorf("failed to create clientset: %w", err)
	}
	req := clientset.CoreV1().RESTClient().Post().
		Resource("pods").Name(podName).Namespace(namespace).SubResource("exec").
		VersionedParams(&corev1.PodExecOptions{
			Container: containerName, Command: command, Stdout: true, Stderr: true,
		}, scheme.ParameterCodec)

	exec, err := remotecommand.NewSPDYExecutor(cfg, "POST", req.URL())
	if err != nil {
		return "", "", fmt.Errorf("failed to create SPDY executor: %w", err)
	}
	var stdout, stderr bytes.Buffer
	err = exec.StreamWithContext(TestEnv.Ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr})
	return stdout.String(), stderr.String(), err
}

func cleanupNetworkPolicyTest(t *testing.T, namespace, crName string) {
	t.Helper()
	cr := &v1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: crName, Namespace: namespace},
	}
	if err := TestEnv.Client.Delete(TestEnv.Ctx, cr); err != nil && !k8serrors.IsNotFound(err) {
		t.Logf("WARNING: failed to delete CR %s/%s: %v", namespace, crName, err)
	}
	if err := EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	}, crName, namespace, ResourceReadyTimeout); err != nil {
		t.Logf("WARNING: failed waiting for deployment deletion: %v", err)
	}
}
