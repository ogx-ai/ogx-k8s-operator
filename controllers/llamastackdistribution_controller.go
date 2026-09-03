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
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	llamav1alpha1 "github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
	"github.com/llamastack/llama-stack-k8s-operator/pkg/cluster"
	"github.com/llamastack/llama-stack-k8s-operator/pkg/deploy"
	"github.com/llamastack/llama-stack-k8s-operator/pkg/featureflags"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/utils/ptr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const (
	operatorConfigData = "llama-stack-operator-config"
	manifestsBasePath  = "manifests/base"

	// CA Bundle related constants.
	DefaultCABundleKey    = "ca-bundle.crt"
	CABundleMountPath     = "/etc/ssl/certs/ca-bundle.crt"
	CABundleTempPath      = "/tmp/ca-bundle/ca-bundle.crt"
	CABundleVolumeName    = "ca-bundle"
	CABundleSourceDir     = "/tmp/ca-source"
	CABundleInitName      = "ca-bundle-init"
	CABundleSourceVolName = "ca-bundle-source"
	CABundleTempDir       = "/tmp/ca-bundle"

	// ODH/RHOAI well-known ConfigMap for trusted CA bundles.
	odhTrustedCABundleConfigMap = "odh-trusted-ca-bundle"
)

// LlamaStackDistributionReconciler reconciles a LlamaStack object.
//
// ConfigMap Watching Feature:
// This reconciler watches for changes to ConfigMaps referenced by LlamaStackDistribution CRs.
// When a ConfigMap's data changes, it automatically triggers reconciliation of the referencing
// LlamaStackDistribution, which recalculates a content-based hash and updates the deployment's
// pod template annotations. This causes Kubernetes to restart the pods with the updated configuration.
type LlamaStackDistributionReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Feature flags
	EnableNetworkPolicy bool
	// Cluster info
	ClusterInfo *cluster.ClusterInfo
	httpClient  *http.Client
}

// hasUserConfigMap checks if the instance has a valid UserConfig with ConfigMapName.
// Returns true if configured, false otherwise.
func (r *LlamaStackDistributionReconciler) hasUserConfigMap(instance *llamav1alpha1.LlamaStackDistribution) bool {
	return instance.Spec.Server.UserConfig != nil && instance.Spec.Server.UserConfig.ConfigMapName != ""
}

// getUserConfigMapNamespace returns the resolved ConfigMap namespace.
// If ConfigMapNamespace is specified, it returns that; otherwise, it returns the instance's namespace.
func (r *LlamaStackDistributionReconciler) getUserConfigMapNamespace(instance *llamav1alpha1.LlamaStackDistribution) string {
	if instance.Spec.Server.UserConfig.ConfigMapNamespace != "" {
		return instance.Spec.Server.UserConfig.ConfigMapNamespace
	}
	return instance.Namespace
}

// hasCABundleConfigMap checks if the instance has a valid TLSConfig with CABundle ConfigMapName.
// Returns true if configured, false otherwise.
func (r *LlamaStackDistributionReconciler) hasCABundleConfigMap(instance *llamav1alpha1.LlamaStackDistribution) bool {
	return instance.Spec.Server.TLSConfig != nil && instance.Spec.Server.TLSConfig.CABundle != nil && instance.Spec.Server.TLSConfig.CABundle.ConfigMapName != ""
}

// getCABundleConfigMapNamespace returns the resolved CA bundle ConfigMap namespace.
// If ConfigMapNamespace is specified, it returns that; otherwise, it returns the instance's namespace.
func (r *LlamaStackDistributionReconciler) getCABundleConfigMapNamespace(instance *llamav1alpha1.LlamaStackDistribution) string {
	if instance.Spec.Server.TLSConfig.CABundle.ConfigMapNamespace != "" {
		return instance.Spec.Server.TLSConfig.CABundle.ConfigMapNamespace
	}
	return instance.Namespace
}

// hasValidUserConfig is a standalone helper function to check if a LlamaStackDistribution has valid UserConfig.
// This is used by functions that don't have access to the reconciler receiver.
func hasValidUserConfig(llsd *llamav1alpha1.LlamaStackDistribution) bool {
	return llsd.Spec.Server.UserConfig != nil && llsd.Spec.Server.UserConfig.ConfigMapName != ""
}

// getUserConfigMapNamespaceStandalone returns the resolved ConfigMap namespace without needing a receiver.
func getUserConfigMapNamespaceStandalone(llsd *llamav1alpha1.LlamaStackDistribution) string {
	if llsd.Spec.Server.UserConfig.ConfigMapNamespace != "" {
		return llsd.Spec.Server.UserConfig.ConfigMapNamespace
	}
	return llsd.Namespace
}

// hasValidCABundleConfig is a standalone helper function to check if a LlamaStackDistribution has valid CA bundle config.
// This is used by functions that don't have access to the reconciler receiver.
func hasValidCABundleConfig(llsd *llamav1alpha1.LlamaStackDistribution) bool {
	return llsd.Spec.Server.TLSConfig != nil && llsd.Spec.Server.TLSConfig.CABundle != nil && llsd.Spec.Server.TLSConfig.CABundle.ConfigMapName != ""
}

