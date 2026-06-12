//nolint:testpackage
package e2e

import (
	"context"
	"os"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v2"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// runCreationTestsForDistribution runs creation tests for a specific distribution type.
func runCreationTestsForDistribution(t *testing.T, distType string) *ogxiov1beta1.OGXServer {
	t.Helper()
	if TestOpts.SkipCreation {
		t.Skip("Skipping creation test suite")
	}

	var ogxServer *ogxiov1beta1.OGXServer

	t.Run("should create OGXServer", func(t *testing.T) {
		ogxServer = testCreateServerForType(t, distType)
	})

	requireServer := func(t *testing.T) {
		t.Helper()
		if ogxServer == nil {
			t.Skip("Skipping: OGXServer creation failed")
		}
	}

	t.Run("should create PVC if storage is configured", func(t *testing.T) {
		requireServer(t)
		testPVCConfiguration(t, ogxServer)
	})

	t.Run("should handle direct deployment updates", func(t *testing.T) {
		requireServer(t)
		testDirectDeploymentUpdates(t, ogxServer)
	})

	t.Run("should check health status", func(t *testing.T) {
		requireServer(t)
		testHealthStatus(t, ogxServer)
	})

	t.Run("should update deployment through CR", func(t *testing.T) {
		requireServer(t)
		testCRDeploymentUpdate(t, ogxServer)
	})

	t.Run("should update distribution status", func(t *testing.T) {
		requireServer(t)
		testDistributionStatus(t, ogxServer)
	})

	t.Run("should use custom ServiceAccount from workload overrides", func(t *testing.T) {
		requireServer(t)
		testServiceAccountOverride(t, ogxServer)
	})

	t.Run("should apply image mapping overrides from ConfigMap", func(t *testing.T) {
		requireServer(t)
		testImageMappingOverrides(t, ogxServer)
	})

	return ogxServer
}

func testCreateServerForType(t *testing.T, distType string) *ogxiov1beta1.OGXServer {
	t.Helper()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ogx-test",
		},
	}
	err := TestEnv.Client.Create(TestEnv.Ctx, ns)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	ogxServer := GetSampleCRForDistribution(t, distType)
	ogxServer.Namespace = ns.Name

	EnsureOverrideConfigMap(t, TestEnv.Client, TestEnv.Ctx, ogxServer)

	t.Logf("Creating %s distribution with name: %s", distType, ogxServer.Name)

	err = TestEnv.Client.Create(TestEnv.Ctx, ogxServer)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	err = EnsureResourceReady(t, TestEnv, schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, ogxServer.Name, ns.Name, ResourceReadyTimeout, isDeploymentReady)
	require.NoError(t, err)

	err = WaitForPodsReady(t, TestEnv, ns.Name, ogxServer.Name, ResourceReadyTimeout)
	require.NoError(t, err, "Pods should be running and ready")

	err = EnsureResourceReady(t, TestEnv, schema.GroupVersionKind{
		Group:   "",
		Version: "v1",
		Kind:    "Service",
	}, ogxServer.Name+"-service", ns.Name, ResourceReadyTimeout, func(u *unstructured.Unstructured) bool {
		spec, specFound, _ := unstructured.NestedMap(u.Object, "spec")
		status, statusFound, _ := unstructured.NestedMap(u.Object, "status")
		return specFound && statusFound && spec != nil && status != nil
	})
	requireNoErrorWithDebugging(t, TestEnv, err, "Service readiness check failed", ogxServer.Namespace, ogxServer.Name)

	return ogxServer
}

func testDirectDeploymentUpdates(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	if server.Spec.Workload != nil && server.Spec.Workload.Autoscaling != nil && server.Spec.Workload.Autoscaling.MaxReplicas > 0 {
		t.Skip("Skipping direct deployment update healing test when autoscaling is enabled")
	}

	deployment := &appsv1.Deployment{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: server.Namespace,
		Name:      server.Name,
	}, deployment)
	require.NoError(t, err)

	originalReplicas := *deployment.Spec.Replicas
	*deployment.Spec.Replicas = 2
	err = TestEnv.Client.Update(TestEnv.Ctx, deployment)
	require.NoError(t, err)

	time.Sleep(5 * time.Second)

	err = TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: server.Namespace,
		Name:      server.Name,
	}, deployment)
	require.NoError(t, err)
	require.Equal(t, originalReplicas, *deployment.Spec.Replicas, "Deployment should be reverted to original state")
}

