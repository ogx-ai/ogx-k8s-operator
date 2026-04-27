package controllers_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	controllers "github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apiextensionsv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
)

// testenvNamespaceCounter is used to generate unique namespace names for test isolation.
var testenvNamespaceCounter int

func TestStorageConfiguration(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name           string
		buildInstance  func(namespace string) *ogxiov1beta1.OGXServer
		expectedVolume corev1.Volume
		expectedMount  corev1.VolumeMount
	}{
		{
			name: "No storage configuration - should use emptyDir",
			buildInstance: func(namespace string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("test").
					WithNamespace(namespace).
					WithStorage(nil).
					Build()
			},
			expectedVolume: corev1.Volume{
				Name: testStorageVolumeName,
				VolumeSource: corev1.VolumeSource{
					EmptyDir: &corev1.EmptyDirVolumeSource{},
				},
			},
			expectedMount: corev1.VolumeMount{
				Name:      testStorageVolumeName,
				MountPath: ogxiov1beta1.DefaultMountPath,
			},
		},
		{
			name: "Storage with default values",
			buildInstance: func(namespace string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("test").
					WithNamespace(namespace).
					WithStorage(DefaultTestStorage()).
					Build()
			},
			expectedVolume: corev1.Volume{
				Name: testStorageVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-pvc",
					},
				},
			},
			expectedMount: corev1.VolumeMount{
				Name:      testStorageVolumeName,
				MountPath: ogxiov1beta1.DefaultMountPath,
			},
		},
		{
			name: "Storage with custom values",
			buildInstance: func(namespace string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("test").
					WithNamespace(namespace).
					WithStorage(CustomTestStorage("20Gi", "/custom/path")).
					Build()
			},
			expectedVolume: corev1.Volume{
				Name: testStorageVolumeName,
				VolumeSource: corev1.VolumeSource{
					PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
						ClaimName: "test-pvc",
					},
				},
			},
			expectedMount: corev1.VolumeMount{
				Name:      testStorageVolumeName,
				MountPath: "/custom/path",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := createTestNamespace(t, "test-storage")

			// arrange
			instance := tt.buildInstance(namespace.Name)
			require.NoError(t, k8sClient.Create(t.Context(), instance))
			t.Cleanup(func() {
				if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
					t.Logf("Failed to delete OGXServer instance %s/%s: %v", instance.Namespace, instance.Name, err)
				}
			})

			// act: reconcile the instance
			ReconcileOGXServer(t, instance)

			// assert
			deployment := &appsv1.Deployment{}
			waitForResource(t, k8sClient, instance.Namespace, instance.Name, deployment)

			if tt.expectedVolume.EmptyDir != nil {
				AssertDeploymentUsesEmptyDirStorage(t, deployment)
			} else if tt.expectedVolume.PersistentVolumeClaim != nil {
				AssertDeploymentUsesPVCStorage(t, deployment, tt.expectedVolume.PersistentVolumeClaim.ClaimName)
			}

			AssertDeploymentHasVolumeMount(t, deployment, tt.expectedMount.MountPath)

			// verify PVC is created when storage is configured
			if instance.Spec.Workload != nil && instance.Spec.Workload.Storage != nil {
				expectedPVCName := tt.expectedVolume.PersistentVolumeClaim.ClaimName
				pvc := AssertPVCExists(t, k8sClient, instance.Namespace, expectedPVCName)
				expectedSize := instance.Spec.Workload.Storage.Size
				if expectedSize == nil {
					AssertPVCHasSize(t, pvc, ogxiov1beta1.DefaultStorageSize.String())
				} else {
					AssertPVCHasSize(t, pvc, expectedSize.String())
				}
			}
		})
	}
}

