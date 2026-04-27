package controllers

import (
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func int32Ptr(v int32) *int32 { return &v }

// createTestOGX builds a minimal OGXServer for tests (distribution by name and/or image).
func createTestOGX(name, image string) *ogxiov1beta1.OGXServer {
	return &ogxiov1beta1.OGXServer{
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: name, Image: image},
		},
	}
}

func setupTestClusterInfo(images map[string]string) *cluster.ClusterInfo {
	if images == nil {
		images = map[string]string{"ollama": "ollama-image:latest"}
	}
	return &cluster.ClusterInfo{
		OperatorNamespace:  "default",
		DistributionImages: images,
	}
}

func newDefaultStartupProbe(port int32) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler: corev1.ProbeHandler{
			HTTPGet: &corev1.HTTPGetAction{
				Path: "/v1/health",
				Port: intstr.FromInt(int(port)),
			},
		},
		InitialDelaySeconds: startupProbeInitialDelaySeconds,
		TimeoutSeconds:      startupProbeTimeoutSeconds,
		FailureThreshold:    startupProbeFailureThreshold,
		SuccessThreshold:    startupProbeSuccessThreshold,
	}
}

func TestBuildContainerSpec(t *testing.T) {
	t.Run("defaults", func(t *testing.T) {
		instance := &ogxiov1beta1.OGXServer{
			Spec: ogxiov1beta1.OGXServerSpec{
				Distribution: ogxiov1beta1.DistributionSpec{Image: "x:latest"},
			},
		}
		c := buildContainerSpec(t.Context(), nil, instance, "test-image:latest")
		assert.Equal(t, ogxiov1beta1.DefaultContainerName, c.Name)
		assert.Equal(t, "test-image:latest", c.Image)
		assert.Equal(t, ogxiov1beta1.DefaultServerPort, c.Ports[0].ContainerPort)
		assert.Equal(t, newDefaultStartupProbe(ogxiov1beta1.DefaultServerPort), c.StartupProbe)
		var foundOgxVol bool
		for _, m := range c.VolumeMounts {
			if m.Name == "ogx-storage" {
				foundOgxVol = true
				assert.Equal(t, ogxiov1beta1.DefaultMountPath, m.MountPath)
			}
		}
		assert.True(t, foundOgxVol, "expected ogx-storage volume mount")
	})

	t.Run("custom port and workload resources", func(t *testing.T) {
		instance := &ogxiov1beta1.OGXServer{
			Spec: ogxiov1beta1.OGXServerSpec{
				Distribution: ogxiov1beta1.DistributionSpec{Image: "x:latest"},
				Network:      &ogxiov1beta1.NetworkSpec{Port: 9000},
				Workload: &ogxiov1beta1.WorkloadSpec{
					Storage: &ogxiov1beta1.PVCStorageSpec{MountPath: "/custom"},
					Resources: &corev1.ResourceRequirements{
						Limits: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse("1"),
							corev1.ResourceMemory: resource.MustParse("2Gi"),
						},
					},
					Overrides: &ogxiov1beta1.WorkloadOverrides{
						Env: []corev1.EnvVar{{Name: "TEST_ENV", Value: "v"}},
					},
				},
			},
		}
		c := buildContainerSpec(t.Context(), nil, instance, "test-image:latest")
		assert.Equal(t, int32(9000), c.Ports[0].ContainerPort)
		assert.Equal(t, newDefaultStartupProbe(9000), c.StartupProbe)
		envNames := make([]string, 0, len(c.Env))
		for _, e := range c.Env {
			envNames = append(envNames, e.Name)
		}
		assert.Contains(t, envNames, "TEST_ENV")
	})
}

func TestResolveImage(t *testing.T) {
	clusterInfo := setupTestClusterInfo(map[string]string{"ollama": "ollama-image:latest"})
	cases := []struct {
		name      string
		instance  *ogxiov1beta1.OGXServer
		want      string
		expectErr bool
	}{
		{"by name", createTestOGX("ollama", ""), "ollama-image:latest", false},
		{"by image", createTestOGX("", "test-image:latest"), "test-image:latest", false},
		{"invalid name", createTestOGX("nope", ""), "", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &OGXServerReconciler{ClusterInfo: clusterInfo}
			img, err := r.resolveImage(tc.instance.Spec.Distribution)
			if tc.expectErr {
				require.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.Equal(t, tc.want, img)
		})
	}
}

