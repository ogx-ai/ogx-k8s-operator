/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"strings"
	"testing"

	llamav1alpha1 "github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
	"github.com/llamastack/llama-stack-k8s-operator/pkg/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

func int32Ptr(val int32) *int32 {
	return &val
}

func TestBuildContainerSpec(t *testing.T) {
	testCases := []struct {
		name           string
		instance       *llamav1alpha1.LlamaStackDistribution
		image          string
		expectedResult corev1.Container
	}{
		{
			name: "default values",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Server: llamav1alpha1.ServerSpec{
						ContainerSpec: llamav1alpha1.ContainerSpec{},
					},
				},
			},
			image: "test-image:latest",
			expectedResult: corev1.Container{
				Name:  llamav1alpha1.DefaultContainerName,
				Image: "test-image:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: llamav1alpha1.DefaultServerMemoryRequest,
					},
				},
				Ports:        []corev1.ContainerPort{{ContainerPort: llamav1alpha1.DefaultServerPort}},
				StartupProbe: newDefaultStartupProbe(llamav1alpha1.DefaultServerPort),
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "lls-storage",
					MountPath: llamav1alpha1.DefaultMountPath,
				}},
				Env: []corev1.EnvVar{
					{Name: "HF_HOME", Value: "/.llama"},
					{Name: "LLS_WORKERS", Value: "1"},
					{Name: "LLS_PORT", Value: "8321"},
					{Name: "LLAMA_STACK_CONFIG", Value: "/etc/llama-stack/config.yaml"},
				},
			},
		},
		{
			name: "custom container values",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Server: llamav1alpha1.ServerSpec{
						ContainerSpec: llamav1alpha1.ContainerSpec{
							Name: "custom-container",
							Port: 9000,
							Resources: corev1.ResourceRequirements{
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("1"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
							},
							Env: []corev1.EnvVar{
								{Name: "TEST_ENV", Value: "test-value"},
							},
							EnvFrom: []corev1.EnvFromSource{
								{SecretRef: &corev1.SecretEnvSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "TEST-SECRET",
									},
								}},
							},
						},
						Storage: &llamav1alpha1.StorageSpec{
							MountPath: "/custom/path",
						},
					},
				},
			},
			image: "test-image:latest",
			expectedResult: corev1.Container{
				Name:         "custom-container",
				Image:        "test-image:latest",
				Ports:        []corev1.ContainerPort{{ContainerPort: 9000}},
				StartupProbe: newDefaultStartupProbe(9000),
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: llamav1alpha1.DefaultServerMemoryRequest,
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: resource.MustParse("2Gi"),
					},
				},
				Env: []corev1.EnvVar{
					{Name: "HF_HOME", Value: "/custom/path"},
					{Name: "LLS_WORKERS", Value: "1"},
					{Name: "LLS_PORT", Value: "9000"},
					{Name: "LLAMA_STACK_CONFIG", Value: "/etc/llama-stack/config.yaml"},
					{Name: "TEST_ENV", Value: "test-value"},
				},
				EnvFrom: []corev1.EnvFromSource{
					{SecretRef: &corev1.SecretEnvSource{
						LocalObjectReference: corev1.LocalObjectReference{
							Name: "TEST-SECRET",
						},
					}},
				},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "lls-storage",
					MountPath: "/custom/path",
				}},
				Command: nil,
			},
		},
		{
			name: "command and args overrides",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Server: llamav1alpha1.ServerSpec{
						ContainerSpec: llamav1alpha1.ContainerSpec{
							Command: []string{"/custom/entrypoint.sh"},
							Args:    []string{"--config", "/etc/config.yaml", "--debug"},
						},
					},
				},
			},
			image: "test-image:latest",
			expectedResult: corev1.Container{
				Name:  llamav1alpha1.DefaultContainerName,
				Image: "test-image:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: llamav1alpha1.DefaultServerMemoryRequest,
					},
				},
				Command:      []string{"/custom/entrypoint.sh"},
				Args:         []string{"--config", "/etc/config.yaml", "--debug"},
				Ports:        []corev1.ContainerPort{{ContainerPort: llamav1alpha1.DefaultServerPort}},
				StartupProbe: newDefaultStartupProbe(llamav1alpha1.DefaultServerPort),
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "lls-storage",
					MountPath: llamav1alpha1.DefaultMountPath,
				}},
				Env: []corev1.EnvVar{
					{Name: "HF_HOME", Value: "/.llama"},
					{Name: "LLS_WORKERS", Value: "1"},
					{Name: "LLS_PORT", Value: "8321"},
					{Name: "LLAMA_STACK_CONFIG", Value: "/etc/llama-stack/config.yaml"},
				},
			},
		},
		{
			name: "uvicorn workers configured",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Server: llamav1alpha1.ServerSpec{
						Workers: int32Ptr(4),
					},
				},
			},
			image: "test-image:latest",
			expectedResult: corev1.Container{
				Name:  llamav1alpha1.DefaultContainerName,
				Image: "test-image:latest",
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: llamav1alpha1.DefaultServerMemoryRequest,
					},
					Limits: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("4"),
						corev1.ResourceMemory: llamav1alpha1.DefaultServerMemoryRequest,
					},
				},
				Ports:        []corev1.ContainerPort{{ContainerPort: llamav1alpha1.DefaultServerPort}},
				StartupProbe: newDefaultStartupProbe(llamav1alpha1.DefaultServerPort),
				Env: []corev1.EnvVar{
					{Name: "HF_HOME", Value: "/.llama"},
					{Name: "LLS_WORKERS", Value: "4"},
					{Name: "LLS_PORT", Value: "8321"},
					{Name: "LLAMA_STACK_CONFIG", Value: "/etc/llama-stack/config.yaml"},
				},
				VolumeMounts: []corev1.VolumeMount{{
					Name:      "lls-storage",
					MountPath: llamav1alpha1.DefaultMountPath,
				}},
			},
		},
		{
			name: "with user config",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Server: llamav1alpha1.ServerSpec{
						Distribution: llamav1alpha1.DistributionType{
							Name: "ollama",
						},
						ContainerSpec: llamav1alpha1.ContainerSpec{},
						UserConfig: &llamav1alpha1.UserConfigSpec{
							ConfigMapName: "test-config",
						},
					},
				},
			},
			image: "test-image:latest",
			expectedResult: corev1.Container{
				Name:            llamav1alpha1.DefaultContainerName,
				Image:           "test-image:latest",
				ImagePullPolicy: corev1.PullAlways,
				Resources: corev1.ResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceCPU:    resource.MustParse("1"),
						corev1.ResourceMemory: llamav1alpha1.DefaultServerMemoryRequest,
					},
				},
				Ports:        []corev1.ContainerPort{{ContainerPort: llamav1alpha1.DefaultServerPort}},
				StartupProbe: newDefaultStartupProbe(llamav1alpha1.DefaultServerPort),
				Command:      []string{"/bin/sh", "-c", startupScript},
				Args:         []string{},
				Env: []corev1.EnvVar{
					{Name: "HF_HOME", Value: llamav1alpha1.DefaultMountPath},
					{Name: "LLS_WORKERS", Value: "1"},
					{Name: "LLS_PORT", Value: "8321"},
					{Name: "LLAMA_STACK_CONFIG", Value: "/etc/llama-stack/config.yaml"},
				},
				VolumeMounts: []corev1.VolumeMount{
					{
						Name:      "lls-storage",
						MountPath: llamav1alpha1.DefaultMountPath,
					},
					{
						Name:      "user-config",
						MountPath: "/etc/llama-stack/",
						ReadOnly:  true,
					},
				},
			},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := buildContainerSpec(t.Context(), nil, tc.instance, tc.image)
			assert.Equal(t, tc.expectedResult.Name, result.Name)
			assert.Equal(t, tc.expectedResult.Image, result.Image)
			assert.Equal(t, tc.expectedResult.Ports, result.Ports)
			assert.Equal(t, tc.expectedResult.Resources, result.Resources)
			assert.Equal(t, tc.expectedResult.Env, result.Env)
			assert.Equal(t, tc.expectedResult.VolumeMounts, result.VolumeMounts)
			assert.Equal(t, tc.expectedResult.Command, result.Command)
			assert.Equal(t, tc.expectedResult.Args, result.Args)
			assert.Equal(t, tc.expectedResult.StartupProbe, result.StartupProbe)
		})
	}
}

