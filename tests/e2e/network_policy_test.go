//nolint:testpackage
package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
)

const (
	// npEnforcementEnvVar gates the CNI-dependent traffic-blocking test. It only produces a
	// meaningful result on a policy-enforcing CNI (OVN-Kubernetes / Calico); on non-enforcing
	// CNIs such as kind's kindnet the NetworkPolicy object exists but nothing is blocked.
	npEnforcementEnvVar = "OGX_E2E_TEST_NP_ENFORCEMENT"

	praxisLabelKey   = "app"
	praxisLabelValue = "payload-processing"
)

// TestNetworkPolicySuite verifies OGX is enforced internal-only: the operator-managed
// NetworkPolicy admits traffic only from Praxis pods plus the operator namespace, no external
// exposure (Ingress) is created even when requested, and the status reflects internal-only.
func TestNetworkPolicySuite(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping NetworkPolicy suite (SkipCreation=true)")
	}

	nsName := "ogx-np-test"
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	err := TestEnv.Client.Create(TestEnv.Ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	server := GetSampleCRForDistribution(t, starterDistType)
	server.Name = "ogx-np"
	server.Namespace = nsName
	// Request external access explicitly to prove it is NOT honored (internal-only).
	if server.Spec.Network == nil {
		server.Spec.Network = &ogxiov1beta1.NetworkSpec{}
	}
	server.Spec.Network.ExternalAccess = &ogxiov1beta1.ExternalAccessConfig{Enabled: true}

	EnsureOverrideConfigMap(t, TestEnv.Client, TestEnv.Ctx, server)
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, server))

	t.Cleanup(func() {
		_ = TestEnv.Client.Delete(context.Background(), server)
		_ = TestEnv.Client.Delete(context.Background(), ns)
	})

	npName := server.Name + "-network-policy"

	t.Run("NetworkPolicy admits only Praxis and operator peers", func(t *testing.T) {
		np := waitForNetworkPolicy(t, nsName, npName)

		require.NotEmpty(t, np.Spec.Ingress, "NetworkPolicy should have ingress rules")

		require.True(t, hasPraxisPeer(np), "NetworkPolicy must allow ingress from Praxis pods (%s=%s)", praxisLabelKey, praxisLabelValue)
		require.True(t, hasOperatorNamespacePeer(np, TestOpts.OperatorNS), "NetworkPolicy must allow ingress from the operator namespace")
		require.False(t, hasSameNamespacePeer(np), "NetworkPolicy must NOT allow ingress from all pods in the same namespace")
		require.False(t, hasRouterPeer(np), "NetworkPolicy must NOT allow ingress from the OpenShift router")
	})

	t.Run("no external Ingress is created", func(t *testing.T) {
		var ing networkingv1.Ingress
		getErr := TestEnv.Client.Get(TestEnv.Ctx, types.NamespacedName{
			Name:      server.Name + "-ingress",
			Namespace: nsName,
		}, &ing)
		require.True(t, apierrors.IsNotFound(getErr),
			"no Ingress should exist even though externalAccess.enabled=true; got err=%v", getErr)
	})

	t.Run("status is internal-only", func(t *testing.T) {
		err := wait.PollUntilContextTimeout(TestEnv.Ctx, generalRetryInterval, ResourceReadyTimeout, true,
			func(ctx context.Context) (bool, error) {
				fetched := &ogxiov1beta1.OGXServer{}
				if getErr := TestEnv.Client.Get(ctx, types.NamespacedName{Name: server.Name, Namespace: nsName}, fetched); getErr != nil {
					if apierrors.IsNotFound(getErr) {
						return false, nil
					}
					return false, getErr
				}
				return fetched.Status.ServiceURL != "", nil
			})
		require.NoError(t, err, "status.serviceURL should be populated with the internal endpoint")

		fetched := &ogxiov1beta1.OGXServer{}
		require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, types.NamespacedName{Name: server.Name, Namespace: nsName}, fetched))
		require.Contains(t, fetched.Status.ServiceURL, ".svc.cluster.local", "serviceURL should be the internal cluster DNS endpoint")
		require.Nil(t, fetched.Status.ExternalURL, "externalURL must be empty for internal-only OGX")
	})

	t.Run("traffic enforcement (CNI-dependent)", func(t *testing.T) {
		if os.Getenv(npEnforcementEnvVar) != "true" {
			t.Skipf("Skipping NetworkPolicy traffic-enforcement test; set %s=true to run on a policy-enforcing CNI "+
				"(OVN-Kubernetes / Calico). This is the strategy's 'validated assumption' and does not hold on kindnet.",
				npEnforcementEnvVar)
		}
		runTrafficEnforcementTest(t, server)
	})
}

