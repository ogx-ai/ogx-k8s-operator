package controllers

import (
	"fmt"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func buildControllerTestScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))
	return scheme
}

func TestReconcileConfigMaps_ClearsStaleGeneratedStatusWhenInactive(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &OGXServerReconciler{
		Client:       k8sClient,
		DirectClient: k8sClient,
		Scheme:       scheme,
	}

	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
			// No declarative config fields -> generation should be inactive.
		},
		Status: ogxiov1beta1.OGXServerStatus{
			ConfigGeneration: &ogxiov1beta1.ConfigGenerationStatus{
				ConfigMapName:      "example-config-deadbeef",
				ObservedGeneration: 2,
			},
		},
	}

	err := r.reconcileConfigMaps(t.Context(), instance)
	require.NoError(t, err)
	assert.Nil(t, instance.Status.ConfigGeneration)

	condition := GetCondition(&instance.Status, ConditionTypeConfigGenerated)
	require.NotNil(t, condition)
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, "ConfigGenerationInactive", condition.Reason)
	assert.Equal(t, "Declarative config generation is not active", condition.Message)
}

func TestReconcileGeneratedConfig_HappyPath(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &OGXServerReconciler{
		Client:         k8sClient,
		DirectClient:   k8sClient,
		Scheme:         scheme,
		configResolver: config.NewDefaultConfigResolver(nil),
		ClusterInfo: &cluster.ClusterInfo{
			DistributionImages: map[string]string{
				"starter": "docker.io/llamastack/distribution-starter:latest",
			},
		},
	}

	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-ns",
			UID:       types.UID("test-uid-123"),
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
			Providers: &ogxiov1beta1.ProvidersSpec{
				Inference: &ogxiov1beta1.InferenceProvidersSpec{
					Remote: &ogxiov1beta1.InferenceRemoteProviders{
						VLLM: []ogxiov1beta1.VLLMProvider{
							{Endpoint: "https://vllm:8000"},
						},
					},
				},
			},
		},
	}

	generated, err := r.reconcileGeneratedConfig(t.Context(), instance)
	require.NoError(t, err)
	require.NotNil(t, generated, "expected generated config for declarative providers")
	assert.Positive(t, generated.ProviderCount)
	assert.Equal(t, 2, generated.ConfigVersion)
	assert.NotEmpty(t, generated.ConfigYAML)
	assert.NotEmpty(t, generated.ContentHash)

	cmName := fmt.Sprintf("test-server-config-%s", generated.ContentHash)
	cm := &corev1.ConfigMap{}
	err = k8sClient.Get(t.Context(), client.ObjectKey{Name: cmName, Namespace: "test-ns"}, cm)
	require.NoError(t, err, "expected generated ConfigMap to exist")
	assert.Equal(t, generated.ConfigYAML, cm.Data["config.yaml"])
	assert.Equal(t, "true", cm.Labels[generatedConfigLabel])
	assert.Equal(t, managedByLabelVal, cm.Labels[managedByLabelKey])
}

func TestReconcileGeneratedConfig_SkippedWhenOverrideSet(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &OGXServerReconciler{
		Client:         k8sClient,
		DirectClient:   k8sClient,
		Scheme:         scheme,
		configResolver: config.NewDefaultConfigResolver(nil),
		ClusterInfo: &cluster.ClusterInfo{
			DistributionImages: map[string]string{
				"starter": "docker.io/llamastack/distribution-starter:latest",
			},
		},
	}

	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-ns",
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
			OverrideConfig: &ogxiov1beta1.ConfigMapKeyRef{
				Name: "my-config",
				Key:  "config.yaml",
			},
			Providers: &ogxiov1beta1.ProvidersSpec{
				Inference: &ogxiov1beta1.InferenceProvidersSpec{
					Remote: &ogxiov1beta1.InferenceRemoteProviders{
						VLLM: []ogxiov1beta1.VLLMProvider{
							{Endpoint: "https://vllm:8000"},
						},
					},
				},
			},
		},
	}

	generated, err := r.reconcileGeneratedConfig(t.Context(), instance)
	require.NoError(t, err)
	assert.Nil(t, generated, "expected nil when override config is set")
}

func TestReconcileConfigMaps_SetsInactiveConditionWithoutPriorStatus(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &OGXServerReconciler{
		Client:       k8sClient,
		DirectClient: k8sClient,
		Scheme:       scheme,
	}

	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "test-ns",
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
		},
	}

	err := r.reconcileConfigMaps(t.Context(), instance)
	require.NoError(t, err)
	assert.Nil(t, instance.Status.ConfigGeneration)
	condition := GetCondition(&instance.Status, ConditionTypeConfigGenerated)
	require.NotNil(t, condition)
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, "ConfigGenerationInactive", condition.Reason)
}