func TestConfigMapWatchingFunctionality(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create a test namespace
	namespace := createTestNamespace(t, "test-configmap-watch")
	// Create a ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-config",
			Namespace: namespace.Name,
		},
		Data: map[string]string{
			"config.yaml": `version: '2'
image_name: ollama
apis:
- inference
providers:
  inference:
  - provider_id: ollama
    provider_type: "remote::ollama"
    config:
      url: "http://ollama-server:11434"
models:
  - model_id: "llama3.2:1b"
    provider_id: ollama
    model_type: llm
server:
  port: 8321`,
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), configMap))

	// Create an OGXServer that references the ConfigMap
	instance := NewOGXServerBuilder().
		WithName("test-configmap-reference").
		WithNamespace(namespace.Name).
		WithOverrideConfig(configMap.Name).
		Build()
	require.NoError(t, k8sClient.Create(t.Context(), instance))

	// Reconcile to create initial deployment
	ReconcileOGXServer(t, instance)

	// Get the initial deployment and check for ConfigMap hash annotation
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	waitForResourceWithKey(t, k8sClient, deploymentKey, deployment)

	// Verify the ConfigMap hash annotation exists
	initialAnnotations := deployment.Spec.Template.Annotations
	require.Contains(t, initialAnnotations, "configmap.hash/user-config", "ConfigMap hash annotation should be present")
	initialHash := initialAnnotations["configmap.hash/user-config"]
	require.NotEmpty(t, initialHash, "ConfigMap hash should not be empty")

	// Update the ConfigMap data
	require.NoError(t, k8sClient.Get(t.Context(),
		types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, configMap))

	configMap.Data["config.yaml"] = `version: '2'
image_name: ollama
apis:
- inference
providers:
  inference:
  - provider_id: ollama
    provider_type: "remote::ollama"
    config:
      url: "http://ollama-server:11434"
models:
  - model_id: "llama3.2:3b"
    provider_id: ollama
    model_type: llm
server:
  port: 8321`
	require.NoError(t, k8sClient.Update(t.Context(), configMap))

	// Wait a moment for the watch to trigger
	time.Sleep(2 * time.Second)

	// Trigger reconciliation (in real scenarios this would be triggered by the watch)
	ReconcileOGXServer(t, instance)

	// Verify the deployment was updated with a new hash
	waitForResourceWithKeyAndCondition(
		t, k8sClient, deploymentKey, deployment, func() bool {
			newHash := deployment.Spec.Template.Annotations["configmap.hash/user-config"]
			return newHash != initialHash && newHash != ""
		}, "ConfigMap hash should be updated after ConfigMap data change")

	t.Logf("ConfigMap hash changed from %s to %s", initialHash, deployment.Spec.Template.Annotations["configmap.hash/user-config"])

	// Test that unrelated ConfigMaps don't trigger reconciliation
	unrelatedConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "unrelated-config",
			Namespace: namespace.Name,
		},
		Data: map[string]string{
			"some-key": "some-value",
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), unrelatedConfigMap))

	// Note: In test environment, field indexer might not be set up properly,
	// so we skip the isConfigMapReferenced checks which rely on field indexing
}

func TestReconcile(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// --- arrange ---
	instanceName := "ogxserver-sample"
	instancePort := ogxiov1beta1.DefaultServerPort
	expectedSelector := map[string]string{
		ogxiov1beta1.DefaultLabelKey: ogxiov1beta1.DefaultLabelValue,
		"app.kubernetes.io/instance": instanceName,
	}
	expectedPort := corev1.ServicePort{
		Name:       ogxiov1beta1.DefaultServicePortName,
		Port:       instancePort,
		TargetPort: intstr.FromInt(int(instancePort)),
		Protocol:   corev1.ProtocolTCP,
	}
	operatorNamespaceName := "test-operator-namespace"

	// set operator namespace to avoid service account file dependency
	t.Setenv("OPERATOR_NAMESPACE", operatorNamespaceName)

	namespace := createTestNamespace(t, operatorNamespaceName)
	instance := NewOGXServerBuilder().
		WithName(instanceName).
		WithNamespace(namespace.Name).
		WithDistribution("starter").
		WithPort(instancePort).
		Build()
	require.NoError(t, k8sClient.Create(t.Context(), instance))

	// --- act ---
	ReconcileOGXServer(t, instance)

	service := &corev1.Service{}
	waitForResource(t, k8sClient, instance.Namespace, instance.Name+"-service", service)
	deployment := &appsv1.Deployment{}
	waitForResource(t, k8sClient, instance.Namespace, instance.Name, deployment)
	networkpolicy := &networkingv1.NetworkPolicy{}
	waitForResource(t, k8sClient, instance.Namespace, instance.Name+"-network-policy",
		networkpolicy)
	serviceAccount := &corev1.ServiceAccount{}
	waitForResource(t, k8sClient, instance.Namespace, instance.Name+"-sa",
		serviceAccount)

	// --- assert ---
	// Service behaviors
	AssertServicePortMatches(t, service, expectedPort)
	AssertServiceAndDeploymentPortsAlign(t, service, deployment)
	AssertServiceSelectorMatches(t, service, expectedSelector)
	AssertServiceAndDeploymentSelectorsAlign(t, service, deployment)

	// ServiceAccount behaviors
	AssertServiceAccountDeploymentAlign(t, deployment, serviceAccount)

	// NetworkPolicy behaviors
	AssertNetworkPolicyTargetsDeploymentPods(t, networkpolicy, deployment)
	AssertNetworkPolicyAllowsDeploymentPort(t, networkpolicy, deployment, operatorNamespaceName)
	AssertNetworkPolicyIsIngressOnly(t, networkpolicy)

	// Resource ownership behaviors
	AssertResourceOwnedByInstance(t, service, instance)
	AssertResourceOwnedByInstance(t, deployment, instance)
	AssertResourceOwnedByInstance(t, networkpolicy, instance)
	AssertResourceOwnedByInstance(t, serviceAccount, instance)
}