func updateCRReplicas(server *ogxiov1beta1.OGXServer, replicas int32) error {
	return wait.PollUntilContextTimeout(TestEnv.Ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		latest := &ogxiov1beta1.OGXServer{}
		if err := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, latest); err != nil {
			return false, err
		}
		if latest.Spec.Workload == nil {
			latest.Spec.Workload = &ogxiov1beta1.WorkloadSpec{}
		}
		latest.Spec.Workload.Replicas = &replicas
		if err := TestEnv.Client.Update(ctx, latest); err != nil {
			if k8serrors.IsConflict(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
}

func testCRDeploymentUpdate(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	if server.Spec.Workload != nil && server.Spec.Workload.Autoscaling != nil && server.Spec.Workload.Autoscaling.MaxReplicas > 0 {
		t.Skip("Skipping CR deployment update test when autoscaling is enabled")
	}

	// Scale to 0 via CR update
	require.NoError(t, updateCRReplicas(server, 0), "Failed to update CR replicas to 0")

	// Verify deployment scales to 0
	err := EnsureResourceReady(t, TestEnv, schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, server.Name, server.Namespace, ResourceReadyTimeout, func(u *unstructured.Unstructured) bool {
		specReplicas, found, nestedErr := unstructured.NestedInt64(u.Object, "spec", "replicas")
		if !found || nestedErr != nil {
			return false
		}
		return specReplicas == 0
	})
	require.NoError(t, err, "Deployment should scale to 0")

	// Scale back to 1
	require.NoError(t, updateCRReplicas(server, 1), "Failed to scale CR back to 1")

	err = WaitForPodsReady(t, TestEnv, server.Namespace, server.Name, ResourceReadyTimeout)
	require.NoError(t, err, "Pod should be ready after scale-up")
}

func testHealthStatus(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	err := wait.PollUntilContextTimeout(TestEnv.Ctx, 1*time.Minute, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		updated := &ogxiov1beta1.OGXServer{}
		err := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, updated)
		if err != nil {
			return false, err
		}
		return updated.Status.Phase == ogxiov1beta1.OGXServerPhaseReady, nil
	})
	requireNoErrorWithDebugging(t, TestEnv, err, "Failed to wait for server status update", server.Namespace, server.Name)
}

func testDistributionStatus(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	err := wait.PollUntilContextTimeout(TestEnv.Ctx, 1*time.Minute, 5*time.Minute, true, func(ctx context.Context) (bool, error) {
		updated := &ogxiov1beta1.OGXServer{}
		err := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, updated)
		if err != nil {
			return false, err
		}

		return isOGXServerStatusReady(updated), nil
	})
	if err != nil {
		finalServer := &ogxiov1beta1.OGXServer{}
		TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, finalServer)
		requireNoErrorWithDebugging(t, TestEnv, err, "Failed to wait for distribution status update", server.Namespace, server.Name)
	}

	updated := &ogxiov1beta1.OGXServer{}
	err = TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: server.Namespace,
		Name:      server.Name,
	}, updated)
	require.NoError(t, err)

	if len(updated.Status.DistributionConfig.AvailableDistributions) > 0 {
		require.NotEmpty(t, updated.Status.DistributionConfig.AvailableDistributions,
			"Available distributions should be populated")
	}

	if updated.Status.DistributionConfig.ActiveDistribution != "" {
		require.Equal(t, updated.Spec.Distribution.Name,
			updated.Status.DistributionConfig.ActiveDistribution,
			"Active distribution should match the spec")
	}

	if len(updated.Status.DistributionConfig.Providers) > 0 {
		validateProviders(t, updated)
	} else {
		t.Log("No providers found in distribution status - this might be expected for some distributions")
	}

	yamlData, err := yaml.Marshal(updated)
	if err != nil {
		t.Fatalf("Failed to marshal server: %v", err)
	}
	err = os.WriteFile("../../distribution.log", yamlData, 0644)
	require.NoError(t, err)
}

