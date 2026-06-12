package controllers

import (
	"encoding/base64"
	"fmt"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/config"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
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
	require.NoError(t, appsv1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))
	return scheme
}

func TestReconcileConfigMaps_DoesNotMutateStatusWhenInactive(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &OGXServerReconciler{
		Client: k8sClient,

		Scheme: scheme,
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

	generated, err := r.reconcileConfigMaps(t.Context(), instance)
	require.NoError(t, err)
	assert.Nil(t, generated)
	require.NotNil(t, instance.Status.ConfigGeneration)
	assert.Equal(t, "example-config-deadbeef", instance.Status.ConfigGeneration.ConfigMapName)
}

func TestReconcileGeneratedConfig_HappyPath(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	fetcher := func(string) (map[string]string, error) {
		return map[string]string{
			config.OCIDefaultConfigLabel:                "config.yaml",
			config.OCIConfigLabelPrefix + "config.yaml": base64.StdEncoding.EncodeToString([]byte("version: '2'\ndistro_name: starter\napis:\n- inference\nserver:\n  port: 8321\n")),
		}, nil
	}

	r := &OGXServerReconciler{
		Client: k8sClient,

		Scheme:         scheme,
		configResolver: config.NewDefaultConfigResolver(fetcher),
		ClusterInfo: &cluster.ClusterInfo{
			DistributionImages: map[string]string{
				"starter": "docker.io/ogxai/distribution-starter:latest",
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
		Client: k8sClient,

		Scheme:         scheme,
		configResolver: config.NewDefaultConfigResolver(nil),
		ClusterInfo: &cluster.ClusterInfo{
			DistributionImages: map[string]string{
				"starter": "docker.io/ogxai/distribution-starter:latest",
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

func TestReconcileGeneratedConfig_UsesBaseConfigMap(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	baseConfig := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "base-config",
			Namespace: "test-ns",
		},
		Data: map[string]string{
			"config.yaml": "version: '2'\ndistro_name: starter\napis:\n- inference\nserver:\n  port: 8321\n",
		},
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(baseConfig).Build()

	r := &OGXServerReconciler{
		Client: k8sClient,

		Scheme: scheme,
	}

	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-ns",
			UID:       types.UID("test-uid-456"),
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
			BaseConfig: &ogxiov1beta1.ConfigMapKeyRef{
				Name: "base-config",
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
	require.NotNil(t, generated)
	assert.Contains(t, generated.ConfigYAML, "distro_name: starter")
	assert.Contains(t, generated.ConfigYAML, "provider_id: remote-vllm")
}

func TestReconcileConfigMaps_LeavesStatusUnchangedWhenInactive(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &OGXServerReconciler{
		Client: k8sClient,

		Scheme: scheme,
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

	generated, err := r.reconcileConfigMaps(t.Context(), instance)
	require.NoError(t, err)
	assert.Nil(t, generated)
	assert.Nil(t, instance.Status.ConfigGeneration)
	assert.Nil(t, GetCondition(&instance.Status, ConditionTypeConfigGenerated))
}

func TestCleanupOldGeneratedConfigMaps_KeepsReferencedReplicaSetConfig(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	current := generatedConfigMapName("test-server", "current")
	referenced := generatedConfigMapName("test-server", "referenced")
	stale := generatedConfigMapName("test-server", "stale")

	cmCurrent := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              current,
			Namespace:         "test-ns",
			CreationTimestamp: metav1.NewTime(time.Unix(30, 0)),
			Labels: map[string]string{
				generatedConfigLabel:         "true",
				"app.kubernetes.io/instance": "test-server",
			},
		},
	}
	cmReferenced := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              referenced,
			Namespace:         "test-ns",
			CreationTimestamp: metav1.NewTime(time.Unix(20, 0)),
			Labels: map[string]string{
				generatedConfigLabel:         "true",
				"app.kubernetes.io/instance": "test-server",
			},
		},
	}
	cmStale := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:              stale,
			Namespace:         "test-ns",
			CreationTimestamp: metav1.NewTime(time.Unix(10, 0)),
			Labels: map[string]string{
				generatedConfigLabel:         "true",
				"app.kubernetes.io/instance": "test-server",
			},
		},
	}
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server",
			Namespace: "test-ns",
			Labels:    map[string]string{"app.kubernetes.io/instance": "test-server"},
		},
	}
	replicaSet := &appsv1.ReplicaSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-server-rs",
			Namespace: "test-ns",
			Labels:    map[string]string{"app.kubernetes.io/instance": "test-server"},
		},
		Spec: appsv1.ReplicaSetSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "user-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{Name: referenced},
								},
							},
						},
					},
				},
			},
		},
	}
	k8sClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(cmCurrent, cmReferenced, cmStale, deployment, replicaSet).
		Build()

	r := &OGXServerReconciler{
		Client: k8sClient,

		Scheme: scheme,
	}
	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test-server", Namespace: "test-ns"},
	}

	err := r.cleanupOldGeneratedConfigMaps(t.Context(), instance, current)
	require.NoError(t, err)

	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKey{Name: current, Namespace: "test-ns"}, &corev1.ConfigMap{}))
	require.NoError(t, k8sClient.Get(t.Context(), client.ObjectKey{Name: referenced, Namespace: "test-ns"}, &corev1.ConfigMap{}))
	err = k8sClient.Get(t.Context(), client.ObjectKey{Name: stale, Namespace: "test-ns"}, &corev1.ConfigMap{})
	require.Error(t, err)
}

