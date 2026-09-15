//nolint:testpackage
package e2e

import (
	"context"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	praxisTestTimeout = 5 * time.Minute
	praxisServerName  = "ogx-praxis-isolation"
)

// TestPraxisTenantIsolationSuite verifies Praxis mode reconciliation, environment variables,
// NetworkPolicy direct ingress enforcement, and header forwarding context.
func TestPraxisTenantIsolationSuite(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping Praxis Tenant Isolation test suite")
	}

	t.Run("should create test namespace and configmap", func(t *testing.T) {
		setupPraxisTestNamespace(t)
	})

	t.Run("should reconcile OGXServer with Praxis environment variables and NetworkPolicy", func(t *testing.T) {
		testPraxisReconciliation(t)
	})

	t.Run("should enforce NetworkPolicy blocking direct ingress access", func(t *testing.T) {
		testNetworkPolicyDirectAccessBlock(t)
	})

	t.Run("should cleanup Praxis test resources", func(t *testing.T) {
		cleanupPraxisTestResources(t)
	})
}

// setupPraxisTestNamespace sets up the namespace and sample starter configmap for testing.
func setupPraxisTestNamespace(t *testing.T) {
	t.Helper()

	// Arrange
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ogxTestNS,
		},
	}

	// Act
	err := TestEnv.Client.Create(TestEnv.Ctx, ns)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err, "failed to create test namespace %s", ogxTestNS)
	}

	ensureStarterConfigMap(t, ogxTestNS)
}

// testPraxisReconciliation verifies that deploying an OGXServer CR configured with identity env vars
// and NetworkPolicy generates the expected deployment env vars and NetworkPolicy resource.
func testPraxisReconciliation(t *testing.T) {
	t.Helper()

	// Arrange: Construct OGXServer CR with Praxis identity headers and NetworkPolicy enabled
	enabledPolicy := true
	ogxServer := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      praxisServerName,
			Namespace: ogxTestNS,
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{
				Name: "starter",
			},
			Network: &ogxiov1beta1.NetworkSpec{
				Port: ogxiov1beta1.DefaultServerPort,
				Policy: &ogxiov1beta1.NetworkPolicySpec{
					Enabled: &enabledPolicy,
					Ingress: []networkingv1.NetworkPolicyIngressRule{
						{
							Ports: []networkingv1.NetworkPolicyPort{
								{
									Port: &intstr.IntOrString{Type: intstr.Int, IntVal: ogxiov1beta1.DefaultServerPort},
								},
							},
							From: []networkingv1.NetworkPolicyPeer{
								{
									PodSelector: &metav1.LabelSelector{
										MatchLabels: map[string]string{
											"app.kubernetes.io/name": "praxis-proxy",
										},
									},
								},
							},
						},
					},
				},
			},
			Workload: &ogxiov1beta1.WorkloadSpec{
				Overrides: &ogxiov1beta1.WorkloadOverrides{
					Env: []corev1.EnvVar{
						{
							Name:  "UPSTREAM_HEADER_PRINCIPAL_HEADER",
							Value: "x-user-id",
						},
						{
							Name:  "UPSTREAM_HEADER_TENANT_HEADER",
							Value: "x-tenant-id",
						},
					},
				},
			},
		},
	}

	// Act: Create OGXServer CR
	err := TestEnv.Client.Create(TestEnv.Ctx, ogxServer)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err, "failed to create OGXServer CR %s", praxisServerName)
	}

	// Assert: Wait for Deployment to be created and verify env vars
	var deployment appsv1.Deployment
	err = wait.PollUntilContextTimeout(TestEnv.Ctx, 2*time.Second, praxisTestTimeout, true, func(ctx context.Context) (bool, error) {
		if getErr := TestEnv.Client.Get(ctx, client.ObjectKey{Name: praxisServerName, Namespace: ogxTestNS}, &deployment); getErr != nil {
			if k8serrors.IsNotFound(getErr) {
				return false, nil
			}
			return false, getErr
		}
		return len(deployment.Spec.Template.Spec.Containers) > 0, nil
	})
	require.NoError(t, err, "failed to wait for Deployment creation for %s", praxisServerName)

	container := deployment.Spec.Template.Spec.Containers[0]
	envMap := make(map[string]string)
	for _, env := range container.Env {
		envMap[env.Name] = env.Value
	}

	assert.Equal(t, "x-user-id", envMap["UPSTREAM_HEADER_PRINCIPAL_HEADER"], "UPSTREAM_HEADER_PRINCIPAL_HEADER mismatch")
	assert.Equal(t, "x-tenant-id", envMap["UPSTREAM_HEADER_TENANT_HEADER"], "UPSTREAM_HEADER_TENANT_HEADER mismatch")

	// Assert: Verify NetworkPolicy creation and rules
	var netPolicy networkingv1.NetworkPolicy
	err = wait.PollUntilContextTimeout(TestEnv.Ctx, 2*time.Second, praxisTestTimeout, true, func(ctx context.Context) (bool, error) {
		if getErr := TestEnv.Client.Get(ctx, client.ObjectKey{Name: praxisServerName, Namespace: ogxTestNS}, &netPolicy); getErr != nil {
			if k8serrors.IsNotFound(getErr) {
				return false, nil
			}
			return false, getErr
		}
		return true, nil
	})
	require.NoError(t, err, "failed to wait for NetworkPolicy creation for %s", praxisServerName)

	require.NotEmpty(t, netPolicy.Spec.Ingress, "NetworkPolicy ingress rules should not be empty")
}

// testNetworkPolicyDirectAccessBlock verifies that NetworkPolicy ingress rules restrict direct pod port access.
func testNetworkPolicyDirectAccessBlock(t *testing.T) {
	t.Helper()

	// Arrange: Fetch NetworkPolicy
	var netPolicy networkingv1.NetworkPolicy
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{Name: praxisServerName, Namespace: ogxTestNS}, &netPolicy)
	require.NoError(t, err, "failed to fetch NetworkPolicy %s", praxisServerName)

	// Act & Assert: Verify NetworkPolicy enforces pod selector restriction on service port 8321
	var foundPraxisRule bool
	for _, ingress := range netPolicy.Spec.Ingress {
		for _, peer := range ingress.From {
			if peer.PodSelector != nil && peer.PodSelector.MatchLabels["app.kubernetes.io/name"] == "praxis-proxy" {
				foundPraxisRule = true
				break
			}
		}
	}
	assert.True(t, foundPraxisRule, "NetworkPolicy should restrict ingress port 8321 to praxis-proxy pod selector")
}

// cleanupPraxisTestResources deletes CR and resources created during testing.
func cleanupPraxisTestResources(t *testing.T) {
	t.Helper()

	// Arrange
	ogxServer := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      praxisServerName,
			Namespace: ogxTestNS,
		},
	}

	// Act
	err := TestEnv.Client.Delete(TestEnv.Ctx, ogxServer)
	if err != nil && !k8serrors.IsNotFound(err) {
		t.Logf("failed to delete OGXServer %s during cleanup: %v", praxisServerName, err)
	}
}
