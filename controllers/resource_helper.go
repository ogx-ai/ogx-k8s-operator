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
	"strconv"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/deploy"
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
	// FSGroup is the filesystem group ID for the pod.
	// This is the default group ID for the ogx server.
	FSGroup = int64(1001)
	// instanceLabelKey is the label we apply to all resources for per-instance targeting.
	instanceLabelKey = "app.kubernetes.io/instance"
)

var (
	// defaultHPACPUUtilization defines the fallback HPA CPU target percentage.
	defaultHPACPUUtilization = int32(80) //nolint:mnd // standard HPA default
)

type runtimeConfigRef struct {
	ConfigMapName string
	ConfigMapKey  string
	Generated     bool
}

func resolveRuntimeConfigRef(instance *ogxiov1beta1.OGXServer, pendingGeneratedConfigMapName string) *runtimeConfigRef {
	if overrideConfig := instance.Spec.OverrideConfig; overrideConfig != nil &&
		overrideConfig.Name != "" && overrideConfig.Key != "" {
		return &runtimeConfigRef{
			ConfigMapName: overrideConfig.Name,
			ConfigMapKey:  overrideConfig.Key,
		}
	}

	if pendingGeneratedConfigMapName != "" {
		return &runtimeConfigRef{
			ConfigMapName: pendingGeneratedConfigMapName,
			ConfigMapKey:  generatedConfigKeyName,
			Generated:     true,
		}
	}

	return nil
}

// Probes configuration.
const (
	startupProbeInitialDelaySeconds = 15 // Time to wait before the first probe
	startupProbeTimeoutSeconds      = 30 // When the probe times out
	startupProbeFailureThreshold    = 3  // Pod is marked Unhealthy after 3 consecutive failures
	startupProbeSuccessThreshold    = 1  // Pod is marked Ready after 1 successful probe
)

// getManagedCABundleConfigMapName returns the name of the managed CA bundle ConfigMap.
func getManagedCABundleConfigMapName(instance *ogxiov1beta1.OGXServer) string {
	return instance.Name + ManagedCABundleConfigMapSuffix
}

// startupScript is the script that will be used to start the server.
var startupScript = `
set -e

# Determine which CLI to use based on ogx version
VERSION_CODE=$(python -c "
import sys
from importlib.metadata import version
from packaging import version as pkg_version

try:
    ogx_version = version('ogx')
    print(f'Detected ogx version: {ogx_version}', file=sys.stderr)

    v = pkg_version.parse(ogx_version)
    # Use base_version to ignore pre-release/post-release/dev suffixes
    # This ensures that 0.3.0rc2, 0.3.0alpha1, etc. are treated as 0.3.0
    base_v = pkg_version.parse(v.base_version)

    if base_v < pkg_version.parse('0.2.17'):
        print('Using legacy module path (ogx.distribution.server.server)', file=sys.stderr)
        print(0)
    elif base_v < pkg_version.parse('0.3.0'):
        print('Using core module path (ogx.core.server.server)', file=sys.stderr)
        print(1)
    else:
        print('Using uvicorn CLI command', file=sys.stderr)
        print(2)
except Exception as e:
    print(f'Version detection failed, defaulting to new CLI: {e}', file=sys.stderr)
    print(2)
")

PORT=${OGX_PORT:-8321}
WORKERS=${OGX_WORKERS:-1}

# Execute the appropriate CLI based on version
case $VERSION_CODE in
    0) python3 -m ogx.distribution.server.server --config /etc/ogx/config.yaml ;;
    1) python3 -m ogx.core.server.server /etc/ogx/config.yaml ;;
    2) exec uvicorn ogx.core.server.server:create_app --host 0.0.0.0 --port "$PORT" --workers "$WORKERS" --factory ;;
    *) echo "Invalid version code: $VERSION_CODE, using uvicorn CLI command"; \
       exec uvicorn ogx.core.server.server:create_app --host 0.0.0.0 --port "$PORT" --workers "$WORKERS" --factory ;;
esac`

const ogxConfigPath = "/etc/ogx/config.yaml"