func TestConfigurePodStorage(t *testing.T) {
	testCases := []struct {
		name              string
		instance          *llamav1alpha1.LlamaStackDistribution
		container         corev1.Container
		expectedPVCVolume bool
		expectedEmptyDir  bool
		expectedOverrides bool
	}{
		{
			name: "with PVC storage",
			instance: &llamav1alpha1.LlamaStackDistribution{
				ObjectMeta: metav1.ObjectMeta{
					Name: "test-instance",
				},
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Server: llamav1alpha1.ServerSpec{
						Storage: &llamav1alpha1.StorageSpec{},
					},
				},
			},
			container:         corev1.Container{Name: "test-container"},
			expectedPVCVolume: true,
			expectedEmptyDir:  false,
			expectedOverrides: false,
		},
		{
			name: "with EmptyDir storage",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Server: llamav1alpha1.ServerSpec{
						Storage: nil,
					},
				},
			},
			container:         corev1.Container{Name: "test-container"},
			expectedPVCVolume: false,
			expectedEmptyDir:  true,
			expectedOverrides: false,
		},
		{
			name: "with pod overrides",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Server: llamav1alpha1.ServerSpec{
						Storage: nil,
						PodOverrides: &llamav1alpha1.PodOverrides{
							Volumes: []corev1.Volume{
								{
									Name: "test-volume",
									VolumeSource: corev1.VolumeSource{
										ConfigMap: &corev1.ConfigMapVolumeSource{
											LocalObjectReference: corev1.LocalObjectReference{
												Name: "test-config",
											},
										},
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "test-volume",
									MountPath: "/test/path",
								},
							},
						},
					},
				},
			},
			container:         corev1.Container{Name: "test-container"},
			expectedPVCVolume: false,
			expectedEmptyDir:  true,
			expectedOverrides: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			result := configurePodStorage(t.Context(), nil, tc.instance, tc.container)

			// Verify container was added.
			assert.Len(t, result.Containers, 1)

			verifyStorageVolumes(t, result, tc.instance, tc.expectedPVCVolume, tc.expectedEmptyDir)

			if tc.expectedOverrides {
				verifyPodOverrides(t, result, tc.instance)
			}
		})
	}
}