func waitForNetworkPolicy(t *testing.T, namespace, name string) *networkingv1.NetworkPolicy {
	t.Helper()
	np := &networkingv1.NetworkPolicy{}
	err := wait.PollUntilContextTimeout(TestEnv.Ctx, generalRetryInterval, ResourceReadyTimeout, true,
		func(ctx context.Context) (bool, error) {
			if getErr := TestEnv.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, np); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					return false, nil
				}
				return false, getErr
			}
			return true, nil
		})
	require.NoError(t, err, "NetworkPolicy %s/%s should be created", namespace, name)
	return np
}

func hasPraxisPeer(np *networkingv1.NetworkPolicy) bool {
	for _, rule := range np.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.PodSelector != nil && peer.PodSelector.MatchLabels[praxisLabelKey] == praxisLabelValue {
				return true
			}
		}
	}
	return false
}

func hasOperatorNamespacePeer(np *networkingv1.NetworkPolicy, operatorNS string) bool {
	for _, rule := range np.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.NamespaceSelector != nil &&
				peer.NamespaceSelector.MatchLabels["kubernetes.io/metadata.name"] == operatorNS {
				return true
			}
		}
	}
	return false
}

func hasSameNamespacePeer(np *networkingv1.NetworkPolicy) bool {
	for _, rule := range np.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.PodSelector != nil && len(peer.PodSelector.MatchLabels) == 0 &&
				len(peer.PodSelector.MatchExpressions) == 0 && peer.NamespaceSelector == nil {
				return true
			}
		}
	}
	return false
}

func hasRouterPeer(np *networkingv1.NetworkPolicy) bool {
	for _, rule := range np.Spec.Ingress {
		for _, peer := range rule.From {
			if peer.NamespaceSelector != nil &&
				peer.NamespaceSelector.MatchLabels["network.openshift.io/policy-group"] == "ingress" {
				return true
			}
		}
	}
	return false
}

// runTrafficEnforcementTest verifies that, on a policy-enforcing CNI, a pod without the Praxis
// label is blocked from reaching OGX on 8321 while a pod carrying the label succeeds. It waits
// for the OGX deployment to be ready first so the Service has endpoints.
func runTrafficEnforcementTest(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	require.NoError(t, WaitForPodsReady(t, TestEnv, server.Namespace, server.Name, ResourceReadyTimeout),
		"OGX pods must be ready before probing connectivity")

	serviceHost := server.Name + "-service." + server.Namespace + ".svc.cluster.local"

	allowedExit := runConnectivityProbe(t, "np-probe-allowed", server.Namespace, serviceHost,
		map[string]string{praxisLabelKey: praxisLabelValue})
	require.Equal(t, int32(0), allowedExit, "a pod labeled %s=%s should reach OGX on 8321", praxisLabelKey, praxisLabelValue)

	blockedExit := runConnectivityProbe(t, "np-probe-blocked", server.Namespace, serviceHost, nil)
	require.NotEqual(t, int32(0), blockedExit, "a pod without the Praxis label must be blocked from OGX on 8321")
}

// runConnectivityProbe creates a short-lived pod that attempts a TCP connection to the OGX
// service port and returns its container exit code (0 = connected, non-zero = blocked/failed).
func runConnectivityProbe(t *testing.T, name, namespace, serviceHost string, labels map[string]string) int32 {
	t.Helper()

	pod := &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace, Labels: labels},
		Spec: corev1.PodSpec{
			RestartPolicy: corev1.RestartPolicyNever,
			Containers: []corev1.Container{
				{
					Name:    "probe",
					Image:   "busybox:1.36",
					Command: []string{"sh", "-c", "nc -z -w 5 " + serviceHost + " 8321"},
				},
			},
		},
	}

	_ = TestEnv.Client.Delete(TestEnv.Ctx, pod)
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, pod))
	t.Cleanup(func() { _ = TestEnv.Client.Delete(context.Background(), pod) })

	var exitCode int32 = -1
	err := wait.PollUntilContextTimeout(TestEnv.Ctx, generalRetryInterval, 3*time.Minute, true,
		func(ctx context.Context) (bool, error) {
			fetched := &corev1.Pod{}
			if getErr := TestEnv.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, fetched); getErr != nil {
				if apierrors.IsNotFound(getErr) {
					return false, nil
				}
				return false, getErr
			}
			for _, cs := range fetched.Status.ContainerStatuses {
				if cs.State.Terminated != nil {
					exitCode = cs.State.Terminated.ExitCode
					return true, nil
				}
			}
			return false, nil
		})
	require.NoError(t, err, "connectivity probe pod %s did not terminate in time", name)
	t.Logf("connectivity probe %s exited with code %d", name, exitCode)
	return exitCode
}