// getHealthProbe returns the health probe handler for the container.
func getHealthProbe(instance *ogxiov1beta1.OGXServer) corev1.ProbeHandler {
	return corev1.ProbeHandler{
		HTTPGet: &corev1.HTTPGetAction{
			Path: "/v1/health",
			Port: intstr.FromInt(int(getContainerPort(instance))),
		},
	}
}

// getStartupProbe returns the startup probe for the container.
func getStartupProbe(instance *ogxiov1beta1.OGXServer) *corev1.Probe {
	return &corev1.Probe{
		ProbeHandler:        getHealthProbe(instance),
		InitialDelaySeconds: startupProbeInitialDelaySeconds,
		TimeoutSeconds:      startupProbeTimeoutSeconds,
		FailureThreshold:    startupProbeFailureThreshold,
		SuccessThreshold:    startupProbeSuccessThreshold,
	}
}

// buildContainerSpec creates the container specification.
func buildContainerSpec(
	ctx context.Context,
	r *OGXServerReconciler,
	instance *ogxiov1beta1.OGXServer,
	image string,
	runtimeConfig *runtimeConfigRef,
	secretEnvVars []corev1.EnvVar,
) corev1.Container {
	workers, workersSet := getEffectiveWorkers(instance)
	container := corev1.Container{
		Name:         ogxiov1beta1.DefaultContainerName,
		Image:        image,
		Resources:    resolveContainerResources(instance, workers, workersSet),
		Ports:        []corev1.ContainerPort{{ContainerPort: getContainerPort(instance)}},
		StartupProbe: getStartupProbe(instance),
	}
	configureContainerEnvironment(ctx, r, instance, &container, secretEnvVars)
	configureContainerMounts(ctx, r, instance, runtimeConfig, &container)
	configureContainerCommands(instance, runtimeConfig, &container)
	return container
}

// resolveContainerResources ensures the container always has CPU and memory
// requests defined so that HPAs using utilization metrics can function.
func resolveContainerResources(instance *ogxiov1beta1.OGXServer, workers int32, workersSet bool) corev1.ResourceRequirements {
	var resources corev1.ResourceRequirements
	if instance.Spec.Workload != nil && instance.Spec.Workload.Resources != nil {
		resources = *instance.Spec.Workload.Resources
	}
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
	).V(1).Info("Defaulted resource values for ogx container",
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
		resources.Requests[corev1.ResourceMemory] = ogxiov1beta1.DefaultServerMemoryRequest
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

// getContainerPort returns the container port, using custom port if specified.
func getContainerPort(instance *ogxiov1beta1.OGXServer) int32 {
	if instance.Spec.Network != nil && instance.Spec.Network.Port != 0 {
		return instance.Spec.Network.Port
	}
	return ogxiov1beta1.DefaultServerPort
}

// getEffectiveWorkers returns a positive worker count, defaulting to 1.
func getEffectiveWorkers(instance *ogxiov1beta1.OGXServer) (int32, bool) {
	if instance.Spec.Workload != nil && instance.Spec.Workload.Workers != nil && *instance.Spec.Workload.Workers > 0 {
		return *instance.Spec.Workload.Workers, true
	}
	return 1, false
}

// configureContainerEnvironment sets up environment variables for the container.
func configureContainerEnvironment(ctx context.Context, r *OGXServerReconciler, instance *ogxiov1beta1.OGXServer, container *corev1.Container, secretEnvVars []corev1.EnvVar) {
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
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "SSL_CERT_FILE",
			Value: ManagedCABundleFilePath,
		})
	}

	// Always provide worker/port/config env for uvicorn; workers default to 1 when unspecified.
	container.Env = append(container.Env,
		corev1.EnvVar{
			Name:  "OGX_WORKERS",
			Value: strconv.Itoa(int(workers)),
		},
		corev1.EnvVar{
			Name:  "OGX_PORT",
			Value: strconv.Itoa(int(getContainerPort(instance))),
		},
		corev1.EnvVar{
			Name:  "OGX_CONFIG",
			Value: ogxConfigPath,
		},
	)

	if instance.Spec.RegistryRefreshIntervalSeconds != nil {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "OGX_REGISTRY_REFRESH_INTERVAL_SECONDS",
			Value: strconv.Itoa(int(*instance.Spec.RegistryRefreshIntervalSeconds)),
		})
	}
	// Inject pre-computed secret env vars for provider/storage references.
	// These are computed once in buildManifestContext to avoid redundant tree walks.
	if len(secretEnvVars) > 0 {
		container.Env = append(container.Env, secretEnvVars...)
	}

	// Apply user-provided env vars, letting them override operator defaults.
	if instance.Spec.Workload != nil && instance.Spec.Workload.Overrides != nil {
		overrides := make(map[string]corev1.EnvVar, len(instance.Spec.Workload.Overrides.Env))
		for _, e := range instance.Spec.Workload.Overrides.Env {
			overrides[e.Name] = e
		}
		deduped := make([]corev1.EnvVar, 0, len(container.Env))
		for _, e := range container.Env {
			if _, ok := overrides[e.Name]; ok {
				continue
			}
			deduped = append(deduped, e)
		}
		deduped = append(deduped, instance.Spec.Workload.Overrides.Env...)
		container.Env = deduped
	}
}