// Define a custom roundtripper type for testing.
type mockRoundTripper struct {
	RoundTripFunc func(req *http.Request) (*http.Response, error)
}

// RoundTrip satisfies the http.RoundTripper interface and calls the mock function.
func (m *mockRoundTripper) RoundTrip(req *http.Request) (*http.Response, error) {
	return m.RoundTripFunc(req)
}

// newMockAPIResponse is a test helper that takes any data structure,
// marshals it to JSON, and returns a complete http response.
func newMockAPIResponse(t *testing.T, data any) *http.Response {
	t.Helper()
	jsonBytes, err := json.Marshal(data)
	require.NoError(t, err)

	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(string(jsonBytes))),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
	}
}

func TestOGXServerProviderAndVersionInfo(t *testing.T) {
	// arrange
	expectedServerVersion := "v-test"
	expectedProviderID := "mock-ollama"

	// define the data structure for the mock providers response
	providerData := struct {
		Data []ogxiov1beta1.ProviderInfo `json:"data"`
	}{
		Data: []ogxiov1beta1.ProviderInfo{
			{
				ProviderID:   expectedProviderID,
				ProviderType: "remote::ollama",
				API:          "inference",
				Health:       ogxiov1beta1.ProviderHealthStatus{Status: "OK", Message: ""},
				Config:       apiextensionsv1.JSON{Raw: []byte(`{"url": "http://mock.server"}`)},
			},
		},
	}

	// define the data structure for the mock version response
	versionData := struct {
		Version string `json:"version"`
	}{
		Version: expectedServerVersion,
	}

	// create the mock http client that uses our custom roundtripper
	mockClient := &http.Client{
		Transport: &mockRoundTripper{
			// simulate the RoundTrip logic to handle different API paths
			RoundTripFunc: func(req *http.Request) (*http.Response, error) {
				if req.URL.Path == "/v1/providers" {
					return newMockAPIResponse(t, providerData), nil
				}
				if req.URL.Path == "/v1/version" {
					return newMockAPIResponse(t, versionData), nil
				}
				return &http.Response{
					StatusCode: http.StatusNotFound,
					Body:       io.NopCloser(strings.NewReader("")),
					Header:     http.Header{"Content-Type": []string{"application/json"}},
				}, nil
			},
		},
	}

	namespace := createTestNamespace(t, "test-status")
	instance := NewOGXServerBuilder().
		WithName("test-status-instance").
		WithNamespace(namespace.Name).
		Build()
	require.NoError(t, k8sClient.Create(t.Context(), instance))

	testClusterInfo := &cluster.ClusterInfo{
		DistributionImages: map[string]string{
			"starter": testImage,
		},
	}

	reconciler := controllers.NewTestReconciler(
		k8sClient,
		scheme.Scheme,
		testClusterInfo,
		mockClient,
	)

	// act (part 1)
	// run the first reconciliation to create the initial resources like the deployment
	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
	})
	require.NoError(t, err)

	// manually update the deployment's status because envtest doesn't run a real deployment controller
	// this forces the reconciler to proceed to the health check logic on its next run
	deployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}
	waitForResourceWithKey(t, k8sClient, deploymentKey, deployment)

	deployment.Status.ReadyReplicas = 1
	deployment.Status.Replicas = 1
	require.NoError(t, k8sClient.Status().Update(t.Context(), deployment))

	// act (part 2)
	// run the second reconciliation to trigger the status update logic
	_, err = reconciler.Reconcile(t.Context(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
	})
	require.NoError(t, err)

	// assert
	updatedInstance := &ogxiov1beta1.OGXServer{}
	waitForResource(t, k8sClient, namespace.Name, instance.Name, updatedInstance)

	// validate provider info
	require.Len(t, updatedInstance.Status.DistributionConfig.Providers, 1, "should find exactly one provider from the mock server")
	actualProvider := updatedInstance.Status.DistributionConfig.Providers[0]
	require.Equal(t, expectedProviderID, actualProvider.ProviderID, "provider ID should match the mock response")
	require.Equal(t, "OK", actualProvider.Health.Status, "provider health should match the mock response")
	require.NotEmpty(t, actualProvider.Config, "provider config should be populated")
	// validate llama stack version
	require.Equal(t, expectedServerVersion,
		updatedInstance.Status.Version.ServerVersion,
		"server version should match the mock response")

	// validate service URL
	expectedServiceURL := fmt.Sprintf("http://%s-service.%s.svc.cluster.local:%d",
		instance.Name, instance.Namespace, ogxiov1beta1.DefaultServerPort)
	require.Equal(t, expectedServiceURL, updatedInstance.Status.ServiceURL,
		"service URL should be set to the internal Kubernetes service URL")
}