// getCABundleConfigMapNamespaceStandalone returns the resolved CA bundle ConfigMap namespace without needing a receiver.
func getCABundleConfigMapNamespaceStandalone(llsd *llamav1alpha1.LlamaStackDistribution) string {
	if llsd.Spec.Server.TLSConfig.CABundle.ConfigMapNamespace != "" {
		return llsd.Spec.Server.TLSConfig.CABundle.ConfigMapNamespace
	}
	return llsd.Namespace
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the LlamaStack object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *LlamaStackDistributionReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Create a logger with request-specific values and store it in the context.
	// This ensures consistent logging across the reconciliation process and its sub-functions.
	// The logger is retrieved from the context in each sub-function that needs it, maintaining
	// the request-specific values throughout the call chain.
	// Always ensure the name of the CR and the namespace are included in the logger.
	logger := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
	ctx = logr.NewContext(ctx, logger)

	// Fetch the LlamaStack instance
	instance, err := r.fetchInstance(ctx, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if instance == nil {
		logger.Info("LlamaStackDistribution resource not found, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Reconcile all resources, storing the error for later.
	reconcileErr := r.reconcileResources(ctx, instance)

	// Update the status, passing in any reconciliation error.
	if statusUpdateErr := r.updateStatus(ctx, instance, reconcileErr); statusUpdateErr != nil {
		// Log the status update error, but prioritize the reconciliation error for return.
		logger.Error(statusUpdateErr, "failed to update status")
		if reconcileErr != nil {
			return ctrl.Result{}, reconcileErr
		}
		return ctrl.Result{}, statusUpdateErr
	}

	// If reconciliation failed, return the error to trigger a requeue.
	if reconcileErr != nil {
		return ctrl.Result{}, reconcileErr
	}

	// Check if requeue is needed based on phase
	if instance.Status.Phase == llamav1alpha1.LlamaStackDistributionPhaseInitializing {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger.Info("Successfully reconciled LlamaStackDistribution")
	return ctrl.Result{}, nil
}

// fetchInstance retrieves the LlamaStackDistribution instance.
func (r *LlamaStackDistributionReconciler) fetchInstance(ctx context.Context, namespacedName types.NamespacedName) (*llamav1alpha1.LlamaStackDistribution, error) {
	logger := log.FromContext(ctx)
	instance := &llamav1alpha1.LlamaStackDistribution{}
	if err := r.Get(ctx, namespacedName, instance); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("failed to find LlamaStackDistribution resource")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch LlamaStackDistribution: %w", err)
	}
	return instance, nil
}

// determineKindsToExclude returns a list of resource kinds that should be excluded
// based on the instance specification.
func (r *LlamaStackDistributionReconciler) determineKindsToExclude(instance *llamav1alpha1.LlamaStackDistribution) []string {
	var kinds []string

	// Exclude PersistentVolumeClaim if storage is not configured
	if instance.Spec.Server.Storage == nil {
		kinds = append(kinds, "PersistentVolumeClaim")
	}

	// Exclude NetworkPolicy if the feature is disabled
	if !r.EnableNetworkPolicy {
		kinds = append(kinds, "NetworkPolicy")
	}

	// Exclude Service if no ports are defined
	if !instance.HasPorts() {
		kinds = append(kinds, "Service")
	}

	return kinds
}

// reconcileManifestResources applies resources that are managed by the operator
// based on the instance specification.
func (r *LlamaStackDistributionReconciler) reconcileManifestResources(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) error {
	resMap, err := deploy.RenderManifest(filesys.MakeFsOnDisk(), manifestsBasePath, instance)
	if err != nil {
		return fmt.Errorf("failed to render manifests: %w", err)
	}

	kindsToExclude := r.determineKindsToExclude(instance)
	filteredResMap, err := deploy.FilterExcludeKinds(resMap, kindsToExclude)
	if err != nil {
		return fmt.Errorf("failed to filter manifests: %w", err)
	}

	if err := deploy.ApplyResources(ctx, r.Client, r.Scheme, instance, filteredResMap); err != nil {
		return fmt.Errorf("failed to apply manifests: %w", err)
	}

	return nil
}

// reconcileResources reconciles all resources for the LlamaStackDistribution instance.
func (r *LlamaStackDistributionReconciler) reconcileResources(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) error {
	// Reconcile ConfigMaps
	if err := r.reconcileConfigMaps(ctx, instance); err != nil {
		return err
	}

	// Reconcile storage
	if err := r.reconcileStorage(ctx, instance); err != nil {
		return err
	}

	// Reconcile manifest-based resources
	if err := r.reconcileManifestResources(ctx, instance); err != nil {
		return err
	}

	// Reconcile the NetworkPolicy
	if err := r.reconcileNetworkPolicy(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile NetworkPolicy: %w", err)
	}

	// Reconcile the Deployment
	if err := r.reconcileDeployment(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile Deployment: %w", err)
	}

	return nil
}

func (r *LlamaStackDistributionReconciler) reconcileConfigMaps(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) error {
	// Reconcile the ConfigMap if specified by the user
	if r.hasUserConfigMap(instance) {
		if err := r.reconcileUserConfigMap(ctx, instance); err != nil {
			return fmt.Errorf("failed to reconcile user ConfigMap: %w", err)
		}
	}

	// Reconcile the CA bundle ConfigMap if specified
	if r.hasCABundleConfigMap(instance) {
		if err := r.reconcileCABundleConfigMap(ctx, instance); err != nil {
			return fmt.Errorf("failed to reconcile CA bundle ConfigMap: %w", err)
		}
	}

	return nil
}

func (r *LlamaStackDistributionReconciler) reconcileStorage(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) error {
	// Reconcile the PVC if storage is configured
	if instance.Spec.Server.Storage != nil {
		resMap, err := deploy.RenderManifest(filesys.MakeFsOnDisk(), manifestsBasePath, instance)
		if err != nil {
			return fmt.Errorf("failed to render PVC manifests: %w", err)
		}
		if err := deploy.ApplyResources(ctx, r.Client, r.Scheme, instance, resMap); err != nil {
			return fmt.Errorf("failed to apply PVC manifests: %w", err)
		}
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *LlamaStackDistributionReconciler) SetupWithManager(ctx context.Context, mgr ctrl.Manager) error {
	// Create a field indexer for ConfigMap references to improve performance
	if err := r.createConfigMapFieldIndexer(ctx, mgr); err != nil {
		return err
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(&llamav1alpha1.LlamaStackDistribution{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: r.llamaStackUpdatePredicate(mgr),
		})).
		Owns(&appsv1.Deployment{}).
		Owns(&corev1.Service{}).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.findLlamaStackDistributionsForConfigMap),
			builder.WithPredicates(predicate.Funcs{
				UpdateFunc: r.configMapUpdatePredicate,
				CreateFunc: r.configMapCreatePredicate,
				DeleteFunc: r.configMapDeletePredicate,
			}),
		).
		Complete(r)
}

// createConfigMapFieldIndexer creates a field indexer for ConfigMap references.
// On older Kubernetes versions that don't support custom field labels for custom resources,
// this will fail gracefully and the operator will fall back to manual searching.
func (r *LlamaStackDistributionReconciler) createConfigMapFieldIndexer(ctx context.Context, mgr ctrl.Manager) error {
	// Create index for user config ConfigMaps
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&llamav1alpha1.LlamaStackDistribution{},
		"spec.server.userConfig.configMapName",
		r.configMapIndexFunc,
	); err != nil {
		// Log warning but don't fail startup - older Kubernetes versions may not support this
		mgr.GetLogger().V(1).Info("Field indexer for ConfigMap references not supported, will use manual search fallback",
			"error", err.Error())
		return nil
	}

	// Create index for CA bundle ConfigMaps
	if err := mgr.GetFieldIndexer().IndexField(
		ctx,
		&llamav1alpha1.LlamaStackDistribution{},
		"spec.server.tlsConfig.caBundle.configMapName",
		r.caBundleConfigMapIndexFunc,
	); err != nil {
		// Log warning but don't fail startup - older Kubernetes versions may not support this
		mgr.GetLogger().Info("Field indexer for CA bundle ConfigMap references not supported, will use manual search fallback",
			"error", err.Error())
		return nil
	}

	mgr.GetLogger().V(1).Info("Successfully created field indexer for ConfigMap references - will use efficient lookups")
	return nil
}

// configMapIndexFunc is the indexer function for ConfigMap references.
func (r *LlamaStackDistributionReconciler) configMapIndexFunc(rawObj client.Object) []string {
	llsd, ok := rawObj.(*llamav1alpha1.LlamaStackDistribution)
	if !ok {
		return nil
	}
	if !hasValidUserConfig(llsd) {
		return nil
	}

	// Create index key as "namespace/name" format
	configMapNamespace := getUserConfigMapNamespaceStandalone(llsd)
	indexKey := fmt.Sprintf("%s/%s", configMapNamespace, llsd.Spec.Server.UserConfig.ConfigMapName)
	return []string{indexKey}
}

// caBundleConfigMapIndexFunc is the indexer function for CA bundle ConfigMap references.
func (r *LlamaStackDistributionReconciler) caBundleConfigMapIndexFunc(rawObj client.Object) []string {
	llsd, ok := rawObj.(*llamav1alpha1.LlamaStackDistribution)
	if !ok {
		return nil
	}
	if !hasValidCABundleConfig(llsd) {
		return nil
	}

	// Create index key as "namespace/name" format
	configMapNamespace := getCABundleConfigMapNamespaceStandalone(llsd)
	indexKey := fmt.Sprintf("%s/%s", configMapNamespace, llsd.Spec.Server.TLSConfig.CABundle.ConfigMapName)
	return []string{indexKey}
}

// llamaStackUpdatePredicate returns a predicate function for LlamaStackDistribution updates.
func (r *LlamaStackDistributionReconciler) llamaStackUpdatePredicate(mgr ctrl.Manager) func(event.UpdateEvent) bool {
	return func(e event.UpdateEvent) bool {
		// Safely type assert old object
		oldObj, ok := e.ObjectOld.(*llamav1alpha1.LlamaStackDistribution)
		if !ok {
			return false
		}
		oldObjCopy := oldObj.DeepCopy()

		// Safely type assert new object
		newObj, ok := e.ObjectNew.(*llamav1alpha1.LlamaStackDistribution)
		if !ok {
			return false
		}
		newObjCopy := newObj.DeepCopy()

		// Compare only spec, ignoring metadata and status
		if diff := cmp.Diff(oldObjCopy.Spec, newObjCopy.Spec); diff != "" {
			logger := mgr.GetLogger().WithValues("namespace", newObjCopy.Namespace, "name", newObjCopy.Name)
			logger.Info("LlamaStackDistribution CR spec changed")
			// Note that both the logger and fmt.Printf could appear entangled in the output
			// but there is no simple way to avoid this (forcing the logger to flush its output).
			// When the logger is used to print the diff the output is hard to read,
			// fmt.Printf is better for readability.
			fmt.Printf("%s\n", diff)
		}

		return true
	}
}

// configMapUpdatePredicate handles ConfigMap update events.
func (r *LlamaStackDistributionReconciler) configMapUpdatePredicate(e event.UpdateEvent) bool {
	oldConfigMap, oldOk := e.ObjectOld.(*corev1.ConfigMap)
	newConfigMap, newOk := e.ObjectNew.(*corev1.ConfigMap)

	if !oldOk || !newOk {
		return false
	}

	// Only proceed if this ConfigMap is referenced by any LlamaStackDistribution
	if !r.isConfigMapReferenced(newConfigMap) {
		return false
	}

	// Only trigger if Data or BinaryData has changed
	dataChanged := !cmp.Equal(oldConfigMap.Data, newConfigMap.Data)
	binaryDataChanged := !cmp.Equal(oldConfigMap.BinaryData, newConfigMap.BinaryData)

	// Log ConfigMap changes for debugging (only for referenced ConfigMaps)
	if dataChanged || binaryDataChanged {
		r.logConfigMapDiff(oldConfigMap, newConfigMap, dataChanged, binaryDataChanged)
	}

	return dataChanged || binaryDataChanged
}

// logConfigMapDiff logs the differences between old and new ConfigMaps.
func (r *LlamaStackDistributionReconciler) logConfigMapDiff(oldConfigMap, newConfigMap *corev1.ConfigMap, dataChanged, binaryDataChanged bool) {
	logger := log.FromContext(context.Background()).WithValues(
		"configMapName", newConfigMap.Name,
		"configMapNamespace", newConfigMap.Namespace)

	logger.Info("Referenced ConfigMap change detected")

	if dataChanged {
		if dataDiff := cmp.Diff(oldConfigMap.Data, newConfigMap.Data); dataDiff != "" {
			logger.Info("ConfigMap Data changed")
			fmt.Printf("ConfigMap %s/%s Data diff:\n%s\n", newConfigMap.Namespace, newConfigMap.Name, dataDiff)
		}
	}

	if binaryDataChanged {
		if binaryDataDiff := cmp.Diff(oldConfigMap.BinaryData, newConfigMap.BinaryData); binaryDataDiff != "" {
			logger.Info("ConfigMap BinaryData changed")
			fmt.Printf("ConfigMap %s/%s BinaryData diff:\n%s\n", newConfigMap.Namespace, newConfigMap.Name, binaryDataDiff)
		}
	}
}

// configMapCreatePredicate handles ConfigMap create events.
func (r *LlamaStackDistributionReconciler) configMapCreatePredicate(e event.CreateEvent) bool {
	configMap, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		return false
	}

	isReferenced := r.isConfigMapReferenced(configMap)
	// Log create events for referenced ConfigMaps
	if isReferenced {
		log.FromContext(context.Background()).Info("ConfigMap create event detected for referenced ConfigMap",
			"configMapName", configMap.Name,
			"configMapNamespace", configMap.Namespace)
	}

	return isReferenced
}