func testPVCConfiguration(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	pvcName := server.Name + "-pvc"
	pvc := &corev1.PersistentVolumeClaim{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: server.Namespace,
		Name:      pvcName,
	}, pvc)

	hasStorage := server.Spec.Workload != nil && server.Spec.Workload.Storage != nil
	if !hasStorage {
		require.Error(t, err, "PVC should not exist when storage is not configured")
		require.True(t, k8serrors.IsNotFound(err), "Expected not found error for PVC when storage is not configured")
	} else {
		require.NoError(t, err, "PVC should be created when storage is configured")
		expectedSize := ogxiov1beta1.DefaultStorageSize
		if server.Spec.Workload.Storage.Size != nil {
			expectedSize = *server.Spec.Workload.Storage.Size
		}
		actualSize := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
		require.Equal(t, expectedSize.String(), actualSize.String(), "PVC storage size should match CR")
	}
}

func testServiceAccountOverride(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	sa := &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-sa",
			Namespace: server.Namespace,
		},
	}
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, sa))

	err := wait.PollUntilContextTimeout(TestEnv.Ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		latest := &ogxiov1beta1.OGXServer{}
		if err := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, latest); err != nil {
			return false, err
		}

		if latest.Spec.Workload == nil {
			latest.Spec.Workload = &ogxiov1beta1.WorkloadSpec{}
		}
		if latest.Spec.Workload.Overrides == nil {
			latest.Spec.Workload.Overrides = &ogxiov1beta1.WorkloadOverrides{}
		}
		latest.Spec.Workload.Overrides.ServiceAccountName = "custom-sa"

		if err := TestEnv.Client.Update(ctx, latest); err != nil {
			if k8serrors.IsConflict(err) {
				return false, nil
			}
			return false, err
		}
		return true, nil
	})
	require.NoError(t, err, "Failed to update CR with ServiceAccount override")

	time.Sleep(5 * time.Second)

	deployment := &appsv1.Deployment{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx,
		client.ObjectKey{
			Name:      server.Name,
			Namespace: server.Namespace,
		},
		deployment))

	assert.Equal(t, "custom-sa", deployment.Spec.Template.Spec.ServiceAccountName)

	// Reset the ServiceAccountName before deleting the SA to prevent subsequent
	// reconciles from producing a deployment that references a deleted SA.
	err = wait.PollUntilContextTimeout(TestEnv.Ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		latest := &ogxiov1beta1.OGXServer{}
		if getErr := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, latest); getErr != nil {
			return false, getErr
		}
		latest.Spec.Workload.Overrides.ServiceAccountName = ""
		if updateErr := TestEnv.Client.Update(ctx, latest); updateErr != nil {
			if k8serrors.IsConflict(updateErr) {
				return false, nil
			}
			return false, updateErr
		}
		return true, nil
	})
	require.NoError(t, err, "Failed to reset ServiceAccount override")

	time.Sleep(5 * time.Second)
	require.NoError(t, TestEnv.Client.Delete(TestEnv.Ctx, sa))
}

func isDeploymentReady(u *unstructured.Unstructured) bool {
	replicas, found, err := unstructured.NestedInt64(u.Object, "status", "replicas")
	if !found || err != nil {
		return false
	}
	availableReplicas, found, err := unstructured.NestedInt64(u.Object, "status", "availableReplicas")
	return found && err == nil && availableReplicas == replicas
}

// isOGXServerStatusReady checks if the server status is ready.
func isOGXServerStatusReady(server *ogxiov1beta1.OGXServer) bool {
	if len(server.Status.DistributionConfig.AvailableDistributions) == 0 {
		if server.Status.Phase != ogxiov1beta1.OGXServerPhaseReady {
			return false
		}
	}

	if server.Status.DistributionConfig.ActiveDistribution == "" {
		if server.Status.Phase != ogxiov1beta1.OGXServerPhaseReady {
			return false
		}
	}

	return server.Status.Phase == ogxiov1beta1.OGXServerPhaseReady
}

