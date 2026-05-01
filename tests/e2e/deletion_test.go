//nolint:testpackage
package e2e

import (
	"testing"

	"github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// runDeletionTests runs deletion tests for a specific distribution.
func runDeletionTests(t *testing.T, instance *v1alpha1.LlamaStackDistribution) {
	t.Helper()

	t.Run("should delete LlamaStackDistribution CR and cleanup resources", func(t *testing.T) {
		// Delete the instance
		err := TestEnv.Client.Delete(TestEnv.Ctx, instance)
		require.NoError(t, err)

		// Wait for deployment to be deleted
		err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		}, instance.Name, instance.Namespace, ResourceReadyTimeout)
		require.NoError(t, err, "Deployment should be deleted")

		// Wait for service to be deleted
		err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Service",
		}, instance.Name+"-service", instance.Namespace, ResourceReadyTimeout)
		require.NoError(t, err, "Service should be deleted")

		// Wait for CR to be deleted
		err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
			Group:   "llamastack.io",
			Version: "v1alpha1",
			Kind:    "LlamaStackDistribution",
		}, instance.Name, instance.Namespace, ResourceReadyTimeout)
		require.NoError(t, err, "CR should be deleted")

		// Verify no orphaned resources
		podList := &corev1.PodList{}
		err = TestEnv.Client.List(TestEnv.Ctx, podList, client.InNamespace(instance.Namespace))
		require.NoError(t, err)
		for _, pod := range podList.Items {
			require.NotEqual(t, instance.Name, pod.Labels["app"], "Found orphaned pod")
		}

		// Verify no orphaned configmaps
		configMapList := &corev1.ConfigMapList{}
		err = TestEnv.Client.List(TestEnv.Ctx, configMapList, client.InNamespace(instance.Namespace))
		require.NoError(t, err)
		for _, cm := range configMapList.Items {
			require.NotEqual(t, instance.Name, cm.Labels["app"], "Found orphaned configmap")
		}
	})
}