// verifyStorageVolumes validates that the correct storage volumes are configured.
func verifyStorageVolumes(t *testing.T, podSpec corev1.PodSpec, instance *llamav1alpha1.LlamaStackDistribution,
	expectPVC, expectEmptyDir bool) {
	t.Helper()

	if expectPVC {
		pvcFound := false
		for _, vol := range podSpec.Volumes {
			if vol.Name == "lls-storage" && vol.PersistentVolumeClaim != nil {
				pvcFound = true
				assert.Equal(t, instance.Name+"-pvc", vol.PersistentVolumeClaim.ClaimName)
				break
			}
		}
		assert.True(t, pvcFound, "Expected PVC volume not found")
	}

	if expectEmptyDir {
		emptyDirFound := false
		for _, vol := range podSpec.Volumes {
			if vol.Name == "lls-storage" && vol.EmptyDir != nil {
				emptyDirFound = true
				break
			}
		}
		assert.True(t, emptyDirFound, "Expected EmptyDir volume not found")
	}
}

// verifyPodOverrides validates that pod overrides are correctly applied.
func verifyPodOverrides(t *testing.T, podSpec corev1.PodSpec, instance *llamav1alpha1.LlamaStackDistribution) {
	t.Helper()

	if instance.Spec.Server.PodOverrides == nil {
		return
	}

	verifyServiceAccount(t, podSpec, instance)
	verifyVolumes(t, podSpec, instance)
	verifyVolumeMounts(t, podSpec, instance)
}

func verifyServiceAccount(t *testing.T, podSpec corev1.PodSpec, instance *llamav1alpha1.LlamaStackDistribution) {
	t.Helper()
	if instance.Spec.Server.PodOverrides.ServiceAccountName != "" {
		assert.Equal(t, instance.Spec.Server.PodOverrides.ServiceAccountName, podSpec.ServiceAccountName)
	}
}

func verifyVolumes(t *testing.T, podSpec corev1.PodSpec, instance *llamav1alpha1.LlamaStackDistribution) {
	t.Helper()
	if len(instance.Spec.Server.PodOverrides.Volumes) > 0 {
		for _, volume := range instance.Spec.Server.PodOverrides.Volumes {
			found := false
			for _, podVolume := range podSpec.Volumes {
				if podVolume.Name == volume.Name {
					found = true
					break
				}
			}
			assert.True(t, found, "Volume %s not found in pod spec", volume.Name)
		}
	}
}