func TestDistributionValidation(t *testing.T) {
	clusterInfo := setupTestClusterInfo(map[string]string{"ollama": "lls/lls-ollama:1.0"})
	cases := []struct {
		name      string
		instance  *ogxiov1beta1.OGXServer
		wantError bool
	}{
		{"valid name", createTestOGX("ollama", ""), false},
		{"valid image", createTestOGX("", "test:latest"), false},
		{"invalid name", createTestOGX("invalid", ""), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &OGXServerReconciler{ClusterInfo: clusterInfo}
			err := r.validateDistribution(tc.instance)
			if tc.wantError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDistributionWithoutClusterInfo(t *testing.T) {
	r := &OGXServerReconciler{ClusterInfo: nil}
	err := r.validateDistribution(createTestOGX("ollama", ""))
	require.Error(t, err)
}

func TestPodOverridesWithServiceAccount(t *testing.T) {
	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test-instance", Namespace: "ns"},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Image: "x:latest"},
			Workload: &ogxiov1beta1.WorkloadSpec{
				Overrides: &ogxiov1beta1.WorkloadOverrides{ServiceAccountName: "custom-sa"},
			},
		},
	}
	spec := &corev1.PodSpec{Containers: []corev1.Container{{Name: "c"}}}
	configurePodOverrides(instance, spec)
	assert.Equal(t, "custom-sa", spec.ServiceAccountName)
}

func TestNeedsPodDisruptionBudget(t *testing.T) {
	tests := []struct {
		name     string
		instance *ogxiov1beta1.OGXServer
		want     bool
	}{
		{
			"single replica",
			&ogxiov1beta1.OGXServer{Spec: ogxiov1beta1.OGXServerSpec{
				Distribution: ogxiov1beta1.DistributionSpec{Image: "x"},
				Workload:     &ogxiov1beta1.WorkloadSpec{Replicas: int32Ptr(1)},
			}},
			false,
		},
		{
			"multiple replicas",
			&ogxiov1beta1.OGXServer{Spec: ogxiov1beta1.OGXServerSpec{
				Distribution: ogxiov1beta1.DistributionSpec{Image: "x"},
				Workload:     &ogxiov1beta1.WorkloadSpec{Replicas: int32Ptr(2)},
			}},
			true,
		},
		{
			"explicit pdb",
			&ogxiov1beta1.OGXServer{Spec: ogxiov1beta1.OGXServerSpec{
				Distribution: ogxiov1beta1.DistributionSpec{Image: "x"},
				Workload: &ogxiov1beta1.WorkloadSpec{
					Replicas:            int32Ptr(1),
					PodDisruptionBudget: &ogxiov1beta1.PodDisruptionBudgetSpec{},
				},
			}},
			true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.want, needsPodDisruptionBudget(tt.instance))
		})
	}
}

func TestBuildPodDisruptionBudgetSpec(t *testing.T) {
	t.Run("defaults when replicas > 1", func(t *testing.T) {
		inst := &ogxiov1beta1.OGXServer{
			Spec: ogxiov1beta1.OGXServerSpec{
				Distribution: ogxiov1beta1.DistributionSpec{Image: "x"},
				Workload:     &ogxiov1beta1.WorkloadSpec{Replicas: int32Ptr(2)},
			},
		}
		spec := buildPodDisruptionBudgetSpec(inst)
		require.NotNil(t, spec)
		require.NotNil(t, spec.MaxUnavailable)
		assert.Equal(t, 1, spec.MaxUnavailable.IntValue())
	})
}

func TestBuildHPASpec(t *testing.T) {
	cpuT := int32(70)
	memT := int32(60)
	minR := int32(3)
	inst := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{Name: "sample"},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Image: "x"},
			Workload: &ogxiov1beta1.WorkloadSpec{
				Replicas: int32Ptr(2),
				Autoscaling: &ogxiov1beta1.AutoscalingSpec{
					MinReplicas:                       &minR,
					MaxReplicas:                       5,
					TargetCPUUtilizationPercentage:    &cpuT,
					TargetMemoryUtilizationPercentage: &memT,
				},
			},
		},
	}
	spec := buildHPASpec(inst)
	require.NotNil(t, spec)
	assert.Equal(t, int32(5), spec.MaxReplicas)
	require.Len(t, spec.Metrics, 2)
}
