//nolint:testpackage
package e2e

import (
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// runDeletionTests runs deletion tests for a specific OGXServer.
func runDeletionTests(t *testing.T, instance *ogxiov1beta1.OGXServer) {
	t.Helper()

	t.Run("should delete OGXServer CR and cleanup resources", func(t *testing.T) {
		err := TestEnv.Client.Delete(TestEnv.Ctx, instance)
		require.NoError(t, err)

		err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
			Group:   "apps",
			Version: "v1",
			Kind:    "Deployment",
		}, instance.Name, instance.Namespace, ResourceReadyTimeout)
		require.NoError(t, err, "Deployment should be deleted")

		err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
			Group:   "",
			Version: "v1",
			Kind:    "Service",
		}, instance.Name+"-service", instance.Namespace, ResourceReadyTimeout)
		require.NoError(t, err, "Service should be deleted")

		err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
			Group:   "ogx.io",
			Version: "v1beta1",
			Kind:    "OGXServer",
		}, instance.Name, instance.Namespace, ResourceReadyTimeout)
		require.NoError(t, err, "CR should be deleted")

		podList := &corev1.PodList{}
		err = TestEnv.Client.List(TestEnv.Ctx, podList, client.InNamespace(instance.Namespace))
		require.NoError(t, err)
		for _, pod := range podList.Items {
			require.NotEqual(t, instance.Name, pod.Labels["app"], "Found orphaned pod")
		}

		configMapList := &corev1.ConfigMapList{}
		err = TestEnv.Client.List(TestEnv.Ctx, configMapList, client.InNamespace(instance.Namespace))
		require.NoError(t, err)
		for _, cm := range configMapList.Items {
			require.NotEqual(t, instance.Name, cm.Labels["app"], "Found orphaned configmap")
		}
	})
}