func verifyVolumeMounts(t *testing.T, podSpec corev1.PodSpec, instance *llamav1alpha1.LlamaStackDistribution) {
	t.Helper()
	if len(instance.Spec.Server.PodOverrides.VolumeMounts) > 0 {
		for _, container := range podSpec.Containers {
			for _, mount := range instance.Spec.Server.PodOverrides.VolumeMounts {
				found := false
				for _, podMount := range container.VolumeMounts {
					if podMount.Name == mount.Name {
						found = true
						break
					}
				}
				assert.True(t, found, "VolumeMount %s not found in container %s", mount.Name, container.Name)
			}
		}
	}
}

// createLSD creates a LlamaStackDistribution instance with optional name and image.
func createLSD(name, image string) *llamav1alpha1.LlamaStackDistribution {
	return &llamav1alpha1.LlamaStackDistribution{
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Server: llamav1alpha1.ServerSpec{
				Distribution: llamav1alpha1.DistributionType{
					Name:  name,
					Image: image,
				},
			},
		},
	}
}

// setupTestClusterInfo creates a ClusterInfo instance for testing with the specified distribution images.
// If no images are provided, it defaults to having "ollama" with "ollama-image:latest".
func setupTestClusterInfo(images map[string]string) *cluster.ClusterInfo {
	if images == nil {
		images = map[string]string{
			"ollama": "ollama-image:latest",
		}
	}
	return &cluster.ClusterInfo{
		OperatorNamespace:  "default",
		DistributionImages: images,
	}
}

func TestResolveImage(t *testing.T) {
	// Setup test cluster info
	clusterInfo := setupTestClusterInfo(map[string]string{
		"ollama": "ollama-image:latest",
	})

	testCases := []struct {
		name          string
		instance      *llamav1alpha1.LlamaStackDistribution
		expectedImage string
		expectError   bool
	}{
		{
			name:          "resolve from name",
			instance:      createLSD("ollama", ""),
			expectedImage: clusterInfo.DistributionImages["ollama"],
			expectError:   false,
		},
		{
			name:          "resolve from image",
			instance:      createLSD("", "test-image:latest"),
			expectedImage: "test-image:latest",
			expectError:   false,
		},
		{
			name:          "invalid distribution name",
			instance:      createLSD("invalid-name", ""),
			expectedImage: "",
			expectError:   true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &LlamaStackDistributionReconciler{ClusterInfo: clusterInfo}
			image, err := r.resolveImage(tc.instance.Spec.Server.Distribution)
			if tc.expectError {
				require.Error(t, err)
				assert.Empty(t, image)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tc.expectedImage, image)
			}
		})
	}
}

func TestDistributionValidation(t *testing.T) {
	// Setup test cluster info
	clusterInfo := setupTestClusterInfo(map[string]string{
		"ollama": "lls/lls-ollama:1.0",
	})

	testCases := []struct {
		name        string
		instance    *llamav1alpha1.LlamaStackDistribution
		expectError bool
	}{
		{
			name:        "valid distribution name",
			instance:    createLSD("ollama", ""),
			expectError: false,
		},
		{
			name:        "valid direct image",
			instance:    createLSD("", "test-image:latest"),
			expectError: false,
		},
		{
			name:        "invalid distribution name",
			instance:    createLSD("invalid-name", ""),
			expectError: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			r := &LlamaStackDistributionReconciler{ClusterInfo: clusterInfo}
			err := r.validateDistribution(tc.instance)
			if tc.expectError {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

func TestDistributionWithoutClusterInfo(t *testing.T) {
	// Clear cluster info
	instance := createLSD("ollama", "")
	r := &LlamaStackDistributionReconciler{ClusterInfo: nil}
	err := r.validateDistribution(instance)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "failed to initialize cluster info")
}

func TestPodOverridesWithServiceAccount(t *testing.T) {
	// Create a test instance with ServiceAccount override
	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-namespace",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Server: llamav1alpha1.ServerSpec{
				PodOverrides: &llamav1alpha1.PodOverrides{
					ServiceAccountName: "custom-sa",
				},
			},
		},
	}

	// Create deployment
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
						},
					},
				},
			},
		},
	}

	// Apply pod overrides
	configurePodOverrides(instance, &deployment.Spec.Template.Spec)

	// Verify ServiceAccount name
	if deployment.Spec.Template.Spec.ServiceAccountName != "custom-sa" {
		t.Errorf("expected ServiceAccountName to be 'custom-sa', got %s", deployment.Spec.Template.Spec.ServiceAccountName)
	}
}