// configureContainerMounts sets up volume mounts for the container.
func configureContainerMounts(
	ctx context.Context,
	r *OGXServerReconciler,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
	container *corev1.Container,
) {
	// Add volume mount for storage
	addStorageVolumeMount(instance, container)

	// Mount the final runtime config when it comes from overrideConfig or
	// operator-generated config. spec.baseConfig is an input to generation only
	// and is never mounted into the pod directly.
	addRuntimeConfigVolumeMount(runtimeConfig, container)

	// Add CA bundle volume mount if TLS config is specified or auto-detected
	addCABundleVolumeMount(ctx, r, instance, container)
}

// hasAnyCABundle checks if any CA bundle will be mounted (explicit or auto-detected).
func hasAnyCABundle(ctx context.Context, r *OGXServerReconciler, instance *ogxiov1beta1.OGXServer) bool {
	// Check for explicit CA certificate configuration
	if instance.Spec.TLS != nil && instance.Spec.TLS.Trust != nil && len(instance.Spec.TLS.Trust.CACertificates) > 0 {
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
func configureContainerCommands(instance *ogxiov1beta1.OGXServer, runtimeConfig *runtimeConfigRef, container *corev1.Container) {
	if runtimeConfig != nil {
		container.Command = []string{"/bin/sh", "-c", startupScript}
		container.Args = []string{}
	}

	// Apply user-specified command and args (takes precedence)
	if instance.Spec.Workload != nil && instance.Spec.Workload.Overrides != nil {
		if len(instance.Spec.Workload.Overrides.Command) > 0 {
			container.Command = instance.Spec.Workload.Overrides.Command
		}
		if len(instance.Spec.Workload.Overrides.Args) > 0 {
			container.Args = instance.Spec.Workload.Overrides.Args
		}
	}
}

// getMountPath returns the mount path, using custom path if specified.
func getMountPath(instance *ogxiov1beta1.OGXServer) string {
	if instance.Spec.Workload != nil && instance.Spec.Workload.Storage != nil && instance.Spec.Workload.Storage.MountPath != "" {
		return instance.Spec.Workload.Storage.MountPath
	}
	return ogxiov1beta1.DefaultMountPath
}

// addStorageVolumeMount adds the storage volume mount to the container.
func addStorageVolumeMount(instance *ogxiov1beta1.OGXServer, container *corev1.Container) {
	mountPath := getMountPath(instance)
	container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
		Name:      "ogx-storage",
		MountPath: mountPath,
	})
}

// addRuntimeConfigVolumeMount mounts the final runtime config.yaml into the pod.
// The mounted ConfigMap comes from either spec.overrideConfig or the operator's
// generated ConfigMap. spec.baseConfig is not mounted directly.
func addRuntimeConfigVolumeMount(runtimeConfig *runtimeConfigRef, container *corev1.Container) {
	if runtimeConfig != nil {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "user-config",
			MountPath: "/etc/ogx/",
			ReadOnly:  true,
		})
	}
}

