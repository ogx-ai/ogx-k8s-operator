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
	"sort"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/config"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// generatedConfigLabel identifies operator-generated ConfigMaps.
	generatedConfigLabel = "ogx.io/generated-config"
	// configMapRetention is the number of generated ConfigMaps to retain.
	configMapRetention = 2
	// generatedConfigKeyName is the key used in the generated ConfigMap data.
	generatedConfigKeyName = "config.yaml"
)

var errReferencedConfigMapKeyNotFound = errors.New("failed to find referenced ConfigMap key")

// generatedConfigMapName returns the name for a generated ConfigMap.
func generatedConfigMapName(crName, contentHash string) string {
	return fmt.Sprintf("%s-config-%s", crName, contentHash)
}

// reconcileGeneratedConfig handles the full config generation lifecycle:
// 1. Generate config.yaml from spec + base config
// 2. Create/verify the ConfigMap with content-hash name
// Returns the generated config result, or nil if no config generation is needed.
// Cleanup of old generated ConfigMaps happens after the Deployment reconcile
// succeeds so in-flight rollouts keep their referenced inputs.
func (r *OGXServerReconciler) reconcileGeneratedConfig(ctx context.Context, instance *ogxiov1beta1.OGXServer) (*config.GeneratedConfig, error) {
	logger := log.FromContext(ctx)

	if instance.HasOverrideConfig() || !instance.HasDeclarativeConfig() {
		return nil, nil
	}

	baseConfigData, err := r.resolveBaseConfig(ctx, instance)
	if err != nil {
		return nil, err
	}
	if baseConfigData == nil {
		return nil, nil
	}

	validateErr := config.ValidateSecretRefEnvVarNames(&instance.Spec)
	if validateErr != nil {
		return nil, fmt.Errorf("failed to validate secret env var mapping: %w", validateErr)
	}

	generated, err := config.GenerateConfig(&instance.Spec, baseConfigData)
	if err != nil {
		return nil, fmt.Errorf("failed to generate config: %w", err)
	}

	if generated.ConfigVersionDefaulted {
		logger.Info("base config has non-numeric version, defaulting to version 2", "rawVersion", "non-numeric")
	}

	configMapName := generatedConfigMapName(instance.Name, generated.ContentHash)
	if err := r.ensureGeneratedConfigMap(ctx, instance, configMapName, generated.ConfigYAML); err != nil {
		return nil, err
	}

	return generated, nil
}

// resolveBaseConfig resolves the base config.yaml from a referenced ConfigMap or
// OCI labels on the resolved distribution image.
func (r *OGXServerReconciler) resolveBaseConfig(ctx context.Context, instance *ogxiov1beta1.OGXServer) ([]byte, error) {
	logger := log.FromContext(ctx)

	if ref := instance.Spec.BaseConfig; ref != nil {
		data, err := r.readReferencedConfigMapKey(ctx, instance.Namespace, *ref)
		if err != nil {
			return nil, &terminalError{message: fmt.Sprintf("failed to resolve base config from ConfigMap %s/%s[%s]: %v", instance.Namespace, ref.Name, ref.Key, err)}
		}
		logger.V(1).Info("resolved base config", "source", "configmap", "configMap", ref.Name, "key", ref.Key)
		return data, nil
	}

	distributionName := instance.Spec.Distribution.Name
	resolvedImage, resolveErr := r.resolveImage(instance.Spec.Distribution)
	if resolveErr != nil {
		return nil, fmt.Errorf("failed to resolve distribution image: %w", resolveErr)
	}

	resolver := r.configResolver
	if resolver == nil {
		resolver = config.NewDefaultConfigResolver(r.OCILabelFetcher)
	}
	data, err := resolver.Resolve(resolvedImage, distributionName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base config from OCI labels: %w", err)
	}

	logger.V(1).Info("resolved base config", "distribution", distributionName, "image", resolvedImage, "source", "oci")
	return data, nil
}

func (r *OGXServerReconciler) readReferencedConfigMapKey(
	ctx context.Context, namespace string, ref ogxiov1beta1.ConfigMapKeyRef,
) ([]byte, error) {
	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, client.ObjectKey{Name: ref.Name, Namespace: namespace}, configMap); err != nil {
		return nil, err
	}

	data, ok := configMap.Data[ref.Key]
	if !ok {
		return nil, fmt.Errorf("failed to find referenced ConfigMap key %q: %w", ref.Key, errReferencedConfigMapKeyNotFound)
	}
	return []byte(data), nil
}

// ensureGeneratedConfigMap creates the generated ConfigMap if it doesn't already exist.
func (r *OGXServerReconciler) ensureGeneratedConfigMap(ctx context.Context, instance *ogxiov1beta1.OGXServer, name, configYAML string) error {
	logger := log.FromContext(ctx)

	existingCM := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: instance.Namespace}, existingCM)
	if err == nil {
		if existingCM.Data[generatedConfigKeyName] != configYAML {
			return fmt.Errorf("failed to verify generated ConfigMap content for %q: hash collision detected", name)
		}
		return nil
	}
	if !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to check for existing generated ConfigMap: %w", err)
	}

	cm := r.buildGeneratedConfigMap(instance, name, configYAML)
	if err := ctrl.SetControllerReference(instance, cm, r.Scheme); err != nil {
		return fmt.Errorf("failed to set owner reference on generated ConfigMap: %w", err)
	}
	if err := r.Create(ctx, cm); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			logger.V(1).Info("generated ConfigMap already exists (concurrent reconcile)", "name", name)
			return nil
		}
		return fmt.Errorf("failed to create generated ConfigMap: %w", err)
	}
	logger.Info("created generated ConfigMap", "name", name)
	return nil
}