func TestPodOverridesWithoutServiceAccount(t *testing.T) {
	// Create a test instance without ServiceAccount override
	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-namespace",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Server: llamav1alpha1.ServerSpec{
				PodOverrides: &llamav1alpha1.PodOverrides{},
			},
		},
	}

	// Create deployment
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
						},
					},
				},
			},
		},
	}

	// Apply pod overrides
	configurePodOverrides(instance, &deployment.Spec.Template.Spec)

	// Verify ServiceAccount name is empty (default ServiceAccountName should be set when not explicitly provided)
	if deployment.Spec.Template.Spec.ServiceAccountName != instance.Name+"-sa" {
		t.Errorf("expected default ServiceAccountName when not explicitly provided, got %s", deployment.Spec.Template.Spec.ServiceAccountName)
	}
}

func TestPodOverridesWithTerminationGracePeriod(t *testing.T) {
	gracePeriod := int64(120)
	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-namespace",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Server: llamav1alpha1.ServerSpec{
				PodOverrides: &llamav1alpha1.PodOverrides{
					TerminationGracePeriodSeconds: &gracePeriod,
				},
			},
		},
	}
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
						},
					},
				},
			},
		},
	}

	configurePodOverrides(instance, &deployment.Spec.Template.Spec)

	require.NotNil(t, deployment.Spec.Template.Spec.TerminationGracePeriodSeconds)
	assert.Equal(t, int64(120), *deployment.Spec.Template.Spec.TerminationGracePeriodSeconds)
}

func TestPodOverridesWithTerminationGracePeriodZero(t *testing.T) {
	// Ensures we distinguish "not set" (nil) from "set to 0" (immediate termination)
	gracePeriod := int64(0)
	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-namespace",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Server: llamav1alpha1.ServerSpec{
				PodOverrides: &llamav1alpha1.PodOverrides{
					TerminationGracePeriodSeconds: &gracePeriod,
				},
			},
		},
	}
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
						},
					},
				},
			},
		},
	}

	configurePodOverrides(instance, &deployment.Spec.Template.Spec)

	require.NotNil(t, deployment.Spec.Template.Spec.TerminationGracePeriodSeconds)
	assert.Equal(t, int64(0), *deployment.Spec.Template.Spec.TerminationGracePeriodSeconds)
}

func TestPodOverridesWithoutTerminationGracePeriod(t *testing.T) {
	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-instance",
			Namespace: "test-namespace",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Server: llamav1alpha1.ServerSpec{
				PodOverrides: &llamav1alpha1.PodOverrides{},
			},
		},
	}
	deployment := &appsv1.Deployment{
		Spec: appsv1.DeploymentSpec{
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name: "test-container",
						},
					},
				},
			},
		},
	}

	configurePodOverrides(instance, &deployment.Spec.Template.Spec)

	assert.Nil(t, deployment.Spec.Template.Spec.TerminationGracePeriodSeconds)
}

