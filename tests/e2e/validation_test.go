//nolint:testpackage
package e2e

import (
	"testing"

	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func TestValidationSuite(t *testing.T) {
	if TestOpts.SkipValidation {
		t.Skip("Skipping validation test suite")
	}

	t.Run("should validate CRDs", func(t *testing.T) {
		err := validateCRD(TestEnv.Client, TestEnv.Ctx, "ogxservers.ogx.io")
		require.NoErrorf(t, err, "error in validating CRD: ogxservers.ogx.io")
	})

	t.Run("should validate operator deployment", func(t *testing.T) {
		deployment, err := GetDeployment(TestEnv.Client, TestEnv.Ctx, "ogx-k8s-operator-controller-manager", TestOpts.OperatorNS)
		require.NoError(t, err, "Operator deployment not found")
		require.Equal(t, int32(1), deployment.Status.ReadyReplicas, "Operator deployment not ready")
	})

	t.Run("should validate operator pods", func(t *testing.T) {
		podList := &corev1.PodList{}
		err := TestEnv.Client.List(TestEnv.Ctx, podList, client.InNamespace(TestOpts.OperatorNS))
		require.NoError(t, err)

		operatorPodFound := false
		for _, pod := range podList.Items {
			if pod.Labels["app.kubernetes.io/name"] == "ogx-k8s-operator" {
				operatorPodFound = true
				require.Equal(t, corev1.PodRunning, pod.Status.Phase)
				break
			}
		}
		require.True(t, operatorPodFound, "Operator pod not found in namespace %s", TestOpts.OperatorNS)
	})

	t.Run("should validate prerequisites", func(t *testing.T) {
		deployment, err := GetDeployment(TestEnv.Client, TestEnv.Ctx, "ollama-server", ollamaNS)
		require.NoError(t, err, "Ollama deployment not found")
		require.Equal(t, int32(1), deployment.Status.ReadyReplicas, "Ollama deployment not ready")

		podList := &corev1.PodList{}
		err = TestEnv.Client.List(TestEnv.Ctx, podList, client.InNamespace(ollamaNS))
		require.NoError(t, err)
		require.NotEmpty(t, podList.Items, "No Ollama pods found")
		require.Equal(t, corev1.PodRunning, podList.Items[0].Status.Phase, "Ollama pod not running")
	})
}