// configMapDeletePredicate handles ConfigMap delete events.
func (r *LlamaStackDistributionReconciler) configMapDeletePredicate(e event.DeleteEvent) bool {
	configMap, ok := e.Object.(*corev1.ConfigMap)
	if !ok {
		return false
	}

	isReferenced := r.isConfigMapReferenced(configMap)
	// Log delete events for referenced ConfigMaps - this is critical for deployment health
	if isReferenced {
		log.FromContext(context.Background()).Error(nil,
			"CRITICAL: ConfigMap delete event detected for referenced ConfigMap - this will break dependent deployments",
			"configMapName", configMap.Name,
			"configMapNamespace", configMap.Namespace)
	}

	return isReferenced
}

// isConfigMapReferenced checks if a ConfigMap is referenced by any LlamaStackDistribution.
func (r *LlamaStackDistributionReconciler) isConfigMapReferenced(configMap client.Object) bool {
	logger := log.FromContext(context.Background()).WithValues(
		"configMapName", configMap.GetName(),
		"configMapNamespace", configMap.GetNamespace())

	// Use field indexer for efficient lookup - create the same index key format
	indexKey := fmt.Sprintf("%s/%s", configMap.GetNamespace(), configMap.GetName())

	// Check for user config ConfigMap references
	userConfigLlamaStacks := llamav1alpha1.LlamaStackDistributionList{}
	err := r.List(context.Background(), &userConfigLlamaStacks, client.MatchingFields{"spec.server.userConfig.configMapName": indexKey})
	if err != nil {
		// Field indexer failed (likely due to older Kubernetes version not supporting custom field labels)
		// Fall back to a manual check instead of assuming all ConfigMaps are referenced
		logger.V(1).Info("Field indexer not supported, falling back to manual ConfigMap reference check", "error", err.Error())
		return r.manuallyCheckConfigMapReference(configMap)
	}

	found := len(userConfigLlamaStacks.Items) > 0

	// Check for CA bundle ConfigMap references if not found in user config
	if !found {
		caBundleLlamaStacks := llamav1alpha1.LlamaStackDistributionList{}
		err := r.List(context.Background(), &caBundleLlamaStacks, client.MatchingFields{"spec.server.tlsConfig.caBundle.configMapName": indexKey})
		if err != nil {
			// Field indexer failed for CA bundle, fall back to manual check
			logger.Info("CA bundle field indexer not supported, falling back to manual ConfigMap reference check", "error", err.Error())
			return r.manuallyCheckConfigMapReference(configMap)
		}
		found = len(caBundleLlamaStacks.Items) > 0
	}

	if !found {
		// Fallback: manually check all LlamaStackDistributions
		manuallyFound := r.manuallyCheckConfigMapReference(configMap)
		if manuallyFound {
			return true
		}
	}

	return found
}