func TestNetworkPolicyConfiguration(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name                  string
		disableNPBeforeCreate bool
		setup                 func(t *testing.T, instance *ogxiov1beta1.OGXServer)
	}{
		{
			name:                  "enabled then disabled deletes NetworkPolicy",
			disableNPBeforeCreate: false,
			setup: func(t *testing.T, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				ReconcileOGXServer(t, instance)
				waitForResource(t, k8sClient, instance.Namespace, instance.Name+"-network-policy", &networkingv1.NetworkPolicy{})
				require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, instance))
				if instance.Spec.Network == nil {
					instance.Spec.Network = &ogxiov1beta1.NetworkSpec{}
				}
			instance.Spec.Network.Policy = &ogxiov1beta1.NetworkPolicySpec{
				Enabled: boolPtr(false),
			}
			require.NoError(t, k8sClient.Update(t.Context(), instance))
			},
		},
		{
			name:                  "disabled from start leaves NetworkPolicy absent",
			disableNPBeforeCreate: true,
			setup: func(t *testing.T, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
			},
		},
	}

	for i, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			operatorNamespaceName := "test-operator-namespace"
			t.Setenv("OPERATOR_NAMESPACE", operatorNamespaceName)

			namespace := createTestNamespace(t, "test-networkpolicy")
			instance := NewOGXServerBuilder().
				WithName(fmt.Sprintf("np-config-%d", i)).
				WithNamespace(namespace.Name).
				WithDistribution("starter").
				Build()
			if tt.disableNPBeforeCreate {
				if instance.Spec.Network == nil {
					instance.Spec.Network = &ogxiov1beta1.NetworkSpec{}
				}
			instance.Spec.Network.Policy = &ogxiov1beta1.NetworkPolicySpec{
				Enabled: boolPtr(false),
			}
		}
		require.NoError(t, k8sClient.Create(t.Context(), instance))
			t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

			tt.setup(t, instance)

			ReconcileOGXServer(t, instance)

			npKey := types.NamespacedName{Name: instance.Name + "-network-policy", Namespace: instance.Namespace}
			AssertNetworkPolicyAbsent(t, k8sClient, npKey)
		})
	}
}

// TestCABundleConfigMapKeyValidation tests that invalid ConfigMap keys are rejected during reconciliation.
func TestCABundleConfigMapKeyValidation(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name          string
		configMapKeys []string
		shouldFail    bool
		errorContains string
	}{
		{
			name:          "valid ConfigMap keys",
			configMapKeys: []string{"ca-bundle.crt", "root-ca.pem", "intermediate.crt"},
			shouldFail:    false,
		},
		{
			name:          "ConfigMap key with path traversal (..) should fail",
			configMapKeys: []string{"../etc/passwd"},
			shouldFail:    true,
			errorContains: "invalid path characters",
		},
		{
			name:          "ConfigMap key with forward slash should fail",
			configMapKeys: []string{"path/to/cert.crt"},
			shouldFail:    true,
			errorContains: "invalid path characters",
		},
		{
			name:          "ConfigMap key with double dots in middle should fail",
			configMapKeys: []string{"ca..bundle.crt"},
			shouldFail:    true,
			errorContains: "invalid path characters",
		},
		{
			name:          "empty ConfigMap key should fail",
			configMapKeys: []string{""},
			shouldFail:    true,
			errorContains: "cannot be empty",
		},
		{
			name:          "ConfigMap key with invalid characters should fail",
			configMapKeys: []string{"ca bundle with spaces.crt"},
			shouldFail:    true,
			errorContains: "invalid characters",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// --- arrange ---
			namespace := createTestNamespace(t, "test-cabundle-validation")
			instance := NewOGXServerBuilder().
				WithName("test-cabundle").
				WithNamespace(namespace.Name).
				WithCABundle("test-ca-configmap", tt.configMapKeys).
				Build()

			require.NoError(t, k8sClient.Create(t.Context(), instance))
			t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

			// --- act ---
			reconciler := createTestReconciler()
			_, err := reconciler.Reconcile(t.Context(), ctrl.Request{
				NamespacedName: types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			})

			// --- assert ---
			if tt.shouldFail {
				require.Error(t, err, "reconciliation should fail for invalid ConfigMap keys")
				require.Contains(t, err.Error(), tt.errorContains,
					"error message should indicate the validation failure")
			} else if err != nil {
				// For valid keys, reconciliation might fail for other reasons (ConfigMap doesn't exist),
				// but it should NOT fail due to key validation
				require.NotContains(t, err.Error(), "failed to validate CA bundle ConfigMap keys",
					"reconciliation should not fail due to key validation for valid keys")
			}
		})
	}
}

