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
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"

	llamav1alpha1 "github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrlLog "sigs.k8s.io/controller-runtime/pkg/log"
)

// Constants for validation limits.
const (
	// maxConfigMapKeyLength defines the maximum allowed length for ConfigMap keys
	// based on Kubernetes DNS subdomain name limits.
	maxConfigMapKeyLength = 253
	// FSGroup is the filesystem group ID for the pod.
	// This is the default group ID for the llama-stack server.
	FSGroup = int64(1001)
	// instanceLabelKey is the label we apply to all resources for per-instance targeting.
	instanceLabelKey = "app.kubernetes.io/instance"
)

var (
	// defaultHPACPUUtilization defines the fallback HPA CPU target percentage.
	defaultHPACPUUtilization = int32(80) //nolint:mnd // standard HPA default
)

// Probes configuration.
const (
	startupProbeInitialDelaySeconds = 15 // Time to wait before the first probe
	startupProbeTimeoutSeconds      = 30 // When the probe times out
	startupProbeFailureThreshold    = 3  // Pod is marked Unhealthy after 3 consecutive failures
	startupProbeSuccessThreshold    = 1  // Pod is marked Ready after 1 successful probe
)

// validConfigMapKeyRegex defines allowed characters for ConfigMap keys.
// Kubernetes ConfigMap keys must be valid DNS subdomain names or data keys.
var validConfigMapKeyRegex = regexp.MustCompile(`^[a-zA-Z0-9]([a-zA-Z0-9\-_.]*[a-zA-Z0-9])?$`)

// getManagedCABundleConfigMapName returns the name of the managed CA bundle ConfigMap.
func getManagedCABundleConfigMapName(instance *llamav1alpha1.LlamaStackDistribution) string {
	return instance.Name + ManagedCABundleConfigMapSuffix
}

// startupScript is the script that will be used to start the server.
var startupScript = `
set -e

# Determine which CLI to use based on llama-stack version
VERSION_CODE=$(python -c "
import sys
from importlib.metadata import version
from packaging import version as pkg_version

try:
    llama_version = version('llama_stack')
    print(f'Detected llama-stack version: {llama_version}', file=sys.stderr)

    v = pkg_version.parse(llama_version)
    # Use base_version to ignore pre-release/post-release/dev suffixes
    # This ensures that 0.3.0rc2, 0.3.0alpha1, etc. are treated as 0.3.0
    base_v = pkg_version.parse(v.base_version)

    if base_v < pkg_version.parse('0.2.17'):
        print('Using legacy module path (llama_stack.distribution.server.server)', file=sys.stderr)
        print(0)
    elif base_v < pkg_version.parse('0.3.0'):
        print('Using core module path (llama_stack.core.server.server)', file=sys.stderr)
        print(1)
    else:
        print('Using uvicorn CLI command', file=sys.stderr)
        print(2)
except Exception as e:
    print(f'Version detection failed, defaulting to new CLI: {e}', file=sys.stderr)
    print(2)
")

PORT=${LLS_PORT:-8321}
WORKERS=${LLS_WORKERS:-1}

# Execute the appropriate CLI based on version
case $VERSION_CODE in
    0) python3 -m llama_stack.distribution.server.server --config /etc/llama-stack/config.yaml ;;
    1) python3 -m llama_stack.core.server.server /etc/llama-stack/config.yaml ;;
    2) exec uvicorn llama_stack.core.server.server:create_app --host 0.0.0.0 --port "$PORT" --workers "$WORKERS" --factory ;;
    *) echo "Invalid version code: $VERSION_CODE, using uvicorn CLI command"; \
       exec uvicorn llama_stack.core.server.server:create_app --host 0.0.0.0 --port "$PORT" --workers "$WORKERS" --factory ;;
esac`

const llamaStackConfigPath = "/etc/llama-stack/config.yaml"