func (r *OGXServerReconciler) buildGeneratedConfigMap(instance *ogxiov1beta1.OGXServer, name, configYAML string) *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				managedByLabelKey:            managedByLabelVal,
				"app.kubernetes.io/instance": instance.Name,
				generatedConfigLabel:         "true",
				WatchLabelKey:                WatchLabelValue,
			},
		},
		Immutable: boolPtr(true),
		Data: map[string]string{
			generatedConfigKeyName: configYAML,
		},
	}
}

// cleanupOldGeneratedConfigMaps deletes generated ConfigMaps beyond the retention limit.
//
//nolint:cyclop // Orchestrates protection, retention, and deletion rules for generated ConfigMaps.
func (r *OGXServerReconciler) cleanupOldGeneratedConfigMaps(ctx context.Context, instance *ogxiov1beta1.OGXServer, currentName string) error {
	logger := log.FromContext(ctx)

	// List all generated ConfigMaps for this instance
	cmList := &corev1.ConfigMapList{}
	selector := labels.SelectorFromSet(map[string]string{
		generatedConfigLabel:         "true",
		"app.kubernetes.io/instance": instance.Name,
	})
	if err := r.List(ctx, cmList, &client.ListOptions{
		Namespace:     instance.Namespace,
		LabelSelector: selector,
	}); err != nil {
		return fmt.Errorf("failed to list generated ConfigMaps: %w", err)
	}

	if len(cmList.Items) <= configMapRetention {
		return nil
	}

	protectedNames, err := r.referencedRuntimeConfigMapNames(ctx, instance, currentName)
	if err != nil {
		return fmt.Errorf("failed to determine referenced generated ConfigMaps: %w", err)
	}

	protectedCount := 0
	for i := range cmList.Items {
		if _, ok := protectedNames[cmList.Items[i].Name]; ok {
			protectedCount++
		}
	}

	// Sort by creation timestamp (oldest first)
	sort.Slice(cmList.Items, func(i, j int) bool {
		return cmList.Items[i].CreationTimestamp.Before(&cmList.Items[j].CreationTimestamp)
	})

	minimumToKeep := configMapRetention
	if protectedCount > minimumToKeep {
		minimumToKeep = protectedCount
	}

	deleteCount := len(cmList.Items) - minimumToKeep
	for i := range cmList.Items {
		if deleteCount == 0 {
			break
		}
		cm := &cmList.Items[i]
		if _, ok := protectedNames[cm.Name]; ok {
			continue
		}
		if err := r.Delete(ctx, cm); err != nil && !k8serrors.IsNotFound(err) {
			logger.Error(err, "failed to delete old generated ConfigMap", "name", cm.Name)
		} else {
			logger.V(1).Info("deleted old generated ConfigMap", "name", cm.Name)
			deleteCount--
		}
	}

	return nil
}

func (r *OGXServerReconciler) getGeneratedConfigMapHashByName(ctx context.Context, namespace, name string) (string, error) {
	cm := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{
		Name:      name,
		Namespace: namespace,
	}, cm)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", cm.ResourceVersion, cm.Name), nil
}

func (r *OGXServerReconciler) referencedRuntimeConfigMapNames(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	currentName string,
) (map[string]struct{}, error) {
	protected := map[string]struct{}{currentName: {}}
	if instance.Status.ConfigGeneration != nil && instance.Status.ConfigGeneration.ConfigMapName != "" {
		protected[instance.Status.ConfigGeneration.ConfigMapName] = struct{}{}
	}

	deployment := &appsv1.Deployment{}
	if err := r.Get(ctx, client.ObjectKey{Name: instance.Name, Namespace: instance.Namespace}, deployment); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, err
		}
	} else {
		collectRuntimeConfigMapRefs(protected, deployment.Spec.Template.Spec.Volumes)
	}

	rsList := &appsv1.ReplicaSetList{}
	if err := r.List(
		ctx,
		rsList,
		client.InNamespace(instance.Namespace),
		client.MatchingLabels{instanceLabelKey: instance.Name},
	); err != nil {
		return nil, err
	}
	for i := range rsList.Items {
		collectRuntimeConfigMapRefs(protected, rsList.Items[i].Spec.Template.Spec.Volumes)
	}

	return protected, nil
}

func collectRuntimeConfigMapRefs(protected map[string]struct{}, volumes []corev1.Volume) {
	for i := range volumes {
		if volumes[i].Name != "user-config" || volumes[i].ConfigMap == nil {
			continue
		}
		if name := volumes[i].ConfigMap.Name; name != "" {
			protected[name] = struct{}{}
		}
	}
}

// updateConfigGenerationStatus updates the status with config generation details.
func (r *OGXServerReconciler) updateConfigGenerationStatus(instance *ogxiov1beta1.OGXServer, generated *config.GeneratedConfig) {
	if generated == nil {
		return
	}
	configMapName := generatedConfigMapName(instance.Name, generated.ContentHash)
	instance.Status.ConfigGeneration = &ogxiov1beta1.ConfigGenerationStatus{
		ObservedGeneration: instance.Generation,
		ConfigMapName:      configMapName,
		GeneratedAt:        metav1.Time{Time: time.Now()},
		ProviderCount:      generated.ProviderCount,
		ResourceCount:      generated.ResourceCount,
		ConfigVersion:      generated.ConfigVersion,
	}
}

// clearConfigGenerationStatus clears generated config status and sets condition false.
// Always sets the condition to reflect inactive/failed generation state.
func (r *OGXServerReconciler) clearConfigGenerationStatus(instance *ogxiov1beta1.OGXServer, reason, message string) {
	instance.Status.ConfigGeneration = nil
	r.setConfigGeneratedCondition(instance, false, reason, message)
}

func boolPtr(b bool) *bool {
	return &b
}