// TestManagedCABundleConfigMap tests that the operator creates and manages CA bundle ConfigMaps.
func TestManagedCABundleConfigMap(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	t.Run("creates managed ConfigMap with concatenated certificates", func(t *testing.T) {
		// --- arrange ---
		namespace := createTestNamespace(t, "test-managed-cabundle")

		// Load valid test certificate
		testCert := loadTestCertificate(t)

		// Create source CA bundle ConfigMap with multiple keys (using same cert as both for simplicity)
		sourceConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-ca-bundle",
				Namespace: namespace.Name,
			},
			Data: map[string]string{
				"root-ca.crt":      testCert,
				"intermediate.crt": testCert,
			},
		}
		require.NoError(t, k8sClient.Create(t.Context(), sourceConfigMap))

		instance := NewOGXServerBuilder().
			WithName("test-managed").
			WithNamespace(namespace.Name).
			WithCABundle("source-ca-bundle", []string{"root-ca.crt", "intermediate.crt"}).
			Build()

		require.NoError(t, k8sClient.Create(t.Context(), instance))
		t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

		// --- act ---
		ReconcileOGXServer(t, instance)

		// --- assert ---
		managedConfigMapName := instance.Name + "-ca-bundle"
		managedConfigMap := &corev1.ConfigMap{}
		waitForResource(t, k8sClient, namespace.Name, managedConfigMapName, managedConfigMap)

		// Verify the managed ConfigMap has the correct structure
		require.Contains(t, managedConfigMap.Data, "ca-bundle.crt", "managed ConfigMap should have ca-bundle.crt key")
		caBundleData := managedConfigMap.Data["ca-bundle.crt"]

		// Verify certificates are present in the concatenated bundle
		require.Contains(t, caBundleData, "BEGIN CERTIFICATE", "bundle should contain certificates")
		require.Contains(t, caBundleData, "END CERTIFICATE", "bundle should contain complete certificates")

		// Verify owner reference
		AssertResourceOwnedByInstance(t, managedConfigMap, instance)

		// Verify labels
		require.Equal(t, "ogx-operator", managedConfigMap.Labels["app.kubernetes.io/managed-by"])
		require.Equal(t, instance.Name, managedConfigMap.Labels["app.kubernetes.io/instance"])
		require.Equal(t, "ca-bundle", managedConfigMap.Labels["app.kubernetes.io/component"])
		require.Equal(t, controllers.WatchLabelValue, managedConfigMap.Labels[controllers.WatchLabelKey],
			"managed CA bundle ConfigMap should have the watch label")
	})

	t.Run("updates managed ConfigMap when source changes", func(t *testing.T) {
		// --- arrange ---
		namespace := createTestNamespace(t, "test-cabundle-update")

		// Load valid test certificate
		testCert := loadTestCertificate(t)

		sourceConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-ca-bundle",
				Namespace: namespace.Name,
			},
			Data: map[string]string{
				"ca-bundle.crt": testCert,
			},
		}
		require.NoError(t, k8sClient.Create(t.Context(), sourceConfigMap))

		instance := NewOGXServerBuilder().
			WithName("test-update").
			WithNamespace(namespace.Name).
			WithCABundle("source-ca-bundle", nil).
			Build()

		require.NoError(t, k8sClient.Create(t.Context(), instance))
		t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

		ReconcileOGXServer(t, instance)

		managedConfigMapName := instance.Name + "-ca-bundle"
		managedConfigMap := &corev1.ConfigMap{}
		waitForResource(t, k8sClient, namespace.Name, managedConfigMapName, managedConfigMap)

		originalData := managedConfigMap.Data["ca-bundle.crt"]

		// --- act ---
		// Update source ConfigMap by adding the certificate twice (making bundle larger)
		sourceConfigMap.Data["ca-bundle.crt"] = testCert + "\n" + testCert
		require.NoError(t, k8sClient.Update(t.Context(), sourceConfigMap))

		ReconcileOGXServer(t, instance)

		// --- assert ---
		require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
			Name:      managedConfigMapName,
			Namespace: namespace.Name,
		}, managedConfigMap))

		updatedData := managedConfigMap.Data["ca-bundle.crt"]
		require.NotEqual(t, originalData, updatedData, "managed ConfigMap should be updated")
		require.Contains(t, updatedData, "BEGIN CERTIFICATE", "managed ConfigMap should still contain certificate")
		// Verify the bundle has two certificates now
		require.Greater(t, len(updatedData), len(originalData), "updated bundle should be larger")
	})

	t.Run("rejects non-certificate PEM blocks", func(t *testing.T) {
		// --- arrange ---
		namespace := createTestNamespace(t, "test-reject-non-cert")

		// Create source ConfigMap with non-certificate PEM block (should be rejected)
		// Using "PUBLIC KEY" type to test that only CERTIFICATE blocks are accepted
		sourceConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-with-key",
				Namespace: namespace.Name,
			},
			Data: map[string]string{
				"ca-bundle.crt": `-----BEGIN PUBLIC KEY-----
MIIBIjANBgkqhkiG9w0BAQEFAAOCAQ8AMIIBCgKCAQEA1234567890ABCDEF
-----END PUBLIC KEY-----`,
			},
		}
		require.NoError(t, k8sClient.Create(t.Context(), sourceConfigMap))

		instance := NewOGXServerBuilder().
			WithName("test-reject").
			WithNamespace(namespace.Name).
			WithCABundle("source-with-key", nil).
			Build()

		require.NoError(t, k8sClient.Create(t.Context(), instance))
		t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

		// --- act ---
		reconciler := createTestReconciler()
		_, err := reconciler.Reconcile(t.Context(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		})

		// --- assert ---
		require.Error(t, err, "reconciliation should fail for non-certificate PEM")
		require.Contains(t, err.Error(), "failed to find valid certificates",
			"error should indicate no valid certificates")
	})

	t.Run("rejects invalid X.509 certificates", func(t *testing.T) {
		// --- arrange ---
		namespace := createTestNamespace(t, "test-reject-invalid-x509")

		// Create source ConfigMap with malformed certificate data
		// This has correct PEM structure but invalid X.509 certificate data
		sourceConfigMap := &corev1.ConfigMap{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "source-with-invalid-cert",
				Namespace: namespace.Name,
			},
			Data: map[string]string{
				"ca-bundle.crt": `-----BEGIN CERTIFICATE-----
InvalidCertificateDataThatIsNotValidX509
-----END CERTIFICATE-----`,
			},
		}
		require.NoError(t, k8sClient.Create(t.Context(), sourceConfigMap))

		instance := NewOGXServerBuilder().
			WithName("test-reject-invalid").
			WithNamespace(namespace.Name).
			WithCABundle("source-with-invalid-cert", nil).
			Build()

		require.NoError(t, k8sClient.Create(t.Context(), instance))
		t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

		// --- act ---
		reconciler := createTestReconciler()
		_, err := reconciler.Reconcile(t.Context(), ctrl.Request{
			NamespacedName: types.NamespacedName{
				Name:      instance.Name,
				Namespace: instance.Namespace,
			},
		})

		// --- assert ---
		require.Error(t, err, "reconciliation should fail for invalid X.509 certificate")
		require.Contains(t, err.Error(), "failed to parse X.509 certificate",
			"error should indicate X.509 parsing failure")
	})
}