// validateConfigMapKeys validates that all ConfigMap keys contain only safe characters.
// Note: This function validates key names only. PEM content validation is performed
// separately in the controller's reconcileCABundleConfigMap function.
func validateConfigMapKeys(keys []string) error {
	for _, key := range keys {
		if key == "" {
			return errors.New("ConfigMap key cannot be empty")
		}
		if len(key) > maxConfigMapKeyLength {
			return fmt.Errorf("failed to validate ConfigMap key '%s': too long (max %d characters)", key, maxConfigMapKeyLength)
		}
		// Check for path traversal attempts first (before general regex check)
		// to provide specific error messages for security-related issues
		if strings.Contains(key, "..") || strings.Contains(key, "/") {
			return fmt.Errorf("failed to validate ConfigMap key '%s': contains invalid path characters", key)
		}
		if !validConfigMapKeyRegex.MatchString(key) {
			return fmt.Errorf("failed to validate ConfigMap key '%s': contains invalid characters. Only alphanumeric characters, hyphens, underscores, and dots are allowed", key)
		}
	}
	return nil
}

// getHealthProbe returns the health probe handler for the container.
func getHealthProbe(instance *llamav1alpha1.LlamaStackDistribution) corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: "/v1/health",
			Port: intstr.FromInt(int(getContainerPort(instance))),
		},
	}
}

// getStartupProbe returns the startup probe for the container.
func getStartupProbe(instance *llamav1alpha1.LlamaStackDistribution) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:        getHealthProbe(instance),
		InitialDelaySeconds: startupProbeInitialDelaySeconds,
		TimeoutSeconds:      startupProbeTimeoutSeconds,
		FailureThreshold:    startupProbeFailureThreshold,
		SuccessThreshold:    startupProbeSuccessThreshold,
	}
}

// buildContainerSpec creates the container specification.
func buildContainerSpec(ctx context.Context, r *LlamaStackDistributionReconciler, instance *llamav1alpha1.LlamaStackDistribution, image string) corev1.Container {
	workers, workersSet := getEffectiveWorkers(instance)

	container := corev1.Container{
		Name:         getContainerName(instance),
		Image:        image,
		Resources:    resolveContainerResources(instance.Spec.Server.ContainerSpec, workers, workersSet),
		Ports:        []corev1.ContainerPort{{ContainerPort: getContainerPort(instance)}},
		StartupProbe: getStartupProbe(instance),
	}

	// Configure environment variables and mounts
	configureContainerEnvironment(ctx, r, instance, &container)
	configureContainerMounts(ctx, r, instance, &container)
	configureContainerCommands(instance, &container)

	return container
}

// resolveContainerResources ensures the container always has CPU and memory
// requests defined so that HPAs using utilization metrics can function.
func resolveContainerResources(spec llamav1alpha1.ContainerSpec, workers int32, workersSet bool) corev1.ResourceRequirements {
	resources := spec.Resources

	ensureRequests(&resources, workers)
	if workersSet {
		ensureLimitsMatchRequests(&resources)
	}

	cpuReq := resources.Requests[corev1.ResourceCPU]
	memReq := resources.Requests[corev1.ResourceMemory]
	cpuLimit := resources.Limits[corev1.ResourceCPU]
	memLimit := resources.Limits[corev1.ResourceMemory]

	ctrlLog.Log.WithName("resource_helper").WithValues(
		"workers", workers,
		"workersEnabled", workersSet,
	).V(1).Info("Defaulted resource values for llama-stack container",
		"cpuRequest", cpuReq.String(),
		"memoryRequest", memReq.String(),
		"cpuLimit", cpuLimit.String(),
		"memoryLimit", memLimit.String(),
	)

	return resources
}

func ensureRequests(resources *corev1.ResourceRequirements, workers int32) {
	if resources.Requests == nil {
		resources.Requests = corev1.ResourceList{}
	}

	if cpuQty, ok := resources.Requests[corev1.ResourceCPU]; !ok || cpuQty.IsZero() {
		// Default to 1 full core per worker unless user overrides.
		resources.Requests[corev1.ResourceCPU] = resource.MustParse(strconv.Itoa(int(workers)))
	}

	if memQty, ok := resources.Requests[corev1.ResourceMemory]; !ok || memQty.IsZero() {
		resources.Requests[corev1.ResourceMemory] = llamav1alpha1.DefaultServerMemoryRequest
	}
}