// validateProviders validates all providers in the server status.
func validateProviders(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	for _, provider := range server.Status.DistributionConfig.Providers {
		require.NotEmpty(t, provider.API, "Provider should have API info")
		require.NotEmpty(t, provider.ProviderID, "Provider should have ProviderID info")
		require.NotEmpty(t, provider.ProviderType, "Provider should have ProviderType info")
		require.NotNil(t, provider.Config, "Provider should have config info")
		if provider.ProviderID == "starter" {
			require.Equal(t, "OK", provider.Health.Status, "Provider should have OK health status")
		}
		require.Contains(t, []string{"OK", "Error", "Not Implemented"}, provider.Health.Status, "Provider health status should be one of: OK, Error, Not Implemented")
		if provider.Health.Status != "OK" {
			require.NotEmpty(t, provider.Health.Message, "Provider should have health message")
		}
		require.NotEmpty(t, provider.Config, "Provider config should not be empty")
	}
}

func testImageMappingOverrides(t *testing.T, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	deployment := &appsv1.Deployment{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: server.Namespace,
		Name:      server.Name,
	}, deployment))
	originalImage := deployment.Spec.Template.Spec.Containers[0].Image

	operatorConfigMap := &corev1.ConfigMap{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: TestOpts.OperatorNS,
		Name:      "ogx-operator-config",
	}, operatorConfigMap))

	testOverrideImage := "quay.io/test/ogx-server:override-test"
	if operatorConfigMap.Data == nil {
		operatorConfigMap.Data = make(map[string]string)
	}
	operatorConfigMap.Data["image-overrides"] = server.Spec.Distribution.Name + ": " + testOverrideImage

	require.NoError(t, TestEnv.Client.Update(TestEnv.Ctx, operatorConfigMap),
		"Failed to update operator ConfigMap with image overrides")

	err := wait.PollUntilContextTimeout(TestEnv.Ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		getErr := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, deployment)
		if getErr != nil {
			return false, getErr
		}
		currentImage := deployment.Spec.Template.Spec.Containers[0].Image
		t.Logf("Current deployment image: %s (waiting for: %s)", currentImage, testOverrideImage)
		return currentImage == testOverrideImage, nil
	})
	requireNoErrorWithDebugging(t, TestEnv, err,
		"Deployment should be updated with override image from ConfigMap",
		server.Namespace, server.Name)

	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: server.Namespace,
		Name:      server.Name,
	}, deployment))
	assert.Equal(t, testOverrideImage, deployment.Spec.Template.Spec.Containers[0].Image,
		"Deployment should use image from ConfigMap override")

	updatedOverrideImage := "quay.io/test/ogx-server:override-test-v2"
	operatorConfigMap.Data["image-overrides"] = server.Spec.Distribution.Name + ": " + updatedOverrideImage
	require.NoError(t, TestEnv.Client.Update(TestEnv.Ctx, operatorConfigMap),
		"Failed to update operator ConfigMap with new image override")

	err = wait.PollUntilContextTimeout(TestEnv.Ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		getErr := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, deployment)
		if getErr != nil {
			return false, getErr
		}
		currentImage := deployment.Spec.Template.Spec.Containers[0].Image
		t.Logf("Current deployment image: %s (waiting for updated override: %s)", currentImage, updatedOverrideImage)
		return currentImage == updatedOverrideImage, nil
	})
	requireNoErrorWithDebugging(t, TestEnv, err,
		"Deployment should be updated with new override image from ConfigMap",
		server.Namespace, server.Name)

	delete(operatorConfigMap.Data, "image-overrides")
	require.NoError(t, TestEnv.Client.Update(TestEnv.Ctx, operatorConfigMap),
		"Failed to restore operator ConfigMap")

	err = wait.PollUntilContextTimeout(TestEnv.Ctx, 10*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		getErr := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: server.Namespace,
			Name:      server.Name,
		}, deployment)
		if getErr != nil {
			return false, getErr
		}
		currentImage := deployment.Spec.Template.Spec.Containers[0].Image
		t.Logf("Current deployment image: %s (waiting for original: %s)", currentImage, originalImage)
		return currentImage == originalImage, nil
	})
	requireNoErrorWithDebugging(t, TestEnv, err,
		"Deployment should revert to original image after removing override",
		server.Namespace, server.Name)
}