// manuallyCheckConfigMapReference manually checks if any LlamaStackDistribution references the given ConfigMap.
func (r *LlamaStackDistributionReconciler) manuallyCheckConfigMapReference(configMap client.Object) bool {
	logger := log.FromContext(context.Background()).WithValues(
		"configMapName", configMap.GetName(),
		"configMapNamespace", configMap.GetNamespace())

	allLlamaStacks := llamav1alpha1.LlamaStackDistributionList{}
	err := r.List(context.Background(), &allLlamaStacks)
	if err != nil {
		logger.Error(err, "CRITICAL: Failed to list all LlamaStackDistributions for manual ConfigMap reference check - assuming ConfigMap is referenced")
		return true // Return true to trigger reconciliation when we can't determine reference status
	}

	targetNamespace := configMap.GetNamespace()
	targetName := configMap.GetName()

	for _, ls := range allLlamaStacks.Items {
		// Check user config ConfigMap references
		if hasValidUserConfig(&ls) {
			configMapNamespace := getUserConfigMapNamespaceStandalone(&ls)

			if configMapNamespace == targetNamespace && ls.Spec.Server.UserConfig.ConfigMapName == targetName {
				// found a LlamaStackDistribution that references the ConfigMap
				return true
			}
		}

		// Check CA bundle ConfigMap references
		if hasValidCABundleConfig(&ls) {
			configMapNamespace := getCABundleConfigMapNamespaceStandalone(&ls)

			if configMapNamespace == targetNamespace && ls.Spec.Server.TLSConfig.CABundle.ConfigMapName == targetName {
				// found a LlamaStackDistribution that references the CA bundle ConfigMap
				return true
			}
		}
	}

	// no LlamaStackDistribution found that references the ConfigMap
	return false
}

// findLlamaStackDistributionsForConfigMap maps ConfigMap changes to LlamaStackDistribution reconcile requests.
func (r *LlamaStackDistributionReconciler) findLlamaStackDistributionsForConfigMap(ctx context.Context, configMap client.Object) []reconcile.Request {
	// Try field indexer lookup first
	attachedLlamaStacks, found := r.tryFieldIndexerLookup(ctx, configMap)
	if !found {
		// Fallback to manual search if field indexer returns no results
		attachedLlamaStacks = r.performManualSearch(ctx, configMap)
	}

	// Convert to reconcile requests
	requests := r.convertToReconcileRequests(attachedLlamaStacks)

	return requests
}

// tryFieldIndexerLookup attempts to find LlamaStackDistributions using the field indexer.
func (r *LlamaStackDistributionReconciler) tryFieldIndexerLookup(ctx context.Context, configMap client.Object) (llamav1alpha1.LlamaStackDistributionList, bool) {
	logger := log.FromContext(ctx).WithValues(
		"configMapName", configMap.GetName(),
		"configMapNamespace", configMap.GetNamespace())

	indexKey := fmt.Sprintf("%s/%s", configMap.GetNamespace(), configMap.GetName())

	// Check for user config ConfigMap references
	userConfigLlamaStacks := llamav1alpha1.LlamaStackDistributionList{}
	err := r.List(ctx, &userConfigLlamaStacks, client.MatchingFields{"spec.server.userConfig.configMapName": indexKey})
	if err != nil {
		logger.V(1).Info("Field indexer not supported, will fall back to a manual search for ConfigMap event processing",
			"indexKey", indexKey, "error", err.Error())
		return userConfigLlamaStacks, false
	}

	// Check for CA bundle ConfigMap references
	caBundleLlamaStacks := llamav1alpha1.LlamaStackDistributionList{}
	err = r.List(ctx, &caBundleLlamaStacks, client.MatchingFields{"spec.server.tlsConfig.caBundle.configMapName": indexKey})
	if err != nil {
		logger.Info("CA bundle field indexer not supported, will fall back to a manual search for ConfigMap event processing",
			"indexKey", indexKey, "error", err.Error())
		return userConfigLlamaStacks, len(userConfigLlamaStacks.Items) > 0
	}

	// Combine results from both searches
	combinedLlamaStacks := llamav1alpha1.LlamaStackDistributionList{}
	combinedLlamaStacks.Items = append(combinedLlamaStacks.Items, userConfigLlamaStacks.Items...)
	combinedLlamaStacks.Items = append(combinedLlamaStacks.Items, caBundleLlamaStacks.Items...)

	return combinedLlamaStacks, len(combinedLlamaStacks.Items) > 0
}

// performManualSearch performs a manual search and filtering when field indexer returns no results.
func (r *LlamaStackDistributionReconciler) performManualSearch(ctx context.Context, configMap client.Object) llamav1alpha1.LlamaStackDistributionList {
	logger := log.FromContext(ctx).WithValues(
		"configMapName", configMap.GetName(),
		"configMapNamespace", configMap.GetNamespace())

	allLlamaStacks := llamav1alpha1.LlamaStackDistributionList{}
	err := r.List(ctx, &allLlamaStacks)
	if err != nil {
		logger.Error(err, "CRITICAL: Failed to list all LlamaStackDistributions for manual ConfigMap reference search")
		return allLlamaStacks
	}

	// Filter for ConfigMap references
	filteredItems := r.filterLlamaStacksForConfigMap(allLlamaStacks.Items, configMap)
	allLlamaStacks.Items = filteredItems

	return allLlamaStacks
}