func ensureLimitsMatchRequests(resources *corev1.ResourceRequirements) {
	if resources.Limits == nil {
		resources.Limits = corev1.ResourceList{}
	}

	if cpuLimit, ok := resources.Limits[corev1.ResourceCPU]; !ok || cpuLimit.IsZero() {
		resources.Limits[corev1.ResourceCPU] = resources.Requests[corev1.ResourceCPU]
	}

	if memLimit, ok := resources.Limits[corev1.ResourceMemory]; !ok || memLimit.IsZero() {
		resources.Limits[corev1.ResourceMemory] = resources.Requests[corev1.ResourceMemory]
	}
}

// getContainerName returns the container name, using custom name if specified.
func getContainerName(instance *llamav1alpha1.LlamaStackDistribution) string {
	if instance.Spec.Server.ContainerSpec.Name != "" {
		return instance.Spec.Server.ContainerSpec.Name
	}
	return llamav1alpha1.DefaultContainerName
}

// getContainerPort returns the container port, using custom port if specified.
func getContainerPort(instance *llamav1alpha1.LlamaStackDistribution) int32 {
	if instance.Spec.Server.ContainerSpec.Port != 0 {
		return instance.Spec.Server.ContainerSpec.Port
	}
	return llamav1alpha1.DefaultServerPort
}

// getEffectiveWorkers returns a positive worker count, defaulting to 1.
func getEffectiveWorkers(instance *llamav1alpha1.LlamaStackDistribution) (int32, bool) {
	if instance.Spec.Server.Workers != nil && *instance.Spec.Server.Workers > 0 {
		return *instance.Spec.Server.Workers, true
	}
	return 1, false
}

// configureContainerEnvironment sets up environment variables for the container.
func configureContainerEnvironment(ctx context.Context, r *LlamaStackDistributionReconciler, instance *llamav1alpha1.LlamaStackDistribution, container *corev1.Container) {
	mountPath := getMountPath(instance)
	workers, _ := getEffectiveWorkers(instance)

	// Add HF_HOME variable to our mount path so that downloaded models and datasets are stored
	// on the same volume as the storage. This is not critical but useful if the server is
	// restarted so the models and datasets are not lost and need to be downloaded again.
	// For more information, see https://huggingface.co/docs/datasets/en/cache
	container.Env = append(container.Env, corev1.EnvVar{
		Name:  "HF_HOME",
		Value: mountPath,
	})

	// Add CA bundle environment variable if any CA bundles are configured
	// (explicit or auto-detected ODH bundles)
	if hasAnyCABundle(ctx, r, instance) {
		// Set SSL_CERT_FILE to point to the managed CA bundle file
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "SSL_CERT_FILE",
			Value: ManagedCABundleFilePath,
		})
	}

	// Always provide worker/port/config env for uvicorn; workers default to 1 when unspecified.
	container.Env = append(container.Env,
		corev1.EnvVar{
			Name:  "LLS_WORKERS",
			Value: strconv.Itoa(int(workers)),
		},
		corev1.EnvVar{
			Name:  "LLS_PORT",
			Value: strconv.Itoa(int(getContainerPort(instance))),
		},
		corev1.EnvVar{
			Name:  "LLAMA_STACK_CONFIG",
			Value: llamaStackConfigPath,
		},
	)

	// Finally, add the user provided env vars
	container.Env = append(container.Env, instance.Spec.Server.ContainerSpec.Env...)
	container.EnvFrom = append(container.EnvFrom, instance.Spec.Server.ContainerSpec.EnvFrom...)
}

// configureContainerMounts sets up volume mounts for the container.
func configureContainerMounts(ctx context.Context, r *LlamaStackDistributionReconciler, instance *llamav1alpha1.LlamaStackDistribution, container *corev1.Container) {
	// Add volume mount for storage
	addStorageVolumeMount(instance, container)

	// Add ConfigMap volume mount if user config is specified
	addUserConfigVolumeMount(instance, container)

	// Add CA bundle volume mount if TLS config is specified or auto-detected
	addCABundleVolumeMount(ctx, r, instance, container)
}