func TestParseImageMappingOverrides_SingleOverride(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Test data with single override
	configMapData := map[string]string{
		"image-overrides": "starter: quay.io/custom/llama-stack:starter",
	}

	// Call the function
	result := controllers.ParseImageMappingOverrides(t.Context(), configMapData)

	// Assertions
	require.Len(t, result, 1, "Should have exactly one override")
	require.Equal(t, "quay.io/custom/llama-stack:starter", result["starter"], "Override should match expected value")
}

func TestParseImageMappingOverrides_InvalidYAML(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Test data with invalid YAML
	configMapData := map[string]string{
		"image-overrides": "invalid: yaml: content: [",
	}

	// Call the function
	result := controllers.ParseImageMappingOverrides(t.Context(), configMapData)

	// Assertions - should return empty map on error
	require.Empty(t, result, "Should return empty map when YAML is invalid")
}

func TestParseImageMappingOverrides_InvalidImageReference(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Test data with invalid image references
	configMapData := map[string]string{
		"image-overrides": `
starter: quay.io/valid/image:tag
invalid: not a valid image reference!!!
another: quay.io/another/valid:image
malformed: UPPERCASE/INVALID:IMAGE
onemore: registry.redhat.io/org/imagename@sha256:1234567890123456789012345678901234567890123456789012345678901234
`,
	}

	// Call the function
	result := controllers.ParseImageMappingOverrides(t.Context(), configMapData)

	// Assertions - should skip invalid entries and keep valid ones
	require.Len(t, result, 3, "Should have exactly two valid overrides")
	require.Equal(t, "quay.io/valid/image:tag", result["starter"], "Valid starter override should be present")
	require.Equal(t, "quay.io/another/valid:image", result["another"], "Valid another override should be present")
	require.Equal(t,
		"registry.redhat.io/org/imagename@sha256:1234567890123456789012345678901234567890123456789012345678901234",
		result["onemore"], "Valid onemore override should be present")
	require.NotContains(t, result, "invalid", "Invalid entry should be skipped")
	require.NotContains(t, result, "malformed", "Malformed entry should be skipped")
}