// filterLlamaStacksForConfigMap filters LlamaStackDistributions that reference the given ConfigMap.
func (r *LlamaStackDistributionReconciler) filterLlamaStacksForConfigMap(llamaStacks []llamav1alpha1.LlamaStackDistribution,
	configMap client.Object) []llamav1alpha1.LlamaStackDistribution {
	var filteredItems []llamav1alpha1.LlamaStackDistribution
	targetNamespace := configMap.GetNamespace()
	targetName := configMap.GetName()

	for _, ls := range llamaStacks {
		if r.doesLlamaStackReferenceConfigMap(ls, targetNamespace, targetName) {
			filteredItems = append(filteredItems, ls)
		}
	}

	return filteredItems
}

// doesLlamaStackReferenceConfigMap checks if a LlamaStackDistribution references the specified ConfigMap.
func (r *LlamaStackDistributionReconciler) doesLlamaStackReferenceConfigMap(ls llamav1alpha1.LlamaStackDistribution, targetNamespace, targetName string) bool {
	// Check user config ConfigMap references
	if hasValidUserConfig(&ls) {
		configMapNamespace := getUserConfigMapNamespaceStandalone(&ls)
		if configMapNamespace == targetNamespace && ls.Spec.Server.UserConfig.ConfigMapName == targetName {
			return true
		}
	}

	// Check CA bundle ConfigMap references
	if hasValidCABundleConfig(&ls) {
		configMapNamespace := getCABundleConfigMapNamespaceStandalone(&ls)
		if configMapNamespace == targetNamespace && ls.Spec.Server.TLSConfig.CABundle.ConfigMapName == targetName {
			return true
		}
	}

	return false
}

// convertToReconcileRequests converts LlamaStackDistribution items to reconcile requests.
func (r *LlamaStackDistributionReconciler) convertToReconcileRequests(attachedLlamaStacks llamav1alpha1.LlamaStackDistributionList) []reconcile.Request {
	requests := make([]reconcile.Request, 0, len(attachedLlamaStacks.Items))
	for _, llamaStack := range attachedLlamaStacks.Items {
		requests = append(requests, reconcile.Request{
			NamespacedName: types.NamespacedName{
				Name:      llamaStack.Name,
				Namespace: llamaStack.Namespace,
			},
		})
	}
	return requests
}

// reconcileDeployment manages the Deployment for the LlamaStack server.
func (r *LlamaStackDistributionReconciler) reconcileDeployment(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) error {
	logger := log.FromContext(ctx)

	// Validate distribution configuration
	if err := r.validateDistribution(instance); err != nil {
		return err
	}

	// Get the image either from the map or direct reference
	resolvedImage, err := r.resolveImage(instance.Spec.Server.Distribution)
	if err != nil {
		return err
	}

	// Build container spec
	container := buildContainerSpec(ctx, r, instance, resolvedImage)

	// Configure storage
	podSpec := configurePodStorage(ctx, r, instance, container)

	// Set the service acc
	// Prepare annotations for the pod template
	podAnnotations := make(map[string]string)

	// Add ConfigMap hash to trigger restarts when the ConfigMap changes
	if r.hasUserConfigMap(instance) {
		configMapHash, err := r.getConfigMapHash(ctx, instance)
		if err != nil {
			return fmt.Errorf("failed to get ConfigMap hash for pod restart annotation: %w", err)
		}
		if configMapHash != "" {
			podAnnotations["configmap.hash/user-config"] = configMapHash
			logger.V(1).Info("Added ConfigMap hash annotation to trigger pod restart",
				"configMapName", instance.Spec.Server.UserConfig.ConfigMapName,
				"hash", configMapHash)
		}
	}

	// Add CA bundle ConfigMap hash to trigger restarts when the CA bundle changes
	if r.hasCABundleConfigMap(instance) {
		caBundleHash, err := r.getCABundleConfigMapHash(ctx, instance)
		if err != nil {
			return fmt.Errorf("failed to get CA bundle ConfigMap hash for pod restart annotation: %w", err)
		}
		if caBundleHash != "" {
			podAnnotations["configmap.hash/ca-bundle"] = caBundleHash
			logger.V(1).Info("Added CA bundle ConfigMap hash annotation to trigger pod restart",
				"configMapName", instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
				"hash", caBundleHash)
		}
	}

	// Create deployment object
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &instance.Spec.Replicas,
			Strategy: appsv1.DeploymentStrategy{
				Type: appsv1.RecreateDeploymentStrategyType,
			},
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					llamav1alpha1.DefaultLabelKey: llamav1alpha1.DefaultLabelValue,
					"app.kubernetes.io/instance":  instance.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						llamav1alpha1.DefaultLabelKey: llamav1alpha1.DefaultLabelValue,
						"app.kubernetes.io/instance":  instance.Name,
					},
					Annotations: podAnnotations,
				},
				Spec: podSpec,
			},
		},
	}

	return deploy.ApplyDeployment(ctx, r.Client, r.Scheme, instance, deployment, logger)
}

// getServerURL returns the URL for the LlamaStack server.
func (r *LlamaStackDistributionReconciler) getServerURL(instance *llamav1alpha1.LlamaStackDistribution, path string) *url.URL {
	serviceName := deploy.GetServiceName(instance)
	port := deploy.GetServicePort(instance)

	return &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s.%s.svc.cluster.local:%d", serviceName, instance.Namespace, port),
		Path:   path,
	}
}

// getProviderInfo makes an HTTP request to the providers endpoint.
func (r *LlamaStackDistributionReconciler) getProviderInfo(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) ([]llamav1alpha1.ProviderInfo, error) {
	u := r.getServerURL(instance, "/v1/providers")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create providers request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make providers request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to query providers endpoint: returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read providers response: %w", err)
	}

	var response struct {
		Data []llamav1alpha1.ProviderInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal providers response: %w", err)
	}

	return response.Data, nil
}

// getVersionInfo makes an HTTP request to the version endpoint.
func (r *LlamaStackDistributionReconciler) getVersionInfo(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) (string, error) {
	u := r.getServerURL(instance, "/v1/version")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create version request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make version request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to query version endpoint: returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("failed to read version response: %w", err)
	}

	var response struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return "", fmt.Errorf("failed to unmarshal version response: %w", err)
	}

	return response.Version, nil
}