// hasAnyCABundle checks if any CA bundle will be mounted (explicit or auto-detected).
func hasAnyCABundle(ctx context.Context, r *LlamaStackDistributionReconciler, instance *llamav1alpha1.LlamaStackDistribution) bool {
	// Check for explicit CA bundle configuration
	if instance.Spec.Server.TLSConfig != nil && instance.Spec.Server.TLSConfig.CABundle != nil {
		return true
	}

	// Check for auto-detected ODH trusted CA bundle
	if r != nil {
		if _, keys, err := r.detectODHTrustedCABundle(ctx, instance); err == nil && len(keys) > 0 {
			return true
		}
	}

	return false
}

// configureContainerCommands sets up container commands and args.
func configureContainerCommands(instance *llamav1alpha1.LlamaStackDistribution, container *corev1.Container) {
	// Override the container entrypoint to use the custom config file if user config is specified
	if instance.Spec.Server.UserConfig != nil && instance.Spec.Server.UserConfig.ConfigMapName != "" {
		// Override the container entrypoint to use the custom config file instead of the default
		// template. The script will determine the llama-stack version and use the appropriate module
		// path to start the server.

		container.Command = []string{"/bin/sh", "-c", startupScript}
		container.Args = []string{}
	}

	// Apply user-specified command and args (takes precedence)
	if len(instance.Spec.Server.ContainerSpec.Command) > 0 {
		container.Command = instance.Spec.Server.ContainerSpec.Command
	}

	if len(instance.Spec.Server.ContainerSpec.Args) > 0 {
		container.Args = instance.Spec.Server.ContainerSpec.Args
	}
}

// getMountPath returns the mount path, using custom path if specified.
func getMountPath(instance *llamav1alpha1.LlamaStackDistribution) string {
	if instance.Spec.Server.Storage != nil && instance.Spec.Server.Storage.MountPath != "" {
		return instance.Spec.Server.Storage.MountPath
	}
	return llamav1alpha1.DefaultMountPath
}

// addStorageVolumeMount adds the storage volume mount to the container.
func addStorageVolumeMount(instance *llamav1alpha1.LlamaStackDistribution, container *corev1.Container) {
	mountPath := getMountPath(instance)
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      "lls-storage",
		MountPath: mountPath,
	})
}

// addUserConfigVolumeMount adds the user config volume mount to the container if specified.
func addUserConfigVolumeMount(instance *llamav1alpha1.LlamaStackDistribution, container *corev1.Container) {
	if instance.Spec.Server.UserConfig != nil && instance.Spec.Server.UserConfig.ConfigMapName != "" {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "user-config",
			MountPath: "/etc/llama-stack/",
			ReadOnly:  true,
		})
	}
}

// addCABundleVolumeMount adds the managed CA bundle volume mount to the container.
// Mounts the operator-managed ConfigMap containing all concatenated certificates.
func addCABundleVolumeMount(ctx context.Context, r *LlamaStackDistributionReconciler, instance *llamav1alpha1.LlamaStackDistribution, container *corev1.Container) {
	// Mount managed CA bundle if any CA bundles are configured
	if hasAnyCABundle(ctx, r, instance) {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      CABundleVolumeName,
			MountPath: ManagedCABundleMountPath,
			ReadOnly:  true,
		})
	}
}

// createCABundleVolume creates the volume configuration for the managed CA bundle ConfigMap.
func createCABundleVolume(managedConfigMapName string) corev1.Volume {
	return corev1.Volume{
		Name: CABundleVolumeName,
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: managedConfigMapName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  ManagedCABundleKey,
						Path: ManagedCABundleKey,
					},
				},
			},
		},
	}
}