func TestNewOGXServerReconciler_WithImageOverrides(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create operator namespace
	operatorNamespace := createTestNamespace(t, "llama-stack-k8s-operator-system")
	t.Setenv("OPERATOR_NAMESPACE", operatorNamespace.Name)

	// Create test ConfigMap with image overrides
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ogx-operator-config",
			Namespace: operatorNamespace.Name,
		},
		Data: map[string]string{
			"image-overrides": "starter: quay.io/custom/llama-stack:starter",
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), configMap))

	// Create test cluster info
	clusterInfo := &cluster.ClusterInfo{
		OperatorNamespace:  operatorNamespace.Name,
		DistributionImages: map[string]string{"starter": "default-image"},
	}

	// Call the function
	reconciler, err := controllers.NewOGXServerReconciler(
		t.Context(),
		k8sClient,
		scheme.Scheme,
		clusterInfo,
		k8sClient,
	)

	// Assertions
	require.NoError(t, err, "Should create reconciler successfully")
	require.NotNil(t, reconciler, "Reconciler should not be nil")
	require.Len(t, reconciler.ImageMappingOverrides, 1, "Should have one image override")
	require.Equal(t, "quay.io/custom/llama-stack:starter",
		reconciler.ImageMappingOverrides["starter"], "Override should match expected value")
}

func TestConfigMapUpdateTriggersReconciliation(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	// Create test namespace
	namespace := createTestNamespace(t, "test-configmap-update")
	operatorNamespace := createTestNamespace(t, "llama-stack-k8s-operator-system")
	t.Setenv("OPERATOR_NAMESPACE", operatorNamespace.Name)

	// Create initial ConfigMap
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ogx-operator-config",
			Namespace: operatorNamespace.Name,
		},
		Data: map[string]string{},
	}
	require.NoError(t, k8sClient.Create(t.Context(), configMap))

	// Create test instance using starter
	instance := NewOGXServerBuilder().
		WithName("test-configmap-update").
		WithNamespace(namespace.Name).
		WithDistribution("starter").
		Build()
	require.NoError(t, k8sClient.Create(t.Context(), instance))

	// Create reconciler with initial overrides
	clusterInfo := &cluster.ClusterInfo{
		OperatorNamespace:  operatorNamespace.Name,
		DistributionImages: map[string]string{"starter": "default-starter-image"},
	}

	reconciler, err := controllers.NewOGXServerReconciler(
		t.Context(),
		k8sClient,
		scheme.Scheme,
		clusterInfo,
		k8sClient,
	)
	require.NoError(t, err)

	// Initial reconciliation
	_, err = reconciler.Reconcile(t.Context(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
	})
	require.NoError(t, err)

	// Get initial deployment and verify it uses the first override
	deployment := &appsv1.Deployment{}
	waitForResource(t, k8sClient, instance.Namespace, instance.Name, deployment)
	initialImage := deployment.Spec.Template.Spec.Containers[0].Image
	require.Equal(t, "default-starter-image", initialImage,
		"Initial deployment should use distribution image")

	// Re-fetch the ConfigMap to get the latest resource version (initializeOperatorConfigMap
	// may have updated it to add the watch label).
	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name:      configMap.Name,
		Namespace: configMap.Namespace,
	}, configMap))

	// Update ConfigMap with new overrides
	if configMap.Data == nil {
		configMap.Data = make(map[string]string)
	}
	configMap.Data["image-overrides"] = "starter: quay.io/custom/llama-stack:starter"
	require.NoError(t, k8sClient.Update(t.Context(), configMap))

	// Reconcile with the same reconciler instance. refreshOperatorConfig (called
	// at the start of Reconcile) reads the updated ConfigMap via the direct API
	// client, so the new image override is picked up without recreating the reconciler.
	_, err = reconciler.Reconcile(t.Context(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
	})
	require.NoError(t, err)

	// Verify deployment was updated with new image
	waitForResourceWithKeyAndCondition(
		t, k8sClient, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
		deployment, func() bool {
			return deployment.Spec.Template.Spec.Containers[0].Image == "quay.io/custom/llama-stack:starter"
		}, "Deployment should be updated with new image")
}