// updateStatus refreshes the LlamaStack status.
func (r *LlamaStackDistributionReconciler) updateStatus(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution, reconcileErr error) error {
	logger := log.FromContext(ctx)
	// Initialize OperatorVersion if not set
	if instance.Status.Version.OperatorVersion == "" {
		instance.Status.Version.OperatorVersion = os.Getenv("OPERATOR_VERSION")
	}

	// A reconciliation error is the highest priority. It overrides all other status checks.
	if reconcileErr != nil {
		instance.Status.Phase = llamav1alpha1.LlamaStackDistributionPhaseFailed
		SetDeploymentReadyCondition(&instance.Status, false, fmt.Sprintf("Resource reconciliation failed: %v", reconcileErr))
	} else {
		// If reconciliation was successful, proceed with detailed status checks.
		deploymentReady, err := r.updateDeploymentStatus(ctx, instance)
		if err != nil {
			return err // Early exit if we can't get deployment status
		}

		r.updateStorageStatus(ctx, instance)
		r.updateServiceStatus(ctx, instance)
		r.updateDistributionConfig(instance)

		if deploymentReady {
			instance.Status.Phase = llamav1alpha1.LlamaStackDistributionPhaseReady

			providers, err := r.getProviderInfo(ctx, instance)
			if err != nil {
				logger.Error(err, "failed to get provider info, clearing provider list")
				instance.Status.DistributionConfig.Providers = nil
			} else {
				instance.Status.DistributionConfig.Providers = providers
			}

			version, err := r.getVersionInfo(ctx, instance)
			if err != nil {
				logger.Error(err, "failed to get version info from API endpoint")
				// Don't clear the version if we cant fetch it - keep the existing one
			} else {
				instance.Status.Version.LlamaStackServerVersion = version
				logger.V(1).Info("Updated LlamaStack version from API endpoint", "version", version)
			}

			SetHealthCheckCondition(&instance.Status, true, MessageHealthCheckPassed)
		} else {
			// If not ready, health can't be checked. Set condition appropriately.
			SetHealthCheckCondition(&instance.Status, false, "Deployment not ready")
			instance.Status.DistributionConfig.Providers = nil // Clear providers
		}
	}

	// Always update the status at the end of the function.
	instance.Status.Version.LastUpdated = metav1.NewTime(metav1.Now().UTC())
	if err := r.Status().Update(ctx, instance); err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}

	return nil
}

func (r *LlamaStackDistributionReconciler) updateDeploymentStatus(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) (bool, error) {
	deployment := &appsv1.Deployment{}
	deploymentErr := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
	if deploymentErr != nil && !k8serrors.IsNotFound(deploymentErr) {
		return false, fmt.Errorf("failed to fetch deployment for status: %w", deploymentErr)
	}

	deploymentReady := false

	switch {
	case deploymentErr != nil: // This case covers when the deployment is not found
		instance.Status.Phase = llamav1alpha1.LlamaStackDistributionPhasePending
		SetDeploymentReadyCondition(&instance.Status, false, MessageDeploymentPending)
	case deployment.Status.ReadyReplicas == 0:
		instance.Status.Phase = llamav1alpha1.LlamaStackDistributionPhaseInitializing
		SetDeploymentReadyCondition(&instance.Status, false, MessageDeploymentPending)
	case deployment.Status.ReadyReplicas < instance.Spec.Replicas:
		instance.Status.Phase = llamav1alpha1.LlamaStackDistributionPhaseInitializing
		deploymentMessage := fmt.Sprintf("Deployment is scaling: %d/%d replicas ready", deployment.Status.ReadyReplicas, instance.Spec.Replicas)
		SetDeploymentReadyCondition(&instance.Status, false, deploymentMessage)
	case deployment.Status.ReadyReplicas > instance.Spec.Replicas:
		instance.Status.Phase = llamav1alpha1.LlamaStackDistributionPhaseInitializing
		deploymentMessage := fmt.Sprintf("Deployment is scaling down: %d/%d replicas ready", deployment.Status.ReadyReplicas, instance.Spec.Replicas)
		SetDeploymentReadyCondition(&instance.Status, false, deploymentMessage)
	default:
		instance.Status.Phase = llamav1alpha1.LlamaStackDistributionPhaseReady
		deploymentReady = true
		SetDeploymentReadyCondition(&instance.Status, true, MessageDeploymentReady)
	}
	instance.Status.AvailableReplicas = deployment.Status.ReadyReplicas
	return deploymentReady, nil
}

func (r *LlamaStackDistributionReconciler) updateStorageStatus(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) {
	if instance.Spec.Server.Storage == nil {
		return
	}
	pvc := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, types.NamespacedName{Name: instance.Name + "-pvc", Namespace: instance.Namespace}, pvc)
	if err != nil {
		SetStorageReadyCondition(&instance.Status, false, fmt.Sprintf("Failed to get PVC: %v", err))
		return
	}

	ready := pvc.Status.Phase == corev1.ClaimBound
	var message string
	if ready {
		message = MessageStorageReady
	} else {
		message = fmt.Sprintf("PVC is not bound: %s", pvc.Status.Phase)
	}
	SetStorageReadyCondition(&instance.Status, ready, message)
}

func (r *LlamaStackDistributionReconciler) updateServiceStatus(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) {
	logger := log.FromContext(ctx)
	if !instance.HasPorts() {
		logger.Info("No ports defined, skipping service status update")
		return
	}
	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: instance.Name + "-service", Namespace: instance.Namespace}, service)
	if err != nil {
		SetServiceReadyCondition(&instance.Status, false, fmt.Sprintf("Failed to get Service: %v", err))
		return
	}
	SetServiceReadyCondition(&instance.Status, true, MessageServiceReady)
}

func (r *LlamaStackDistributionReconciler) updateDistributionConfig(instance *llamav1alpha1.LlamaStackDistribution) {
	instance.Status.DistributionConfig.AvailableDistributions = r.ClusterInfo.DistributionImages
	var activeDistribution string
	if instance.Spec.Server.Distribution.Name != "" {
		activeDistribution = instance.Spec.Server.Distribution.Name
	} else if instance.Spec.Server.Distribution.Image != "" {
		activeDistribution = "custom"
	}
	instance.Status.DistributionConfig.ActiveDistribution = activeDistribution
}

// reconcileNetworkPolicy manages the NetworkPolicy for the LlamaStack server.
func (r *LlamaStackDistributionReconciler) reconcileNetworkPolicy(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) error {
	logger := log.FromContext(ctx)
	networkPolicy := &networkingv1.NetworkPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + "-network-policy",
			Namespace: instance.Namespace,
		},
	}

	// If feature is disabled, delete the NetworkPolicy if it exists
	if !r.EnableNetworkPolicy {
		return deploy.HandleDisabledNetworkPolicy(ctx, r.Client, networkPolicy, logger)
	}

	port := deploy.GetServicePort(instance)

	// get operator namespace
	operatorNamespace, err := deploy.GetOperatorNamespace()
	if err != nil {
		return fmt.Errorf("failed to get operator namespace: %w", err)
	}

	networkPolicy.Spec = networkingv1.NetworkPolicySpec{
		PodSelector: metav1.LabelSelector{
			MatchLabels: map[string]string{
				llamav1alpha1.DefaultLabelKey: llamav1alpha1.DefaultLabelValue,
				"app.kubernetes.io/instance":  instance.Name,
			},
		},
		PolicyTypes: []networkingv1.PolicyType{
			networkingv1.PolicyTypeIngress,
		},
		Ingress: []networkingv1.NetworkPolicyIngressRule{
			{
				From: []networkingv1.NetworkPolicyPeer{
					{ // to match all pods in all namespaces
						PodSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"app.kubernetes.io/part-of": llamav1alpha1.DefaultContainerName,
							},
						},
						NamespaceSelector: &metav1.LabelSelector{}, // Empty namespaceSelector to match all namespaces
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: (*corev1.Protocol)(ptr.To("TCP")),
						Port: &intstr.IntOrString{
							IntVal: port,
						},
					},
				},
			},
			{
				From: []networkingv1.NetworkPolicyPeer{
					{ // to match all pods in matched namespace
						PodSelector: &metav1.LabelSelector{},
						NamespaceSelector: &metav1.LabelSelector{
							MatchLabels: map[string]string{
								"kubernetes.io/metadata.name": operatorNamespace,
							},
						},
					},
				},
				Ports: []networkingv1.NetworkPolicyPort{
					{
						Protocol: (*corev1.Protocol)(ptr.To("TCP")),
						Port: &intstr.IntOrString{
							IntVal: port,
						},
					},
				},
			},
		},
	}

	return deploy.ApplyNetworkPolicy(ctx, r.Client, r.Scheme, instance, networkPolicy, logger)
}