// configurePodStorage configures the pod storage and returns the complete pod spec.
func configurePodStorage(ctx context.Context, r *LlamaStackDistributionReconciler, instance *llamav1alpha1.LlamaStackDistribution, container corev1.Container) corev1.PodSpec {
	fsGroup := FSGroup
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
		SecurityContext: &corev1.PodSecurityContext{
			FSGroup: &fsGroup,
		},
	}

	// Configure storage volumes
	configureStorage(instance, &podSpec)

	// Configure TLS CA bundle (with auto-detection support)
	configureTLSCABundle(ctx, r, instance, &podSpec)

	// Configure user config
	configureUserConfig(instance, &podSpec)

	// Apply pod overrides including ServiceAccount, volumes, and volume mounts
	configurePodOverrides(instance, &podSpec)

	configurePodScheduling(instance, &podSpec)

	return podSpec
}

// configureStorage handles storage volume configuration.
func configureStorage(instance *llamav1alpha1.LlamaStackDistribution, podSpec *corev1.PodSpec) {
	if instance.Spec.Server.Storage != nil {
		configurePersistentStorage(instance, podSpec)
	} else {
		configureEmptyDirStorage(podSpec)
	}
}

// configurePersistentStorage sets up PVC-based storage.
func configurePersistentStorage(instance *llamav1alpha1.LlamaStackDistribution, podSpec *corev1.PodSpec) {
	// Use PVC for persistent storage
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "lls-storage",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: instance.Name + "-pvc",
			},
		},
	})
}

// configureEmptyDirStorage sets up temporary storage using emptyDir.
func configureEmptyDirStorage(podSpec *corev1.PodSpec) {
	// Use emptyDir for non-persistent storage
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "lls-storage",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
}

// configureTLSCABundle handles TLS CA bundle configuration.
// Mounts the operator-managed CA bundle ConfigMap that contains all certificates.
func configureTLSCABundle(ctx context.Context, r *LlamaStackDistributionReconciler, instance *llamav1alpha1.LlamaStackDistribution, podSpec *corev1.PodSpec) {
	// Check if any CA bundles are configured (explicit or auto-detected ODH)
	if !hasAnyCABundle(ctx, r, instance) {
		return
	}

	// Add the managed CA bundle ConfigMap volume
	managedConfigMapName := getManagedCABundleConfigMapName(instance)
	volume := createCABundleVolume(managedConfigMapName)
	podSpec.Volumes = append(podSpec.Volumes, volume)
}

// configureUserConfig handles user configuration setup.
func configureUserConfig(instance *llamav1alpha1.LlamaStackDistribution, podSpec *corev1.PodSpec) {
	userConfig := instance.Spec.Server.UserConfig
	if userConfig == nil || userConfig.ConfigMapName == "" {
		return
	}

	// Add ConfigMap volume if user config is specified
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "user-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: userConfig.ConfigMapName,
				},
			},
		},
	})
}

// configurePodOverrides applies pod-level overrides from the LlamaStackDistribution spec.
func configurePodOverrides(instance *llamav1alpha1.LlamaStackDistribution, podSpec *corev1.PodSpec) {
	// Set ServiceAccount name - use override if specified, otherwise use default
	if instance.Spec.Server.PodOverrides != nil && instance.Spec.Server.PodOverrides.ServiceAccountName != "" {
		podSpec.ServiceAccountName = instance.Spec.Server.PodOverrides.ServiceAccountName
	} else {
		podSpec.ServiceAccountName = instance.Name + "-sa"
	}

	// Apply other pod overrides if specified
	if instance.Spec.Server.PodOverrides != nil {
		// Add volumes if specified
		if len(instance.Spec.Server.PodOverrides.Volumes) > 0 {
			podSpec.Volumes = append(podSpec.Volumes, instance.Spec.Server.PodOverrides.Volumes...)
		}

		// Add volume mounts if specified
		if len(instance.Spec.Server.PodOverrides.VolumeMounts) > 0 {
			if len(podSpec.Containers) > 0 {
				podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, instance.Spec.Server.PodOverrides.VolumeMounts...)
			}
		}

		// Apply termination grace period if specified
		if instance.Spec.Server.PodOverrides.TerminationGracePeriodSeconds != nil {
			podSpec.TerminationGracePeriodSeconds = instance.Spec.Server.PodOverrides.TerminationGracePeriodSeconds
		}
	}
}

