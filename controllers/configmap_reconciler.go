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
	"fmt"
	"sort"
	"time"

	"github.com/go-logr/logr"
	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/config"
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

// generatedConfigMapName returns the name for a generated ConfigMap.
func generatedConfigMapName(crName, contentHash string) string {
	return fmt.Sprintf("%s-config-%s", crName, contentHash)
}

// reconcileGeneratedConfig handles the full config generation lifecycle:
// 1. Generate config.yaml from spec + base config
// 2. Create/verify the ConfigMap with content-hash name
// 3. Clean up old ConfigMaps beyond retention limit
// Returns the generated config result, or nil if no config generation is needed.
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

	if err := r.cleanupOldGeneratedConfigMaps(ctx, instance, configMapName); err != nil {
		logger.Error(err, "failed to clean up old generated ConfigMaps")
	}

	return generated, nil
}

// resolveBaseConfig resolves the base config.yaml from OCI labels or embedded configs.
// Returns nil data (no error) when neither image nor distribution name is available.
func (r *OGXServerReconciler) resolveBaseConfig(ctx context.Context, instance *ogxiov1beta1.OGXServer) ([]byte, error) {
	logger := log.FromContext(ctx)

	distributionName := instance.Spec.Distribution.Name
	resolvedImage, resolveErr := r.resolveImage(instance.Spec.Distribution)

	if distributionName == "" && resolvedImage == "" {
		if resolveErr != nil {
			logger.V(1).Info("skipping generated config because distribution name and image are unset", "error", resolveErr)
		}
		return nil, nil
	}

	resolver := r.configResolver
	if resolver == nil {
		resolver = config.NewDefaultConfigResolver(r.OCILabelFetcher)
	}
	data, err := resolver.Resolve(resolvedImage, distributionName)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve base config: %w", err)
	}

	r.logConfigResolutionSource(logger, resolver, resolvedImage, distributionName)
	return data, nil
}

func (r *OGXServerReconciler) logConfigResolutionSource(logger logr.Logger, resolver config.ConfigResolver, resolvedImage, distributionName string) {
	source := "embedded"
	if dr, ok := resolver.(*config.DefaultConfigResolver); ok {
		if ociErr := dr.LastOCIError(); ociErr != nil {
			logger.V(1).Info("OCI config resolution failed, using embedded fallback", "error", ociErr, "image", resolvedImage)
		} else if resolvedImage != "" && r.OCILabelFetcher != nil {
			source = "oci"
		}
	}
	logger.V(1).Info("resolved base config", "distribution", distributionName, "image", resolvedImage, "source", source)
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

	// Sort by creation timestamp (oldest first)
	sort.Slice(cmList.Items, func(i, j int) bool {
		return cmList.Items[i].CreationTimestamp.Before(&cmList.Items[j].CreationTimestamp)
	})

	// Filter out the current ConfigMap before computing the delete set so that
	// skipping it doesn't leave more ConfigMaps than the retention limit.
	var candidates []corev1.ConfigMap
	for i := range cmList.Items {
		if cmList.Items[i].Name != currentName {
			candidates = append(candidates, cmList.Items[i])
		}
	}

	// +1 because currentName (excluded above) also counts toward retention
	deleteCount := len(candidates) - (configMapRetention - 1)
	for i := 0; i < deleteCount; i++ {
		cm := &candidates[i]
		if err := r.Delete(ctx, cm); err != nil && !k8serrors.IsNotFound(err) {
			logger.Error(err, "failed to delete old generated ConfigMap", "name", cm.Name)
		} else {
			logger.V(1).Info("deleted old generated ConfigMap", "name", cm.Name)
		}
	}

	return nil
}

// getGeneratedConfigMapHash returns a hash for the generated ConfigMap for rollout detection.
func (r *OGXServerReconciler) getGeneratedConfigMapHash(ctx context.Context, instance *ogxiov1beta1.OGXServer) (string, error) {
	if instance.Status.ConfigGeneration == nil || instance.Status.ConfigGeneration.ConfigMapName == "" {
		return "", nil
	}
	cm := &corev1.ConfigMap{}
	err := r.directGet(ctx, client.ObjectKey{
		Name:      instance.Status.ConfigGeneration.ConfigMapName,
		Namespace: instance.Namespace,
	}, cm)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("%s-%s", cm.ResourceVersion, cm.Name), nil
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