// reconcileUserConfigMap validates that the referenced ConfigMap exists.
func (r *LlamaStackDistributionReconciler) reconcileUserConfigMap(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) error {
	logger := log.FromContext(ctx)

	if !r.hasUserConfigMap(instance) {
		logger.V(1).Info("No user ConfigMap specified, skipping")
		return nil
	}

	// Determine the ConfigMap namespace - default to the same namespace as the LlamaStackDistribution.
	configMapNamespace := r.getUserConfigMapNamespace(instance)

	logger.V(1).Info("Validating referenced ConfigMap exists",
		"configMapName", instance.Spec.Server.UserConfig.ConfigMapName,
		"configMapNamespace", configMapNamespace)

	// Check if the ConfigMap exists
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.Server.UserConfig.ConfigMapName,
		Namespace: configMapNamespace,
	}, configMap)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Error(err, "Referenced ConfigMap not found",
				"configMapName", instance.Spec.Server.UserConfig.ConfigMapName,
				"configMapNamespace", configMapNamespace)
			return fmt.Errorf("failed to find referenced ConfigMap %s/%s", configMapNamespace, instance.Spec.Server.UserConfig.ConfigMapName)
		}
		return fmt.Errorf("failed to fetch ConfigMap %s/%s: %w", configMapNamespace, instance.Spec.Server.UserConfig.ConfigMapName, err)
	}

	logger.V(1).Info("User ConfigMap found and validated",
		"configMap", configMap.Name,
		"namespace", configMap.Namespace,
		"dataKeys", len(configMap.Data))
	return nil
}

// isValidPEM validates that the given data contains valid PEM formatted content.
func isValidPEM(data []byte) bool {
	// Basic PEM validation using pem.Decode.
	block, _ := pem.Decode(data)
	return block != nil
}

// reconcileCABundleConfigMap validates that the referenced CA bundle ConfigMap exists.
func (r *LlamaStackDistributionReconciler) reconcileCABundleConfigMap(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) error {
	logger := log.FromContext(ctx)

	if !r.hasCABundleConfigMap(instance) {
		logger.V(1).Info("No CA bundle ConfigMap specified, skipping")
		return nil
	}

	// Determine the ConfigMap namespace - default to the same namespace as the LlamaStackDistribution.
	configMapNamespace := r.getCABundleConfigMapNamespace(instance)

	logger.V(1).Info("Validating referenced CA bundle ConfigMap exists",
		"configMapName", instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
		"configMapNamespace", configMapNamespace)

	// Check if the ConfigMap exists
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
		Namespace: configMapNamespace,
	}, configMap)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Error(err, "Referenced CA bundle ConfigMap not found",
				"configMapName", instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
				"configMapNamespace", configMapNamespace)
			return fmt.Errorf("failed to find referenced CA bundle ConfigMap %s/%s", configMapNamespace, instance.Spec.Server.TLSConfig.CABundle.ConfigMapName)
		}
		return fmt.Errorf("failed to fetch CA bundle ConfigMap %s/%s: %w", configMapNamespace, instance.Spec.Server.TLSConfig.CABundle.ConfigMapName, err)
	}

	// Validate that the specified keys exist in the ConfigMap
	var keysToValidate []string
	if len(instance.Spec.Server.TLSConfig.CABundle.ConfigMapKeys) > 0 {
		keysToValidate = instance.Spec.Server.TLSConfig.CABundle.ConfigMapKeys
	} else {
		// Default to DefaultCABundleKey when no keys are specified
		keysToValidate = []string{DefaultCABundleKey}
	}

	for _, key := range keysToValidate {
		if _, exists := configMap.Data[key]; !exists {
			logger.Error(err, "CA bundle key not found in ConfigMap",
				"configMapName", instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
				"configMapNamespace", configMapNamespace,
				"key", key)
			return fmt.Errorf("failed to find CA bundle key '%s' in ConfigMap %s/%s", key, configMapNamespace, instance.Spec.Server.TLSConfig.CABundle.ConfigMapName)
		}

		// Validate that the key contains valid PEM data
		pemData, exists := configMap.Data[key]
		if !exists {
			// This should not happen since we checked above, but just to be safe
			return fmt.Errorf("failed to find CA bundle key '%s' in ConfigMap %s/%s", key, configMapNamespace, instance.Spec.Server.TLSConfig.CABundle.ConfigMapName)
		}

		if !isValidPEM([]byte(pemData)) {
			logger.Error(nil, "CA bundle key contains invalid PEM data",
				"configMapName", instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
				"configMapNamespace", configMapNamespace,
				"key", key)
			return fmt.Errorf("failed to validate CA bundle key '%s' in ConfigMap %s/%s: contains invalid PEM data",
				key,
				configMapNamespace,
				instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
			)
		}

		logger.V(1).Info("CA bundle key contains valid PEM data",
			"configMapName", instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
			"configMapNamespace", configMapNamespace,
			"key", key)
	}

	logger.V(1).Info("CA bundle ConfigMap found and validated",
		"configMap", configMap.Name,
		"namespace", configMap.Namespace,
		"keys", keysToValidate,
		"dataKeys", len(configMap.Data))
	return nil
}

// getConfigMapHash calculates a hash of the ConfigMap data to detect changes.
func (r *LlamaStackDistributionReconciler) getConfigMapHash(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) (string, error) {
	if !r.hasUserConfigMap(instance) {
		return "", nil
	}

	configMapNamespace := r.getUserConfigMapNamespace(instance)

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.Server.UserConfig.ConfigMapName,
		Namespace: configMapNamespace,
	}, configMap)
	if err != nil {
		return "", err
	}

	// Create a content-based hash that will change when the ConfigMap data changes
	return fmt.Sprintf("%s-%s", configMap.ResourceVersion, configMap.Name), nil
}