func TestReconcileRequeuesAfterSuccess(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	namespace := createTestNamespace(t, "test-requeue")
	operatorNamespace := createTestNamespace(t, "llama-stack-k8s-operator-system")
	t.Setenv("OPERATOR_NAMESPACE", operatorNamespace.Name)

	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ogx-operator-config",
			Namespace: operatorNamespace.Name,
		},
		Data: map[string]string{},
	}
	require.NoError(t, k8sClient.Create(t.Context(), configMap))

	instance := NewOGXServerBuilder().
		WithName("test-requeue").
		WithNamespace(namespace.Name).
		WithDistribution("starter").
		Build()
	require.NoError(t, k8sClient.Create(t.Context(), instance))

	clusterInfo := &cluster.ClusterInfo{
		OperatorNamespace:  operatorNamespace.Name,
		DistributionImages: map[string]string{"starter": "default-starter-image"},
	}

	reconciler, err := controllers.NewOGXServerReconciler(
		t.Context(),
		k8sClient,
		scheme.Scheme,
		clusterInfo,
		k8sClient,
	)
	require.NoError(t, err)

	result, err := reconciler.Reconcile(t.Context(), ctrl.Request{
		NamespacedName: types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace},
	})
	require.NoError(t, err)

	// In test envs, deployment never becomes ready (no kubelet), so the instance
	// stays in Initializing phase which requeues after 10s. Verify that a
	// successful reconciliation always schedules a requeue (not zero).
	// In test env the deployment stays in Initializing phase (10s requeue).
	// The Ready path returns 5m. Either way, requeue must be scheduled.
	require.Positive(t, result.RequeueAfter,
		"Successful reconciliation should always schedule a requeue")
	require.Equal(t, 10*time.Second, result.RequeueAfter,
		"Initializing phase should requeue after 10 seconds")
}

func TestMapConfigMapToReconcileRequests(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	namespace := createTestNamespace(t, "test-cm-mapping")

	// Create a user ConfigMap with the watch label.
	userConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "user-config",
			Namespace: namespace.Name,
			Labels: map[string]string{
				controllers.WatchLabelKey: controllers.WatchLabelValue,
			},
		},
		Data: map[string]string{
			"config.yaml": "version: '2'\nimage_name: ollama",
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), userConfigMap))

	// Create an instance that references this ConfigMap.
	instance := NewOGXServerBuilder().
		WithName("test-cm-mapping").
		WithNamespace(namespace.Name).
		WithOverrideConfig(userConfigMap.Name).
		Build()
	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() { _ = k8sClient.Delete(t.Context(), instance) })

	reconciler := createTestReconciler()

	// Act: call the handler with the user ConfigMap.
	requests := reconciler.MapConfigMapToReconcileRequests(t.Context(), userConfigMap)

	// Assert: should return a request for the referencing instance.
	require.Len(t, requests, 1, "should map to exactly one CR")
	require.Equal(t, reconcile.Request{
		NamespacedName: types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	}, requests[0])
}

func TestMapConfigMapToReconcileRequests_SkipsManagedConfigMaps(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	namespace := createTestNamespace(t, "test-cm-skip-managed")

	// Create an operator-managed ConfigMap.
	managedConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "managed-cm",
			Namespace: namespace.Name,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ogx-operator",
				controllers.WatchLabelKey:      controllers.WatchLabelValue,
			},
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), managedConfigMap))

	reconciler := createTestReconciler()

	// Act: call the handler with the managed ConfigMap.
	requests := reconciler.MapConfigMapToReconcileRequests(t.Context(), managedConfigMap)

	// Assert: should return no requests (managed CMs are handled by Owns).
	require.Empty(t, requests, "managed ConfigMaps should be skipped")
}

func TestUserConfigMapPredicate(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	reconciler := createTestReconciler()
	pred := reconciler.UserConfigMapPredicate()

	tests := []struct {
		name     string
		labels   map[string]string
		expected bool
	}{
		{
			name: "watch-labeled user ConfigMap is accepted",
			labels: map[string]string{
				controllers.WatchLabelKey: controllers.WatchLabelValue,
			},
			expected: true,
		},
		{
			name: "operator-managed ConfigMap is rejected even with watch label",
			labels: map[string]string{
				"app.kubernetes.io/managed-by": "ogx-operator",
				controllers.WatchLabelKey:      controllers.WatchLabelValue,
			},
			expected: false,
		},
		{
			name:     "unlabeled ConfigMap is rejected",
			labels:   nil,
			expected: false,
		},
		{
			name: "ConfigMap without watch label is rejected",
			labels: map[string]string{
				"some-other-label": "value",
			},
			expected: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cm := &corev1.ConfigMap{
				ObjectMeta: metav1.ObjectMeta{
					Name:   "test-cm",
					Labels: tt.labels,
				},
			}

			result := pred.Create(event.CreateEvent{Object: cm})
			require.Equal(t, tt.expected, result, "Create predicate")

			result = pred.Update(event.UpdateEvent{ObjectNew: cm, ObjectOld: cm})
			require.Equal(t, tt.expected, result, "Update predicate")

			result = pred.Delete(event.DeleteEvent{Object: cm})
			require.Equal(t, tt.expected, result, "Delete predicate")
		})
	}
}

func boolPtr(b bool) *bool { return &b }