// addCABundleVolumeMount adds the managed CA bundle volume mount to the container.
// Mounts the operator-managed ConfigMap containing all concatenated certificates.
func addCABundleVolumeMount(ctx context.Context, r *OGXServerReconciler, instance *ogxiov1beta1.OGXServer, container *corev1.Container) {
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
func configurePodStorage(
	ctx context.Context,
	r *OGXServerReconciler,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
	container corev1.Container,
	effectivePVCName string,
) corev1.PodSpec {
	fsGroup := FSGroup
	podSpec := corev1.PodSpec{
		Containers: []corev1.Container{container},
		SecurityContext: &corev1.PodSecurityContext{
			FSGroup: &fsGroup,
		},
	}

	// Configure storage volumes
	configureStorage(instance, &podSpec, effectivePVCName)

	// Configure TLS CA bundle (with auto-detection support)
	configureTLSCABundle(ctx, r, instance, &podSpec)

	// Configure the final runtime config volume.
	configureRuntimeConfigVolume(runtimeConfig, &podSpec)

	// Apply pod overrides including ServiceAccount, volumes, and volume mounts
	configurePodOverrides(instance, &podSpec)

	configurePodScheduling(instance, &podSpec)

	return podSpec
}

// configureStorage handles storage volume configuration.
func configureStorage(instance *ogxiov1beta1.OGXServer, podSpec *corev1.PodSpec, effectivePVCName string) {
	if instance.Spec.Workload != nil && instance.Spec.Workload.Storage != nil {
		configurePersistentStorage(podSpec, effectivePVCName)
	} else {
		configureEmptyDirStorage(podSpec)
	}
}

// configurePersistentStorage sets up PVC-based storage.
func configurePersistentStorage(podSpec *corev1.PodSpec, pvcName string) {
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "ogx-storage",
		VolumeSource: corev1.VolumeSource{
			PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
				ClaimName: pvcName,
			},
		},
	})
}

// configureEmptyDirStorage sets up temporary storage using emptyDir.
func configureEmptyDirStorage(podSpec *corev1.PodSpec) {
	// Use emptyDir for non-persistent storage
	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "ogx-storage",
		VolumeSource: corev1.VolumeSource{
			EmptyDir: &corev1.EmptyDirVolumeSource{},
		},
	})
}

// configureTLSCABundle handles TLS CA bundle configuration.
// Mounts the operator-managed CA bundle ConfigMap that contains all certificates.
func configureTLSCABundle(ctx context.Context, r *OGXServerReconciler, instance *ogxiov1beta1.OGXServer, podSpec *corev1.PodSpec) {
	// Check if any CA bundles are configured (explicit or auto-detected ODH)
	if !hasAnyCABundle(ctx, r, instance) {
		return
	}

	// Add the managed CA bundle ConfigMap volume
	managedConfigMapName := getManagedCABundleConfigMapName(instance)
	volume := createCABundleVolume(managedConfigMapName)
	podSpec.Volumes = append(podSpec.Volumes, volume)
}

// configureRuntimeConfigVolume adds the ConfigMap volume that provides the
// runtime config.yaml consumed by the container. Override config takes
// precedence over generated config, and spec.baseConfig is never mounted.
func configureRuntimeConfigVolume(runtimeConfig *runtimeConfigRef, podSpec *corev1.PodSpec) {
	if runtimeConfig == nil {
		return
	}

	podSpec.Volumes = append(podSpec.Volumes, corev1.Volume{
		Name: "user-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{
					Name: runtimeConfig.ConfigMapName,
				},
				Items: []corev1.KeyToPath{
					{
						Key:  runtimeConfig.ConfigMapKey,
						Path: "config.yaml",
					},
				},
			},
		},
	})
}

// configurePodOverrides applies pod-level overrides from the OGXServer spec.
func configurePodOverrides(instance *ogxiov1beta1.OGXServer, podSpec *corev1.PodSpec) {
	if instance.Spec.Workload != nil && instance.Spec.Workload.Overrides != nil && instance.Spec.Workload.Overrides.ServiceAccountName != "" {
		podSpec.ServiceAccountName = instance.Spec.Workload.Overrides.ServiceAccountName
	} else {
		podSpec.ServiceAccountName = instance.Name + "-sa"
	}

	if instance.Spec.Workload != nil && instance.Spec.Workload.Overrides != nil {
		overrides := instance.Spec.Workload.Overrides
		if len(overrides.Volumes) > 0 {
			podSpec.Volumes = append(podSpec.Volumes, overrides.Volumes...)
		}
		if len(overrides.VolumeMounts) > 0 {
			if len(podSpec.Containers) > 0 {
				podSpec.Containers[0].VolumeMounts = append(podSpec.Containers[0].VolumeMounts, overrides.VolumeMounts...)
			}
		}
	}
}