// getCABundleConfigMapHash calculates a hash of the CA bundle ConfigMap data to detect changes.
func (r *LlamaStackDistributionReconciler) getCABundleConfigMapHash(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) (string, error) {
	if !r.hasCABundleConfigMap(instance) {
		return "", nil
	}

	configMapNamespace := r.getCABundleConfigMapNamespace(instance)

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.Server.TLSConfig.CABundle.ConfigMapName,
		Namespace: configMapNamespace,
	}, configMap)
	if err != nil {
		return "", err
	}

	// Create a content-based hash that will change when the ConfigMap data changes
	// Include information about which keys are being used
	var keyInfo string
	if len(instance.Spec.Server.TLSConfig.CABundle.ConfigMapKeys) > 0 {
		keyInfo = fmt.Sprintf("-%s", strings.Join(instance.Spec.Server.TLSConfig.CABundle.ConfigMapKeys, ","))
	} else {
		// Default to DefaultCABundleKey when no keys are specified
		keyInfo = fmt.Sprintf("-%s", DefaultCABundleKey)
	}

	return fmt.Sprintf("%s-%s%s", configMap.ResourceVersion, configMap.Name, keyInfo), nil
}

// detectODHTrustedCABundle checks if the well-known ODH trusted CA bundle ConfigMap
// exists in the same namespace as the LlamaStackDistribution and returns its available keys.
// Returns the ConfigMap and a list of data keys if found, or nil and empty slice if not found.
func (r *LlamaStackDistributionReconciler) detectODHTrustedCABundle(ctx context.Context, instance *llamav1alpha1.LlamaStackDistribution) (*corev1.ConfigMap, []string, error) {
	logger := log.FromContext(ctx)

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      odhTrustedCABundleConfigMap,
		Namespace: instance.Namespace,
	}, configMap)

	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.V(1).Info("ODH trusted CA bundle ConfigMap not found, skipping auto-detection",
				"configMapName", odhTrustedCABundleConfigMap,
				"namespace", instance.Namespace)
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("failed to check for ODH trusted CA bundle ConfigMap %s/%s: %w",
			instance.Namespace, odhTrustedCABundleConfigMap, err)
	}

	// Extract available data keys and validate they contain valid PEM data
	keys := make([]string, 0, len(configMap.Data))

	for key, value := range configMap.Data {
		// Only include keys that contain valid PEM data
		if isValidPEM([]byte(value)) {
			keys = append(keys, key)
			logger.V(1).Info("Auto-detected CA bundle key contains valid PEM data",
				"configMapName", odhTrustedCABundleConfigMap,
				"namespace", instance.Namespace,
				"key", key)
		} else {
			logger.V(1).Info("Auto-detected CA bundle key contains invalid PEM data, skipping",
				"configMapName", odhTrustedCABundleConfigMap,
				"namespace", instance.Namespace,
				"key", key)
		}
	}

	logger.V(1).Info("ODH trusted CA bundle ConfigMap detected",
		"configMapName", odhTrustedCABundleConfigMap,
		"namespace", instance.Namespace,
		"availableKeys", keys)

	return configMap, keys, nil
}

// createDefaultConfigMap creates a ConfigMap with default feature flag values.
func createDefaultConfigMap(configMapName types.NamespacedName) (*corev1.ConfigMap, error) {
	featureFlags := featureflags.FeatureFlags{
		EnableNetworkPolicy: featureflags.FeatureFlag{
			Enabled: featureflags.NetworkPolicyDefaultValue,
		},
	}

	featureFlagsYAML, err := yaml.Marshal(featureFlags)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal default feature flags: %w", err)
	}

	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName.Name,
			Namespace: configMapName.Namespace,
		},
		Data: map[string]string{
			featureflags.FeatureFlagsKey: string(featureFlagsYAML),
		},
	}, nil
}

// parseFeatureFlags extracts and parses feature flags from ConfigMap data.
func parseFeatureFlags(configMapData map[string]string) (bool, error) {
	enableNetworkPolicy := featureflags.NetworkPolicyDefaultValue

	featureFlagsYAML, exists := configMapData[featureflags.FeatureFlagsKey]
	if !exists {
		return enableNetworkPolicy, nil
	}

	var flags featureflags.FeatureFlags
	if err := yaml.Unmarshal([]byte(featureFlagsYAML), &flags); err != nil {
		return false, fmt.Errorf("failed to parse feature flags: %w", err)
	}

	return flags.EnableNetworkPolicy.Enabled, nil
}

// NewLlamaStackDistributionReconciler creates a new reconciler with default image mappings.
func NewLlamaStackDistributionReconciler(ctx context.Context, client client.Client, scheme *runtime.Scheme,
	clusterInfo *cluster.ClusterInfo) (*LlamaStackDistributionReconciler, error) {
	// get operator namespace
	operatorNamespace, err := deploy.GetOperatorNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to get operator namespace: %w", err)
	}

	// Get the ConfigMap
	// If the ConfigMap doesn't exist, create it with default feature flags
	// If the ConfigMap exists, parse the feature flags from the Configmap
	configMap := &corev1.ConfigMap{}
	configMapName := types.NamespacedName{
		Name:      operatorConfigData,
		Namespace: operatorNamespace,
	}

	if err = client.Get(ctx, configMapName, configMap); err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, fmt.Errorf("failed to get ConfigMap: %w", err)
		}

		// ConfigMap doesn't exist, create it with defaults
		configMap, err = createDefaultConfigMap(configMapName)
		if err != nil {
			return nil, fmt.Errorf("failed to generate default configMap: %w", err)
		}

		if err = client.Create(ctx, configMap); err != nil {
			return nil, fmt.Errorf("failed to create ConfigMap: %w", err)
		}
	}

	// Parse feature flags from ConfigMap
	enableNetworkPolicy, err := parseFeatureFlags(configMap.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to parse feature flags: %w", err)
	}
	return &LlamaStackDistributionReconciler{
		Client:              client,
		Scheme:              scheme,
		EnableNetworkPolicy: enableNetworkPolicy,
		ClusterInfo:         clusterInfo,
		httpClient:          &http.Client{Timeout: 5 * time.Second},
	}, nil
}

// NewTestReconciler creates a reconciler for testing, allowing injection of a custom http client and feature flags.
func NewTestReconciler(client client.Client, scheme *runtime.Scheme, clusterInfo *cluster.ClusterInfo,
	httpClient *http.Client, enableNetworkPolicy bool) *LlamaStackDistributionReconciler {
	return &LlamaStackDistributionReconciler{
		Client:              client,
		Scheme:              scheme,
		ClusterInfo:         clusterInfo,
		httpClient:          httpClient,
		EnableNetworkPolicy: enableNetworkPolicy,
	}
}