func TestValidateConfigMapKeys(t *testing.T) {
	tests := []struct {
		name        string
		keys        []string
		expectError bool
		errorMsg    string
	}{
		{
			name:        "valid keys",
			keys:        []string{DefaultCABundleKey, "intermediate.pem", "root-ca.cert"},
			expectError: false,
		},
		{
			name:        "empty key",
			keys:        []string{""},
			expectError: true,
			errorMsg:    "ConfigMap key cannot be empty",
		},
		{
			name:        "command injection attempt",
			keys:        []string{"valid-key; rm -rf /; echo malicious"},
			expectError: true,
			errorMsg:    "contains invalid path characters",
		},
		{
			name:        "path traversal attempt",
			keys:        []string{"../../../etc/passwd"},
			expectError: true,
			errorMsg:    "contains invalid path characters",
		},
		{
			name:        "shell metacharacters",
			keys:        []string{"key$(whoami)"},
			expectError: true,
			errorMsg:    "contains invalid characters",
		},
		{
			name:        "pipe injection",
			keys:        []string{"key | cat /etc/passwd"},
			expectError: true,
			errorMsg:    "contains invalid path characters",
		},
		{
			name:        "too long key",
			keys:        []string{strings.Repeat("a", 254)},
			expectError: true,
			errorMsg:    "too long",
		},
		{
			name:        "valid alphanumeric with allowed chars",
			keys:        []string{"ca_bundle-v1.2.crt"},
			expectError: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateConfigMapKeys(tt.keys)
			if tt.expectError {
				if err == nil {
					t.Errorf("Expected error but got none")
				} else if tt.errorMsg != "" && !strings.Contains(err.Error(), tt.errorMsg) {
					t.Errorf("Expected error message to contain '%s', got '%s'", tt.errorMsg, err.Error())
				}
			} else {
				if err != nil {
					t.Errorf("Expected no error but got: %v", err)
				}
			}
		})
	}
}

func TestDefaultSchedulingAppliedWhenReplicasGreaterThanOne(t *testing.T) {
	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "sample",
			Namespace: "default",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Replicas: 2,
			Server:   llamav1alpha1.ServerSpec{},
		},
	}

	podSpec := configurePodStorage(t.Context(), nil, instance, corev1.Container{})

	require.NotNil(t, podSpec.Affinity)
	require.NotNil(t, podSpec.Affinity.PodAntiAffinity)
	require.Len(t, podSpec.Affinity.PodAntiAffinity.PreferredDuringSchedulingIgnoredDuringExecution, 1)
	require.Len(t, podSpec.TopologySpreadConstraints, 3)

	keys := []string{
		podSpec.TopologySpreadConstraints[0].TopologyKey,
		podSpec.TopologySpreadConstraints[1].TopologyKey,
		podSpec.TopologySpreadConstraints[2].TopologyKey,
	}
	assert.ElementsMatch(t, []string{
		"topology.kubernetes.io/region",
		"topology.kubernetes.io/zone",
		"kubernetes.io/hostname",
	}, keys)
}

func TestDefaultSchedulingSkippedForSingleReplica(t *testing.T) {
	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "solo",
			Namespace: "default",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Replicas: 1,
			Server:   llamav1alpha1.ServerSpec{},
		},
	}

	podSpec := configurePodStorage(t.Context(), nil, instance, corev1.Container{})
	assert.Nil(t, podSpec.Affinity)
	assert.Empty(t, podSpec.TopologySpreadConstraints)
}

func TestCustomTopologySpreadConstraintsRespected(t *testing.T) {
	customConstraint := corev1.TopologySpreadConstraint{
		MaxSkew:           2,
		TopologyKey:       "custom.topology/key",
		WhenUnsatisfiable: corev1.DoNotSchedule,
		LabelSelector: &metav1.LabelSelector{
			MatchLabels: map[string]string{
				"custom": "label",
			},
		},
	}

	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom",
			Namespace: "default",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Replicas: 3,
			Server: llamav1alpha1.ServerSpec{
				TopologySpreadConstraints: []corev1.TopologySpreadConstraint{customConstraint},
			},
		},
	}

	podSpec := configurePodStorage(t.Context(), nil, instance, corev1.Container{})

	require.Len(t, podSpec.TopologySpreadConstraints, 1)
	assert.Equal(t, customConstraint.TopologyKey, podSpec.TopologySpreadConstraints[0].TopologyKey)
	assert.Equal(t, customConstraint.WhenUnsatisfiable, podSpec.TopologySpreadConstraints[0].WhenUnsatisfiable)
}

func TestNeedsPodDisruptionBudget(t *testing.T) {
	tests := []struct {
		name     string
		instance *llamav1alpha1.LlamaStackDistribution
		expected bool
	}{
		{
			name: "single replica without override",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{Replicas: 1, Server: llamav1alpha1.ServerSpec{}},
			},
			expected: false,
		},
		{
			name: "multiple replicas without override",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{Replicas: 2, Server: llamav1alpha1.ServerSpec{}},
			},
			expected: true,
		},
		{
			name: "explicit override",
			instance: &llamav1alpha1.LlamaStackDistribution{
				Spec: llamav1alpha1.LlamaStackDistributionSpec{
					Replicas: 1,
					Server: llamav1alpha1.ServerSpec{
						PodDisruptionBudget: &llamav1alpha1.PodDisruptionBudgetSpec{},
					},
				},
			},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.expected, needsPodDisruptionBudget(tt.instance))
		})
	}
}