func configurePodScheduling(instance *llamav1alpha1.LlamaStackDistribution, podSpec *corev1.PodSpec) {
	if len(instance.Spec.Server.TopologySpreadConstraints) > 0 {
		podSpec.TopologySpreadConstraints = deepCopyTopologySpreadConstraints(instance.Spec.Server.TopologySpreadConstraints)
	} else if instance.Spec.Replicas > 1 {
		podSpec.TopologySpreadConstraints = defaultTopologySpreadConstraints(instance)
	}

	if instance.Spec.Replicas > 1 {
		ensureDefaultPodAntiAffinity(instance, podSpec)
	}
}

func deepCopyTopologySpreadConstraints(constraints []corev1.TopologySpreadConstraint) []corev1.TopologySpreadConstraint {
	copied := make([]corev1.TopologySpreadConstraint, len(constraints))
	for i := range constraints {
		copied[i] = *constraints[i].DeepCopy()
	}
	return copied
}

func defaultTopologySpreadConstraints(instance *llamav1alpha1.LlamaStackDistribution) []corev1.TopologySpreadConstraint {
	labelSelector := defaultInstanceLabelSelector(instance)
	return []corev1.TopologySpreadConstraint{
		newTopologySpreadConstraint(labelSelector, "topology.kubernetes.io/region"),
		newTopologySpreadConstraint(labelSelector, "topology.kubernetes.io/zone"),
		newTopologySpreadConstraint(labelSelector, "kubernetes.io/hostname"),
	}
}

func newTopologySpreadConstraint(selector *metav1.LabelSelector, topologyKey string) corev1.TopologySpreadConstraint {
	return corev1.TopologySpreadConstraint{
		MaxSkew:           1,
		TopologyKey:       topologyKey,
		WhenUnsatisfiable: corev1.ScheduleAnyway,
		LabelSelector:     selector.DeepCopy(),
	}
}

func ensureDefaultPodAntiAffinity(instance *llamav1alpha1.LlamaStackDistribution, podSpec *corev1.PodSpec) {
	if podSpec.Affinity != nil && podSpec.Affinity.PodAntiAffinity != nil {
		return
	}

	selector := defaultInstanceLabelSelector(instance)
	term := corev1.PodAffinityTerm{
		LabelSelector: selector,
		TopologyKey:   "kubernetes.io/hostname",
	}

	defaultAntiAffinity := &corev1.PodAntiAffinity{
		PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
			{
				Weight:          100,
				PodAffinityTerm: term,
			},
		},
	}

	if podSpec.Affinity == nil {
		podSpec.Affinity = &corev1.Affinity{}
	}

	// Deep copy to avoid sharing selectors across pods
	podSpec.Affinity.PodAntiAffinity = defaultAntiAffinity.DeepCopy()
}

func defaultInstanceLabelSelector(instance *llamav1alpha1.LlamaStackDistribution) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			instanceLabelKey: instance.Name,
		},
	}
}

// validateDistribution validates the distribution configuration.
func (r *LlamaStackDistributionReconciler) validateDistribution(instance *llamav1alpha1.LlamaStackDistribution) error {
	// If using distribution name, validate it exists in clusterInfo
	if instance.Spec.Server.Distribution.Name != "" {
		if r.ClusterInfo == nil {
			return errors.New("failed to initialize cluster info")
		}
		if _, exists := r.ClusterInfo.DistributionImages[instance.Spec.Server.Distribution.Name]; !exists {
			return fmt.Errorf("failed to validate distribution: %s. Distribution name not supported", instance.Spec.Server.Distribution.Name)
		}
	}

	return nil
}