func configurePodScheduling(instance *ogxiov1beta1.OGXServer, podSpec *corev1.PodSpec) {
	if instance.Spec.Workload != nil && len(instance.Spec.Workload.TopologySpreadConstraints) > 0 {
		podSpec.TopologySpreadConstraints = deepCopyTopologySpreadConstraints(instance.Spec.Workload.TopologySpreadConstraints)
	} else if deploy.GetEffectiveReplicas(instance) > 1 {
		podSpec.TopologySpreadConstraints = defaultTopologySpreadConstraints(instance)
	}

	if deploy.GetEffectiveReplicas(instance) > 1 {
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

func defaultTopologySpreadConstraints(instance *ogxiov1beta1.OGXServer) []corev1.TopologySpreadConstraint {
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

func ensureDefaultPodAntiAffinity(instance *ogxiov1beta1.OGXServer, podSpec *corev1.PodSpec) {
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

func defaultInstanceLabelSelector(instance *ogxiov1beta1.OGXServer) *metav1.LabelSelector {
	return &metav1.LabelSelector{
		MatchLabels: map[string]string{
			instanceLabelKey: instance.Name,
		},
	}
}

// validateDistribution validates the distribution configuration.
func (r *OGXServerReconciler) validateDistribution(instance *ogxiov1beta1.OGXServer) error {
	// If using distribution name, validate it exists in clusterInfo
	if instance.Spec.Distribution.Name != "" {
		if r.ClusterInfo == nil {
			return errors.New("failed to initialize cluster info")
		}
		if _, exists := r.ClusterInfo.DistributionImages[instance.Spec.Distribution.Name]; !exists {
			return fmt.Errorf("failed to validate distribution: %s. Distribution name not supported", instance.Spec.Distribution.Name)
		}
	}

	return nil
}

// resolveImage determines the container image to use based on the distribution configuration.
// It returns the resolved image and any error encountered.
func (r *OGXServerReconciler) resolveImage(distribution ogxiov1beta1.DistributionSpec) (string, error) {
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

func buildPodDisruptionBudgetSpec(instance *ogxiov1beta1.OGXServer) *policyv1.PodDisruptionBudgetSpec {
	if !needsPodDisruptionBudget(instance) {
		return nil
	}

	spec := &policyv1.PodDisruptionBudgetSpec{}
	if instance.Spec.Workload != nil && instance.Spec.Workload.PodDisruptionBudget != nil {
		spec.MinAvailable = copyIntOrString(instance.Spec.Workload.PodDisruptionBudget.MinAvailable)
		spec.MaxUnavailable = copyIntOrString(instance.Spec.Workload.PodDisruptionBudget.MaxUnavailable)
	} else {
		// Fix for RHAIENG-3783: Use maxUnavailable instead of minAvailable
		// to avoid allowedDisruptions=0 with single-replica deployments
		maxUnavailable := intstr.FromInt(1)
		spec.MaxUnavailable = &maxUnavailable
	}

	return spec
}

func buildHPASpec(instance *ogxiov1beta1.OGXServer) *autoscalingv2.HorizontalPodAutoscalerSpec {
	if instance.Spec.Workload == nil || instance.Spec.Workload.Autoscaling == nil || instance.Spec.Workload.Autoscaling.MaxReplicas == 0 {
		return nil
	}
	auto := instance.Spec.Workload.Autoscaling
	minReplicas := resolveMinReplicas(auto.MinReplicas, deploy.GetEffectiveReplicas(instance))
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

func buildHPAMetrics(auto *ogxiov1beta1.AutoscalingSpec) []autoscalingv2.MetricSpec {
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

func needsPodDisruptionBudget(instance *ogxiov1beta1.OGXServer) bool {
	if instance.Spec.Workload != nil && instance.Spec.Workload.PodDisruptionBudget != nil {
		return true
	}
	return deploy.GetEffectiveReplicas(instance) > 1
}

func copyIntOrString(value *intstr.IntOrString) *intstr.IntOrString {
	if value == nil {
		return nil
	}
	copied := *value
	return &copied
}