func TestBuildPodDisruptionBudgetSpec(t *testing.T) {
	t.Run("defaults when replicas greater than one", func(t *testing.T) {
		instance := &llamav1alpha1.LlamaStackDistribution{
			Spec: llamav1alpha1.LlamaStackDistributionSpec{
				Replicas: 2,
				Server:   llamav1alpha1.ServerSpec{},
			},
		}

		spec := buildPodDisruptionBudgetSpec(instance)
		require.NotNil(t, spec)
		require.NotNil(t, spec.MaxUnavailable)
		assert.Equal(t, 1, spec.MaxUnavailable.IntValue())
		assert.Nil(t, spec.MinAvailable)
	})

	t.Run("respects custom overrides", func(t *testing.T) {
		minValue := intstr.FromString("50%")
		maxValue := intstr.FromInt(1)
		instance := &llamav1alpha1.LlamaStackDistribution{
			Spec: llamav1alpha1.LlamaStackDistributionSpec{
				Replicas: 1,
				Server: llamav1alpha1.ServerSpec{
					PodDisruptionBudget: &llamav1alpha1.PodDisruptionBudgetSpec{
						MinAvailable:   &minValue,
						MaxUnavailable: &maxValue,
					},
				},
			},
		}

		spec := buildPodDisruptionBudgetSpec(instance)
		require.NotNil(t, spec)
		require.NotNil(t, spec.MinAvailable)
		assert.Equal(t, minValue.String(), spec.MinAvailable.String())
		require.NotNil(t, spec.MaxUnavailable)
		assert.Equal(t, maxValue.IntValue(), spec.MaxUnavailable.IntValue())
	})
}

func TestBuildHPASpec(t *testing.T) {
	cpuTarget := int32(70)
	memoryTarget := int32(60)
	minReplicas := int32(3)
	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{Name: "sample"},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Replicas: 2,
			Server: llamav1alpha1.ServerSpec{
				Autoscaling: &llamav1alpha1.AutoscalingSpec{
					MinReplicas:                       &minReplicas,
					MaxReplicas:                       5,
					TargetCPUUtilizationPercentage:    &cpuTarget,
					TargetMemoryUtilizationPercentage: &memoryTarget,
				},
			},
		},
	}

	spec := buildHPASpec(instance)
	require.NotNil(t, spec)
	assert.Equal(t, int32(5), spec.MaxReplicas)
	require.NotNil(t, spec.MinReplicas)
	assert.Equal(t, minReplicas, *spec.MinReplicas)
	require.Len(t, spec.Metrics, 2)
	assert.Equal(t, autoscalingv2.ResourceMetricSourceType, spec.Metrics[0].Type)
	assert.Equal(t, autoscalingv2.UtilizationMetricType, spec.Metrics[0].Resource.Target.Type)
	assert.Equal(t, cpuTarget, *spec.Metrics[0].Resource.Target.AverageUtilization)
	assert.Equal(t, "Deployment", spec.ScaleTargetRef.Kind)
	assert.Equal(t, instance.Name, spec.ScaleTargetRef.Name)

	t.Run("defaults metric when none provided", func(t *testing.T) {
		instance := &llamav1alpha1.LlamaStackDistribution{
			ObjectMeta: metav1.ObjectMeta{Name: "sample"},
			Spec: llamav1alpha1.LlamaStackDistributionSpec{
				Replicas: 1,
				Server: llamav1alpha1.ServerSpec{
					Autoscaling: &llamav1alpha1.AutoscalingSpec{MaxReplicas: 4},
				},
			},
		}

		spec := buildHPASpec(instance)
		require.NotNil(t, spec)
		require.Len(t, spec.Metrics, 1)
		assert.Equal(t, int32(80), *spec.Metrics[0].Resource.Target.AverageUtilization)
		require.NotNil(t, spec.MinReplicas)
		assert.Equal(t, int32(1), *spec.MinReplicas)
	})
}

// newDefaultStartupProbe returns a Kubernetes HTTP readiness probe that checks
// the "/v1/health" endpoint on the given port using default timing and
// threshold settings.
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