// resolveImage determines the container image to use based on the distribution configuration.
// It returns the resolved image and any error encountered.
func (r *LlamaStackDistributionReconciler) resolveImage(distribution llamav1alpha1.DistributionType) (string, error) {
	distributionMap := r.ClusterInfo.DistributionImages
	switch {
	case distribution.Name != "":
		if _, exists := distributionMap[distribution.Name]; !exists {
			return "", fmt.Errorf("failed to validate distribution name: %s", distribution.Name)
		}
		// Check for image override in the operator config ConfigMap
		// The override is keyed by distribution name only (e.g., "starter")
		// This allows the same override to apply across all distributions
		if override, exists := r.ImageMappingOverrides[distribution.Name]; exists {
			return override, nil
		}
		return distributionMap[distribution.Name], nil
	case distribution.Image != "":
		return distribution.Image, nil
	default:
		return "", errors.New("failed to validate distribution: either distribution.name or distribution.image must be set")
	}
}

func buildPodDisruptionBudgetSpec(instance *llamav1alpha1.LlamaStackDistribution) *policyv1.PodDisruptionBudgetSpec {
	if !needsPodDisruptionBudget(instance) {
		return nil
	}

	spec := &policyv1.PodDisruptionBudgetSpec{}
	if instance.Spec.Server.PodDisruptionBudget != nil {
		spec.MinAvailable = copyIntOrString(instance.Spec.Server.PodDisruptionBudget.MinAvailable)
		spec.MaxUnavailable = copyIntOrString(instance.Spec.Server.PodDisruptionBudget.MaxUnavailable)
	} else {
		// Fix for RHAIENG-3783: Use maxUnavailable instead of minAvailable
		// to avoid allowedDisruptions=0 with single-replica deployments
		maxUnavailable := intstr.FromInt(1)
		spec.MaxUnavailable = &maxUnavailable
	}

	return spec
}

func buildHPASpec(instance *llamav1alpha1.LlamaStackDistribution) *autoscalingv2.HorizontalPodAutoscalerSpec {
	auto := instance.Spec.Server.Autoscaling
	if auto == nil || auto.MaxReplicas == 0 {
		return nil
	}

	minReplicas := resolveMinReplicas(auto.MinReplicas, instance.Spec.Replicas)

	spec := &autoscalingv2.HorizontalPodAutoscalerSpec{
		ScaleTargetRef: autoscalingv2.CrossVersionObjectReference{
			APIVersion: "apps/v1",
			Kind:       "Deployment",
			Name:       instance.Name,
		},
		MinReplicas: minReplicas,
		MaxReplicas: auto.MaxReplicas,
		Metrics:     buildHPAMetrics(auto),
	}

	return spec
}

func resolveMinReplicas(value *int32, defaultVal int32) *int32 {
	resolved := int32(1)
	if defaultVal > resolved {
		resolved = defaultVal
	}
	if value != nil && *value > resolved {
		resolved = *value
	}
	return &resolved
}

func buildHPAMetrics(auto *llamav1alpha1.AutoscalingSpec) []autoscalingv2.MetricSpec {
	var metrics []autoscalingv2.MetricSpec

	if auto.TargetCPUUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: auto.TargetCPUUtilizationPercentage,
				},
			},
		})
	}

	if auto.TargetMemoryUtilizationPercentage != nil {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceMemory,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: auto.TargetMemoryUtilizationPercentage,
				},
			},
		})
	}

	if len(metrics) == 0 {
		metrics = append(metrics, autoscalingv2.MetricSpec{
			Type: autoscalingv2.ResourceMetricSourceType,
			Resource: &autoscalingv2.ResourceMetricSource{
				Name: corev1.ResourceCPU,
				Target: autoscalingv2.MetricTarget{
					Type:               autoscalingv2.UtilizationMetricType,
					AverageUtilization: &defaultHPACPUUtilization,
				},
			},
		})
	}

	return metrics
}

func needsPodDisruptionBudget(instance *llamav1alpha1.LlamaStackDistribution) bool {
	if instance.Spec.Server.PodDisruptionBudget != nil {
		return true
	}
	return instance.Spec.Replicas > 1
}

func copyIntOrString(value *intstr.IntOrString) *intstr.IntOrString {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