func TestReconcileResources_DoesNotAdvanceConfigGenerationStatusOnSecretHashFailure(t *testing.T) {
	scheme := buildControllerTestScheme(t)
	fetcher := func(string) (map[string]string, error) {
		return map[string]string{
			config.OCIDefaultConfigLabel:                "config.yaml",
			config.OCIConfigLabelPrefix + "config.yaml": base64.StdEncoding.EncodeToString([]byte("version: '2'\ndistro_name: starter\napis:\n- inference\nserver:\n  port: 8321\n")),
		}, nil
	}
	k8sClient := fake.NewClientBuilder().WithScheme(scheme).Build()

	r := &OGXServerReconciler{
		Client: k8sClient,

		Scheme:         scheme,
		configResolver: config.NewDefaultConfigResolver(fetcher),
		ClusterInfo: &cluster.ClusterInfo{
			DistributionImages: map[string]string{
				"starter": "docker.io/ogxai/distribution-starter:latest",
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
			Providers: &ogxiov1beta1.ProvidersSpec{
				Inference: &ogxiov1beta1.InferenceProvidersSpec{
					Remote: &ogxiov1beta1.InferenceRemoteProviders{
						VLLM: []ogxiov1beta1.VLLMProvider{
							{
								Endpoint: "https://vllm:8000",
								APIToken: &ogxiov1beta1.SecretKeyRef{Name: "missing-secret", Key: "api-token"},
							},
						},
					},
				},
			},
			Resources: &ogxiov1beta1.ResourcesSpec{
				Models: []ogxiov1beta1.ModelConfig{{Name: "llama3"}},
			},
		},
		Status: ogxiov1beta1.OGXServerStatus{
			ConfigGeneration: &ogxiov1beta1.ConfigGenerationStatus{
				ConfigMapName:      "test-server-config-oldhash",
				ObservedGeneration: 1,
			},
		},
	}

	err := r.reconcileResources(t.Context(), instance)
	require.Error(t, err)
	require.NotNil(t, instance.Status.ConfigGeneration)
	assert.Equal(t, "test-server-config-oldhash", instance.Status.ConfigGeneration.ConfigMapName)

	condition := GetCondition(&instance.Status, ConditionTypeConfigGenerated)
	require.NotNil(t, condition)
	assert.Equal(t, metav1.ConditionFalse, condition.Status)
	assert.Equal(t, "ConfigGenerationFailed", condition.Reason)
}
