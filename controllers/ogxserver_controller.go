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
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/x509"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-containerregistry/pkg/name"
	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/config"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/deploy"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"gopkg.in/yaml.v3"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
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
	operatorConfigData = "ogx-operator-config"
	manifestsBasePath  = "manifests/base"
	managedByLabelKey  = "app.kubernetes.io/managed-by"
	managedByLabelVal  = "ogx-operator"

	// CA Bundle related constants.
	DefaultCABundleKey             = "ca-bundle.crt"
	CABundleVolumeName             = "ca-bundle"
	ManagedCABundleConfigMapSuffix = "-ca-bundle"
	ManagedCABundleKey             = "ca-bundle.crt"
	ManagedCABundleMountPath       = "/etc/ssl/certs/ca-bundle"
	ManagedCABundleFilePath        = "/etc/ssl/certs/ca-bundle/ca-bundle.crt"

	// Security limits for CA bundle processing.
	MaxCABundleSize         = 10 * 1024 * 1024 // 10MB max total size
	MaxCABundleCertificates = 1000             // Maximum number of certificates

	// ODH/RHOAI well-known ConfigMap for trusted CA bundles.
	odhTrustedCABundleConfigMap = "odh-trusted-ca-bundle"

	// WatchLabelKey is the label key used to include ConfigMaps in the operator's cache.
	// Operator-managed ConfigMaps get this label automatically. Users can add it to
	// their ConfigMaps for instant reconciliation on change.
	WatchLabelKey = "ogx.io/watch"
	// WatchLabelValue is the expected value for the watch label.
	WatchLabelValue = "true"
)

// OGXServerReconciler reconciles an OGXServer object.
type OGXServerReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	// Image mapping overrides
	ImageMappingOverrides map[string]string
	// Cluster info
	ClusterInfo *cluster.ClusterInfo
	httpClient  *http.Client
	// OCILabelFetcher fetches OCI image labels for config resolution.
	// When nil, OCI label resolution is disabled.
	OCILabelFetcher config.OCILabelFetcher
	// configResolver resolves base config from OCI labels. Kept on the reconciler
	// so the OCI config cache persists across reconciliations.
	configResolver config.ConfigResolver

	// Cached operator namespace used for config refresh during reconciliation.
	operatorNamespace string
}

// hasCACertificates checks if the instance has TLS trust CA certificates configured.
func (r *OGXServerReconciler) hasCACertificates(instance *ogxiov1beta1.OGXServer) bool {
	return instance.Spec.TLS != nil && instance.Spec.TLS.Trust != nil && len(instance.Spec.TLS.Trust.CACertificates) > 0
}

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// the OGXServer object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.12.2/pkg/reconcile
func (r *OGXServerReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Create a logger with request-specific values and store it in the context.
	// This ensures consistent logging across the reconciliation process and its sub-functions.
	// The logger is retrieved from the context in each sub-function that needs it, maintaining
	// the request-specific values throughout the call chain.
	// Always ensure the name of the CR and the namespace are included in the logger.
	logger := log.FromContext(ctx).WithValues("namespace", req.Namespace, "name", req.Name)
	ctx = logr.NewContext(ctx, logger)

	// Refresh image mapping overrides from the operator config ConfigMap.
	r.refreshOperatorConfig(ctx)

	// Fetch the OGXServer instance
	instance, err := r.fetchInstance(ctx, req.NamespacedName)
	if err != nil {
		return ctrl.Result{}, err
	}

	if instance == nil {
		logger.V(1).Info("OGXServer resource not found, skipping reconciliation")
		return ctrl.Result{}, nil
	}

	// Reconcile all resources, storing the error for later.
	reconcileErr := r.reconcileResources(ctx, instance)

	if result, done := r.handleSentinelErrors(ctx, instance, reconcileErr); done {
		return result, nil
	}

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
	if instance.Status.Phase == ogxiov1beta1.OGXServerPhaseInitializing {
		return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
	}

	logger.Info("Successfully reconciled OGXServer")
	return ctrl.Result{RequeueAfter: 5 * time.Minute}, nil
}

// refreshOperatorConfig re-reads the operator config ConfigMap and updates image mapping overrides.
func (r *OGXServerReconciler) refreshOperatorConfig(ctx context.Context) {
	logger := log.FromContext(ctx)

	operatorNamespace := r.operatorNamespace
	if operatorNamespace == "" {
		var err error
		operatorNamespace, err = deploy.GetOperatorNamespace()
		if err != nil {
			logger.Error(err, "failed to get operator namespace for config refresh")
			return
		}
		r.operatorNamespace = operatorNamespace
	}

	configMap := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{
		Name:      operatorConfigData,
		Namespace: operatorNamespace,
	}, configMap); err != nil {
		logger.Error(err, "failed to refresh operator config")
		return
	}

	r.ImageMappingOverrides = ParseImageMappingOverrides(ctx, configMap.Data)
}

// fetchInstance retrieves the OGXServer instance.
func (r *OGXServerReconciler) fetchInstance(ctx context.Context, namespacedName types.NamespacedName) (*ogxiov1beta1.OGXServer, error) {
	logger := log.FromContext(ctx)
	instance := &ogxiov1beta1.OGXServer{}
	if err := r.Get(ctx, namespacedName, instance); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("failed to find OGXServer resource")
			return nil, nil
		}
		return nil, fmt.Errorf("failed to fetch OGXServer: %w", err)
	}
	return instance, nil
}

// determineKindsToExclude returns a list of resource kinds that should be excluded
// based on the instance specification and adoption annotations.
func (r *OGXServerReconciler) determineKindsToExclude(instance *ogxiov1beta1.OGXServer, effectivePVCName string) []string {
	var kinds []string

	if shouldExcludePVC(instance, effectivePVCName) {
		kinds = append(kinds, "PersistentVolumeClaim")
	}

	// Per-CR NetworkPolicy toggle (default: enabled)
	if instance.Spec.Network != nil && instance.Spec.Network.Policy != nil &&
		instance.Spec.Network.Policy.Enabled != nil && !*instance.Spec.Network.Policy.Enabled {
		kinds = append(kinds, "NetworkPolicy")
	}

	if !needsPodDisruptionBudget(instance) {
		kinds = append(kinds, "PodDisruptionBudget")
	}

	if instance.Spec.Workload == nil || instance.Spec.Workload.Autoscaling == nil {
		kinds = append(kinds, "HorizontalPodAutoscaler")
	}

	if isMonitoringDisabled(instance) {
		kinds = append(kinds, "PrometheusRule", "ServiceMonitor")
	}

	return kinds
}

func shouldExcludePVC(instance *ogxiov1beta1.OGXServer, effectivePVCName string) bool {
	// Suppress PVC creation when the deployment is using an adopted PVC
	// (either via annotation or discovered by label after annotation removal).
	if effectivePVCName != instance.Name+"-pvc" {
		return true
	}

	return instance.Spec.Workload == nil || instance.Spec.Workload.Storage == nil
}

// reconcileAllManifestResources applies all manifest-based resources using kustomize.
func (r *OGXServerReconciler) reconcileAllManifestResources(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
) error {
	// Resolve the PVC name once — may use annotation or label-based discovery.
	effectivePVCName, err := r.resolveEffectivePVCName(ctx, instance)
	if err != nil {
		return fmt.Errorf("failed to resolve effective PVC name: %w", err)
	}

	// Build manifest context for Deployment
	manifestCtx, err := r.buildManifestContext(ctx, instance, runtimeConfig, effectivePVCName)
	if err != nil {
		return fmt.Errorf("failed to build manifest context: %w", err)
	}

	// Render manifests with context
	resMap, err := deploy.RenderManifestWithContext(filesys.MakeFsOnDisk(), manifestsBasePath, instance, manifestCtx)
	if err != nil {
		return fmt.Errorf("failed to render manifests: %w", err)
	}

	kindsToExclude := r.determineKindsToExclude(instance, effectivePVCName)

	if !slices.Contains(kindsToExclude, "PrometheusRule") {
		monitoringAvailable, monErr := deploy.MonitoringCRDsAvailable(ctx, r.Client)
		if monErr != nil {
			return fmt.Errorf("failed to check monitoring CRD availability: %w", monErr)
		}
		if !monitoringAvailable {
			kindsToExclude = append(kindsToExclude, "PrometheusRule", "ServiceMonitor")
		}
	}

	filteredResMap, err := deploy.FilterExcludeKinds(resMap, kindsToExclude)
	if err != nil {
		return fmt.Errorf("failed to filter manifests: %w", err)
	}

	// Delete excluded resources that might exist from previous reconciliations
	if err := r.deleteExcludedResources(ctx, instance, kindsToExclude); err != nil {
		return fmt.Errorf("failed to delete excluded resources: %w", err)
	}

	// Apply resources to cluster
	if err := deploy.ApplyResources(ctx, r.Client, r.Scheme, instance, filteredResMap); err != nil {
		return fmt.Errorf("failed to apply manifests: %w", err)
	}

	return nil
}

// deleteExcludedResources deletes resources that are excluded from the current reconciliation
// but might exist from previous reconciliations.
func (r *OGXServerReconciler) deleteExcludedResources(ctx context.Context, instance *ogxiov1beta1.OGXServer, kindsToExclude []string) error {
	logger := log.FromContext(ctx)

	if slices.Contains(kindsToExclude, "NetworkPolicy") {
		if err := r.deleteNetworkPolicyIfExists(ctx, instance); err != nil {
			logger.Error(err, "Failed to delete NetworkPolicy")
			return err
		}
	}

	if slices.Contains(kindsToExclude, "PodDisruptionBudget") {
		if err := r.deletePodDisruptionBudgetIfExists(ctx, instance); err != nil {
			logger.Error(err, "Failed to delete PodDisruptionBudget")
			return err
		}
	}

	if slices.Contains(kindsToExclude, "HorizontalPodAutoscaler") {
		if err := r.deleteHorizontalPodAutoscalerIfExists(ctx, instance); err != nil {
			logger.Error(err, "Failed to delete HorizontalPodAutoscaler")
			return err
		}
	}

	if err := r.deleteMonitoringResourcesIfExcluded(ctx, instance, kindsToExclude); err != nil {
		return err
	}

	return nil
}

// deleteNetworkPolicyIfExists deletes the NetworkPolicy if it exists.
func (r *OGXServerReconciler) deleteNetworkPolicyIfExists(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	networkPolicy := &networkingv1.NetworkPolicy{}
	networkPolicyName := instance.Name + "-network-policy"
	key := types.NamespacedName{Name: networkPolicyName, Namespace: instance.Namespace}

	err := r.Get(ctx, key, networkPolicy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get NetworkPolicy: %w", err)
	}

	if !metav1.IsControlledBy(networkPolicy, instance) {
		logger.V(1).Info("NetworkPolicy not owned by this instance, skipping deletion",
			"networkPolicy", networkPolicyName)
		return nil
	}

	logger.Info("Deleting NetworkPolicy as it is disabled for this instance", "networkPolicy", networkPolicyName)
	if err := r.Delete(ctx, networkPolicy); err != nil {
		return fmt.Errorf("failed to delete NetworkPolicy: %w", err)
	}

	return nil
}

func (r *OGXServerReconciler) deletePodDisruptionBudgetIfExists(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	pdb := &policyv1.PodDisruptionBudget{}
	pdbName := instance.Name + "-pdb"
	key := types.NamespacedName{Name: pdbName, Namespace: instance.Namespace}

	if err := r.Get(ctx, key, pdb); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get PodDisruptionBudget: %w", err)
	}

	if !metav1.IsControlledBy(pdb, instance) {
		logger.V(1).Info("PodDisruptionBudget not owned by this instance, skipping deletion", "pdb", pdbName)
		return nil
	}

	logger.Info("Deleting PodDisruptionBudget as feature is disabled", "pdb", pdbName)
	if err := r.Delete(ctx, pdb); err != nil {
		return fmt.Errorf("failed to delete PodDisruptionBudget: %w", err)
	}

	return nil
}

func (r *OGXServerReconciler) deleteHorizontalPodAutoscalerIfExists(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	hpa := &autoscalingv2.HorizontalPodAutoscaler{}
	hpaName := instance.Name + "-hpa"
	key := types.NamespacedName{Name: hpaName, Namespace: instance.Namespace}

	if err := r.Get(ctx, key, hpa); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get HorizontalPodAutoscaler: %w", err)
	}

	if !metav1.IsControlledBy(hpa, instance) {
		logger.V(1).Info("HorizontalPodAutoscaler not owned by this instance, skipping deletion", "hpa", hpaName)
		return nil
	}

	logger.Info("Deleting HorizontalPodAutoscaler as feature is disabled", "hpa", hpaName)
	if err := r.Delete(ctx, hpa); err != nil {
		return fmt.Errorf("failed to delete HorizontalPodAutoscaler: %w", err)
	}

	return nil
}

func (r *OGXServerReconciler) deleteMonitoringResourcesIfExcluded(ctx context.Context, instance *ogxiov1beta1.OGXServer, kindsToExclude []string) error {
	logger := log.FromContext(ctx)

	if slices.Contains(kindsToExclude, "PrometheusRule") {
		if err := r.deletePrometheusRuleIfExists(ctx, instance); err != nil {
			logger.Error(err, "Failed to delete PrometheusRule")
			return err
		}
	}

	if slices.Contains(kindsToExclude, "ServiceMonitor") {
		if err := r.deleteServiceMonitorIfExists(ctx, instance); err != nil {
			logger.Error(err, "Failed to delete ServiceMonitor")
			return err
		}
	}

	return nil
}

func (r *OGXServerReconciler) deletePrometheusRuleIfExists(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	promRule := &monitoringv1.PrometheusRule{}
	promRuleName := instance.Name + "-prometheus-rules"
	key := types.NamespacedName{Name: promRuleName, Namespace: instance.Namespace}

	if err := r.Get(ctx, key, promRule); err != nil {
		if k8serrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("failed to get PrometheusRule: %w", err)
	}

	if !metav1.IsControlledBy(promRule, instance) {
		logger.V(1).Info("PrometheusRule not owned by this instance, skipping deletion", "prometheusRule", promRuleName)
		return nil
	}

	logger.Info("Deleting PrometheusRule as monitoring is disabled", "prometheusRule", promRuleName)
	if err := r.Delete(ctx, promRule); err != nil {
		return fmt.Errorf("failed to delete PrometheusRule: %w", err)
	}

	return nil
}

func (r *OGXServerReconciler) deleteServiceMonitorIfExists(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	sm := &monitoringv1.ServiceMonitor{}
	smName := instance.Name + "-service-monitor"
	key := types.NamespacedName{Name: smName, Namespace: instance.Namespace}

	if err := r.Get(ctx, key, sm); err != nil {
		if k8serrors.IsNotFound(err) || apimeta.IsNoMatchError(err) {
			return nil
		}
		return fmt.Errorf("failed to get ServiceMonitor: %w", err)
	}

	if !metav1.IsControlledBy(sm, instance) {
		logger.V(1).Info("ServiceMonitor not owned by this instance, skipping deletion", "serviceMonitor", smName)
		return nil
	}

	logger.Info("Deleting ServiceMonitor as monitoring is disabled", "serviceMonitor", smName)
	if err := r.Delete(ctx, sm); err != nil {
		return fmt.Errorf("failed to delete ServiceMonitor: %w", err)
	}

	return nil
}

// buildManifestContext creates the manifest context for Deployment using existing helper functions.
//
// buildManifestContext builds the desired pod/deployment inputs for the
// current reconcile pass using the resolved runtime config reference.
func (r *OGXServerReconciler) buildManifestContext(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
	effectivePVCName string,
) (*deploy.ManifestContext, error) {
	if err := r.validateDistribution(instance); err != nil {
		return nil, err
	}

	resolvedImage, err := r.resolveImage(instance.Spec.Distribution)
	if err != nil {
		return nil, err
	}

	// Compute secret env vars once; reused for both container env injection
	// and rollout-triggering secret hash computation.
	var secretEnvVars []corev1.EnvVar
	if runtimeConfig != nil && runtimeConfig.Generated {
		secretEnvVars = config.CollectSecretRefs(&instance.Spec)
	}

	container := buildContainerSpec(ctx, r, instance, resolvedImage, runtimeConfig, secretEnvVars)
	podSpec := configurePodStorage(ctx, r, instance, runtimeConfig, container, effectivePVCName)

	configMapHash, err := r.resolveConfigMapHash(ctx, instance, runtimeConfig)
	if err != nil {
		return nil, err
	}

	caBundleHash, err := r.resolveCABundleHash(ctx, instance)
	if err != nil {
		return nil, err
	}

	secretHash, err := r.resolveSecretRefsHash(ctx, instance, runtimeConfig, secretEnvVars)
	if err != nil {
		return nil, err
	}

	podSpecMap, err := runtime.DefaultUnstructuredConverter.ToUnstructured(&podSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to convert pod spec to map: %w", err)
	}

	return &deploy.ManifestContext{
		ResolvedImage:           resolvedImage,
		ConfigMapHash:           configMapHash,
		CABundleHash:            caBundleHash,
		SecretHash:              secretHash,
		PodSpec:                 podSpecMap,
		PodDisruptionBudgetSpec: buildPodDisruptionBudgetSpec(instance),
		HPASpec:                 buildHPASpec(instance),
	}, nil
}

func (r *OGXServerReconciler) resolveConfigMapHash(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
) (string, error) {
	if runtimeConfig == nil {
		return "", nil
	}
	if !runtimeConfig.Generated {
		return r.getConfigMapHash(ctx, instance)
	}
	return r.getGeneratedConfigMapHashByName(ctx, instance.Namespace, runtimeConfig.ConfigMapName)
}

func (r *OGXServerReconciler) resolveCABundleHash(ctx context.Context, instance *ogxiov1beta1.OGXServer) (string, error) {
	if r.hasCACertificates(instance) {
		return r.getCABundleConfigMapHash(ctx, instance)
	}
	return "", nil
}

func (r *OGXServerReconciler) resolveSecretRefsHash(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
	envVars []corev1.EnvVar,
) (string, error) {
	if runtimeConfig == nil || !runtimeConfig.Generated || len(envVars) == 0 {
		return "", nil
	}

	entries := make([]string, 0, len(envVars))
	for _, env := range envVars {
		if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
			continue
		}
		selector := env.ValueFrom.SecretKeyRef
		secret := &corev1.Secret{}
		if err := r.Get(ctx, types.NamespacedName{Name: selector.Name, Namespace: instance.Namespace}, secret); err != nil {
			return "", fmt.Errorf("failed to resolve secret %s/%s for env %s: %w", instance.Namespace, selector.Name, env.Name, err)
		}
		// Hash the actual secret data value so that metadata-only changes
		// (label edits, etc.) don't trigger unnecessary pod restarts.
		valHash := sha256.Sum256(secret.Data[selector.Key])
		entries = append(entries, fmt.Sprintf("%s=%s/%s@%s", env.Name, selector.Name, selector.Key, hex.EncodeToString(valHash[:8])))
	}

	if len(entries) == 0 {
		return "", nil
	}

	sort.Strings(entries)
	hash := sha256.Sum256([]byte(strings.Join(entries, "|")))
	return hex.EncodeToString(hash[:8]), nil
}

// reconcileResources reconciles all resources for the OGXServer instance.
//
//nolint:cyclop // Reconcile orchestration intentionally sequences several independent subsystems.
func (r *OGXServerReconciler) reconcileResources(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	// Run adoption logic before manifest reconciliation so that adopted
	// resources are available for the kustomize pipeline to reference.
	adoptResult, err := r.adoptLegacyResources(ctx, instance)
	if err != nil {
		return fmt.Errorf("failed to adopt legacy resources: %w", err)
	}
	if adoptResult.requeue {
		return &requeueError{after: adoptResult.requeueAfter}
	}

	// Reconcile ConfigMaps first
	generated, err := r.reconcileConfigMaps(ctx, instance)
	if err != nil {
		if instance.HasDeclarativeConfig() || instance.Status.ConfigGeneration != nil {
			r.setConfigGeneratedCondition(instance, false, "ConfigGenerationFailed", err.Error())
		}
		return err
	}

	pendingGeneratedConfigMapName := ""
	if generated != nil {
		pendingGeneratedConfigMapName = generatedConfigMapName(instance.Name, generated.ContentHash)
	}
	runtimeConfig := resolveRuntimeConfigRef(instance, pendingGeneratedConfigMapName)

	// Reconcile all manifest-based resources including Deployment, PVC, ServiceAccount, Service, NetworkPolicy.
	// NetworkPolicy ingress rules are configured via the kustomize transformer plugin.
	if err := r.reconcileAllManifestResources(ctx, instance, runtimeConfig); err != nil {
		if generated != nil {
			r.setConfigGeneratedCondition(instance, false, "ConfigGenerationFailed", err.Error())
		}
		return err
	}

	if generated != nil {
		r.setConfigGeneratedCondition(instance, true, "ConfigGenerationSucceeded",
			fmt.Sprintf("Generated config.yaml with %d providers and %d resources", generated.ProviderCount, generated.ResourceCount))
		r.updateConfigGenerationStatus(instance, generated)
		if err := r.cleanupOldGeneratedConfigMaps(ctx, instance, pendingGeneratedConfigMapName); err != nil {
			log.FromContext(ctx).Error(err, "failed to clean up old generated ConfigMaps")
		}
	} else {
		r.clearConfigGenerationStatus(instance, "ConfigGenerationInactive", "Declarative config generation is not active")
	}

	// Reconcile Ingress for external access (not part of kustomize manifests)
	if err := r.reconcileIngress(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile Ingress: %w", err)
	}

	// Clean up adopted networking resources if the annotation was removed.
	// This runs after normal networking reconciliation to avoid delete-before-create
	// gaps during the migration-off path.
	if err := r.cleanupAdoptedNetworking(ctx, instance); err != nil {
		return fmt.Errorf("failed to clean up adopted networking: %w", err)
	}

	return nil
}

// requeueError signals that the reconciler should requeue after a delay
// without reporting an error to the controller runtime.
type requeueError struct {
	after time.Duration
}

func (e *requeueError) Error() string {
	return fmt.Sprintf("requeue after %s", e.after)
}

// terminalError signals a problem that cannot be resolved by retrying.
// The reconciler sets a status condition and stops without requeueing.
type terminalError struct {
	message string
}

func (e *terminalError) Error() string {
	return e.message
}

func (r *OGXServerReconciler) handleSentinelErrors(
	ctx context.Context, instance *ogxiov1beta1.OGXServer, reconcileErr error,
) (ctrl.Result, bool) {
	logger := log.FromContext(ctx)

	var requeueErr *requeueError
	if errors.As(reconcileErr, &requeueErr) {
		if statusUpdateErr := r.updateStatus(ctx, instance, nil); statusUpdateErr != nil {
			logger.Error(statusUpdateErr, "failed to update status during adoption requeue")
		}
		return ctrl.Result{RequeueAfter: requeueErr.after}, true
	}

	var termErr *terminalError
	if errors.As(reconcileErr, &termErr) {
		if statusUpdateErr := r.updateStatus(ctx, instance, nil); statusUpdateErr != nil {
			logger.Error(statusUpdateErr, "failed to update status for terminal error")
		}
		return ctrl.Result{}, true
	}

	return ctrl.Result{}, false
}

func (r *OGXServerReconciler) reconcileConfigMaps(ctx context.Context, instance *ogxiov1beta1.OGXServer) (*config.GeneratedConfig, error) {
	if err := r.reconcileOverrideAndCABundleConfigMaps(ctx, instance); err != nil {
		return nil, err
	}

	// Reconcile operator-generated config (from declarative providers/resources/storage)
	generated, err := r.reconcileGeneratedConfig(ctx, instance)
	if err != nil {
		return nil, fmt.Errorf("failed to reconcile generated config: %w", err)
	}

	if err := r.reconcileManagedCABundle(ctx, instance); err != nil {
		return nil, err
	}

	return generated, nil
}

func (r *OGXServerReconciler) reconcileOverrideAndCABundleConfigMaps(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	if instance.Spec.BaseConfig != nil {
		if err := r.reconcileBaseConfigMap(ctx, instance); err != nil {
			return fmt.Errorf("failed to reconcile base ConfigMap: %w", err)
		}
	}

	if instance.HasOverrideConfig() {
		if err := r.reconcileOverrideConfigMap(ctx, instance); err != nil {
			return fmt.Errorf("failed to reconcile override ConfigMap: %w", err)
		}
	}

	if r.hasCACertificates(instance) {
		if err := r.reconcileCABundleConfigMap(ctx, instance); err != nil {
			return fmt.Errorf("failed to reconcile CA bundle ConfigMap: %w", err)
		}
	}

	return nil
}

func (r *OGXServerReconciler) reconcileManagedCABundle(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)
	managedConfigMapName := getManagedCABundleConfigMapName(instance)

	if !r.hasCACertificates(instance) && !r.hasODHTrustedCABundle(ctx, instance) {
		// No CA bundles configured, delete managed ConfigMap if it exists
		existingConfigMap := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      managedConfigMapName,
			Namespace: instance.Namespace,
		}, existingConfigMap)

		if err == nil {
			// ConfigMap exists but is no longer needed, delete it
			logger.Info("Deleting unused managed CA bundle ConfigMap", "configMap", managedConfigMapName)
			if delErr := r.Delete(ctx, existingConfigMap); delErr != nil && !k8serrors.IsNotFound(delErr) {
				return fmt.Errorf("failed to delete unused managed CA bundle ConfigMap: %w", delErr)
			}
			logger.Info("Successfully deleted unused managed CA bundle ConfigMap", "configMap", managedConfigMapName)
		} else if !k8serrors.IsNotFound(err) {
			// Unexpected error
			return fmt.Errorf("failed to check for managed CA bundle ConfigMap: %w", err)
		}
		return nil
	}

	if err := r.reconcileManagedCABundleConfigMap(ctx, instance); err != nil {
		return fmt.Errorf("failed to reconcile managed CA bundle ConfigMap: %w", err)
	}

	return nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OGXServerReconciler) SetupWithManager(_ context.Context, mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&ogxiov1beta1.OGXServer{}, builder.WithPredicates(predicate.Funcs{
			UpdateFunc: r.ogxServerUpdatePredicate(mgr),
		})).
		Owns(&appsv1.Deployment{}).
		Owns(&policyv1.PodDisruptionBudget{}).
		Owns(&autoscalingv2.HorizontalPodAutoscaler{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		Watches(
			&corev1.ConfigMap{},
			handler.EnqueueRequestsFromMapFunc(r.mapConfigMapToReconcileRequests),
			builder.WithPredicates(r.userConfigMapPredicate()),
		).
		Watches(
			&corev1.Secret{},
			handler.EnqueueRequestsFromMapFunc(r.mapSecretToReconcileRequests),
			builder.WithPredicates(r.userSecretPredicate()),
		).
		Owns(&networkingv1.NetworkPolicy{}).
		Owns(&networkingv1.Ingress{}).
		Owns(&corev1.PersistentVolumeClaim{}).
		Complete(r)
}

// ogxServerUpdatePredicate returns a predicate function for OGXServer updates.
func (r *OGXServerReconciler) ogxServerUpdatePredicate(mgr ctrl.Manager) func(event.UpdateEvent) bool {
	return func(e event.UpdateEvent) bool {
		// Safely type assert old object
		oldObj, ok := e.ObjectOld.(*ogxiov1beta1.OGXServer)
		if !ok {
			return false
		}
		oldObjCopy := oldObj.DeepCopy()

		// Safely type assert new object
		newObj, ok := e.ObjectNew.(*ogxiov1beta1.OGXServer)
		if !ok {
			return false
		}
		newObjCopy := newObj.DeepCopy()

		// Compare only spec, ignoring metadata and status
		if diff := cmp.Diff(oldObjCopy.Spec, newObjCopy.Spec); diff != "" {
			logger := mgr.GetLogger().WithValues("namespace", newObjCopy.Namespace, "name", newObjCopy.Name)
			logger.Info("OGXServer CR spec changed")
			// Note that both the logger and fmt.Printf could appear entangled in the output
			// but there is no simple way to avoid this (forcing the logger to flush its output).
			// When the logger is used to print the diff the output is hard to read,
			// fmt.Printf is better for readability.
			fmt.Printf("%s\n", diff)
		}

		return true
	}
}

// mapConfigMapToReconcileRequests maps a user-opted-in ConfigMap change to the
// OGXServer CR(s) that reference it.
func (r *OGXServerReconciler) mapConfigMapToReconcileRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	configMap, ok := obj.(*corev1.ConfigMap)
	if !ok {
		return nil
	}

	// Skip operator-managed ConfigMaps — they are handled by Owns().
	if configMap.Labels[managedByLabelKey] == managedByLabelVal {
		return nil
	}

	// List relevant OGXServer CRs to find which ones reference this ConfigMap.
	// User ConfigMaps are namespace-scoped per 003 design, so default to same-namespace
	// listing. Keep operator config global since it can affect all instances.
	var instances ogxiov1beta1.OGXServerList
	listOpts := []client.ListOption{client.InNamespace(configMap.Namespace)}
	if configMap.Name == operatorConfigData {
		listOpts = nil
	}
	if err := r.List(ctx, &instances, listOpts...); err != nil {
		logger.Error(err, "failed to list OGXServer instances for ConfigMap mapping")
		return nil
	}

	var requests []reconcile.Request
	for i := range instances.Items {
		instance := &instances.Items[i]
		if r.instanceReferencesConfigMap(instance, configMap.Name, configMap.Namespace) {
			logger.Info("ConfigMap change mapped to OGXServer",
				"configMap", configMap.Name, "configMapNamespace", configMap.Namespace,
				"instance", instance.Name, "instanceNamespace", instance.Namespace)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			})
		}
	}

	return requests
}

// instanceReferencesConfigMap checks if an OGXServer instance references
// a ConfigMap with the given name and namespace.
//
//nolint:cyclop // Aggregates multiple independent ConfigMap reference sources.
func (r *OGXServerReconciler) instanceReferencesConfigMap(
	instance *ogxiov1beta1.OGXServer, cmName, cmNamespace string,
) bool {
	// Override config ConfigMap (always in the CR namespace).
	if instance.HasOverrideConfig() &&
		instance.Spec.OverrideConfig.Name == cmName &&
		instance.Namespace == cmNamespace {
		return true
	}

	// Declarative base config ConfigMap (always in the CR namespace).
	if instance.Spec.BaseConfig != nil &&
		instance.Spec.BaseConfig.Name == cmName &&
		instance.Namespace == cmNamespace {
		return true
	}

	// CA certificate source ConfigMaps.
	if r.referencesCACertificateConfigMap(instance, cmName, cmNamespace) {
		return true
	}

	// ODH trusted CA bundle well-known ConfigMap (same namespace as instance).
	if cmName == odhTrustedCABundleConfigMap && cmNamespace == instance.Namespace {
		return true
	}

	// Operator config well-known ConfigMap.
	return cmName == operatorConfigData && cmNamespace == r.operatorNamespace
}

func (r *OGXServerReconciler) referencesCACertificateConfigMap(instance *ogxiov1beta1.OGXServer, cmName, cmNamespace string) bool {
	if !r.hasCACertificates(instance) || cmNamespace != instance.Namespace {
		return false
	}
	for _, ref := range instance.Spec.TLS.Trust.CACertificates {
		if ref.Name == cmName {
			return true
		}
	}
	return false
}

// mapSecretToReconcileRequests maps a user-opted-in Secret change to
// OGXServer CR(s) that reference that Secret through declarative providers/storage.
func (r *OGXServerReconciler) mapSecretToReconcileRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	logger := log.FromContext(ctx)

	secret, ok := obj.(*corev1.Secret)
	if !ok {
		return nil
	}

	var instances ogxiov1beta1.OGXServerList
	if err := r.List(ctx, &instances, client.InNamespace(secret.Namespace)); err != nil {
		logger.Error(err, "failed to list OGXServer instances for Secret mapping")
		return nil
	}

	var requests []reconcile.Request
	for i := range instances.Items {
		instance := &instances.Items[i]
		if r.instanceReferencesSecret(instance, secret.Name, secret.Namespace) {
			logger.Info("Secret change mapped to OGXServer",
				"secret", secret.Name, "secretNamespace", secret.Namespace,
				"instance", instance.Name, "instanceNamespace", instance.Namespace)
			requests = append(requests, reconcile.Request{
				NamespacedName: types.NamespacedName{
					Name:      instance.Name,
					Namespace: instance.Namespace,
				},
			})
		}
	}

	return requests
}

func (r *OGXServerReconciler) instanceReferencesSecret(instance *ogxiov1beta1.OGXServer, secretName, secretNamespace string) bool {
	if secretNamespace != instance.Namespace {
		return false
	}

	for _, env := range config.CollectSecretRefs(&instance.Spec) {
		if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
			continue
		}
		if env.ValueFrom.SecretKeyRef.Name == secretName {
			return true
		}
	}

	return false
}

// userConfigMapPredicate returns a predicate that accepts only ConfigMaps with
// the watch label and rejects operator-managed ConfigMaps (handled by Owns()).
func (r *OGXServerReconciler) userConfigMapPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isWatchLabeledUserConfigMap(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isWatchLabeledUserConfigMap(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isWatchLabeledUserConfigMap(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return isWatchLabeledUserConfigMap(e.Object)
		},
	}
}

// isWatchLabeledUserConfigMap returns true if the object has the watch label
// and is NOT an operator-managed ConfigMap.
func isWatchLabeledUserConfigMap(obj client.Object) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	// Reject operator-managed ConfigMaps — they are handled by Owns().
	if labels[managedByLabelKey] == managedByLabelVal {
		return false
	}
	return labels[WatchLabelKey] == WatchLabelValue
}

// userSecretPredicate returns a predicate that accepts only user Secrets with
// the watch label and rejects operator-managed objects.
func (r *OGXServerReconciler) userSecretPredicate() predicate.Funcs {
	return predicate.Funcs{
		CreateFunc: func(e event.CreateEvent) bool {
			return isWatchLabeledUserSecret(e.Object)
		},
		UpdateFunc: func(e event.UpdateEvent) bool {
			return isWatchLabeledUserSecret(e.ObjectNew)
		},
		DeleteFunc: func(e event.DeleteEvent) bool {
			return isWatchLabeledUserSecret(e.Object)
		},
		GenericFunc: func(e event.GenericEvent) bool {
			return isWatchLabeledUserSecret(e.Object)
		},
	}
}

// isWatchLabeledUserSecret returns true if the Secret has the watch label
// and is NOT an operator-managed Secret.
func isWatchLabeledUserSecret(obj client.Object) bool {
	labels := obj.GetLabels()
	if labels == nil {
		return false
	}
	if labels[managedByLabelKey] == managedByLabelVal {
		return false
	}
	return labels[WatchLabelKey] == WatchLabelValue
}

// getServerURL returns the URL for the OGX server.
func (r *OGXServerReconciler) getServerURL(instance *ogxiov1beta1.OGXServer, path string) *url.URL {
	serviceName := deploy.GetServiceName(instance)
	port := deploy.GetServicePort(instance)

	return &url.URL{
		Scheme: "http",
		Host:   fmt.Sprintf("%s.%s.svc.cluster.local:%d", serviceName, instance.Namespace, port),
		Path:   path,
	}
}

// getProviderInfo makes an HTTP request to the providers endpoint.
func (r *OGXServerReconciler) getProviderInfo(ctx context.Context, instance *ogxiov1beta1.OGXServer) ([]ogxiov1beta1.ProviderInfo, error) {
	u := r.getServerURL(instance, "/v1/providers")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create providers request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make providers request: %w", err)
	}
	// Close error after successful read is not actionable; anon func required to explicitly discard return value
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to query providers endpoint: returned status code %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read providers response: %w", err)
	}

	var response struct {
		Data []ogxiov1beta1.ProviderInfo `json:"data"`
	}
	if err := json.Unmarshal(body, &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal providers response: %w", err)
	}

	return response.Data, nil
}

// getVersionInfo makes an HTTP request to the version endpoint.
func (r *OGXServerReconciler) getVersionInfo(ctx context.Context, instance *ogxiov1beta1.OGXServer) (string, error) {
	u := r.getServerURL(instance, "/v1/version")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create version request: %w", err)
	}

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make version request: %w", err)
	}
	// Close error after successful read is not actionable; anon func required to explicitly discard return value
	defer func() { _ = resp.Body.Close() }()

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

// updateStatus refreshes the OGXServer status.
func (r *OGXServerReconciler) updateStatus(ctx context.Context, instance *ogxiov1beta1.OGXServer, reconcileErr error) error {
	logger := log.FromContext(ctx)
	instance.Status.Version.OperatorVersion = os.Getenv("OPERATOR_VERSION")
	// A reconciliation error is the highest priority. It overrides all other status checks.
	if reconcileErr != nil {
		instance.Status.Phase = ogxiov1beta1.OGXServerPhaseFailed
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
			instance.Status.Phase = ogxiov1beta1.OGXServerPhaseReady

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
				instance.Status.Version.ServerVersion = version
				logger.V(1).Info("Updated server version from API endpoint", "version", version)
			}

			SetHealthCheckCondition(&instance.Status, true, MessageHealthCheckPassed)
		} else {
			// If not ready, health can't be checked. Set condition appropriately.
			SetHealthCheckCondition(&instance.Status, false, "Deployment not ready")
			instance.Status.DistributionConfig.Providers = nil // Clear providers
		}
	}

	instance.Status.Version.LastUpdated = metav1.NewTime(metav1.Now().UTC())
	desiredStatus := instance.Status.DeepCopy()

	err := retry.RetryOnConflict(retry.DefaultRetry, func() error {
		latest := &ogxiov1beta1.OGXServer{}
		if getErr := r.Get(ctx, client.ObjectKeyFromObject(instance), latest); getErr != nil {
			return getErr
		}
		latest.Status = *desiredStatus
		return r.Status().Update(ctx, latest)
	})
	if err != nil {
		return fmt.Errorf("failed to update status: %w", err)
	}
	return nil
}

func (r *OGXServerReconciler) updateDeploymentStatus(ctx context.Context, instance *ogxiov1beta1.OGXServer) (bool, error) {
	deployment := &appsv1.Deployment{}
	deploymentErr := r.Get(ctx, types.NamespacedName{Name: instance.Name, Namespace: instance.Namespace}, deployment)
	if deploymentErr != nil && !k8serrors.IsNotFound(deploymentErr) {
		return false, fmt.Errorf("failed to fetch deployment for status: %w", deploymentErr)
	}

	deploymentReady := false

	switch {
	case deploymentErr != nil: // This case covers when the deployment is not found
		instance.Status.Phase = ogxiov1beta1.OGXServerPhasePending
		SetDeploymentReadyCondition(&instance.Status, false, MessageDeploymentPending)
	case deployment.Status.ReadyReplicas == 0:
		instance.Status.Phase = ogxiov1beta1.OGXServerPhaseInitializing
		SetDeploymentReadyCondition(&instance.Status, false, MessageDeploymentPending)
	case deployment.Status.ReadyReplicas < deploy.GetEffectiveReplicas(instance):
		instance.Status.Phase = ogxiov1beta1.OGXServerPhaseInitializing
		deploymentMessage := fmt.Sprintf("Deployment is scaling: %d/%d replicas ready", deployment.Status.ReadyReplicas, deploy.GetEffectiveReplicas(instance))
		SetDeploymentReadyCondition(&instance.Status, false, deploymentMessage)
	case deployment.Status.ReadyReplicas > deploy.GetEffectiveReplicas(instance):
		instance.Status.Phase = ogxiov1beta1.OGXServerPhaseInitializing
		deploymentMessage := fmt.Sprintf("Deployment is scaling down: %d/%d replicas ready", deployment.Status.ReadyReplicas, deploy.GetEffectiveReplicas(instance))
		SetDeploymentReadyCondition(&instance.Status, false, deploymentMessage)
	default:
		instance.Status.Phase = ogxiov1beta1.OGXServerPhaseReady
		deploymentReady = true
		SetDeploymentReadyCondition(&instance.Status, true, MessageDeploymentReady)
	}
	instance.Status.AvailableReplicas = deployment.Status.ReadyReplicas
	return deploymentReady, nil
}

func (r *OGXServerReconciler) updateStorageStatus(ctx context.Context, instance *ogxiov1beta1.OGXServer) {
	if instance.Spec.Workload == nil || instance.Spec.Workload.Storage == nil {
		return
	}

	pvcName, err := r.resolveEffectivePVCName(ctx, instance)
	if err != nil {
		SetStorageReadyCondition(&instance.Status, false, fmt.Sprintf("Failed to resolve PVC name: %v", err))
		return
	}

	pvc := &corev1.PersistentVolumeClaim{}
	if err := r.Get(ctx, types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}, pvc); err != nil {
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

func (r *OGXServerReconciler) updateServiceStatus(ctx context.Context, instance *ogxiov1beta1.OGXServer) {
	service := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: instance.Name + "-service", Namespace: instance.Namespace}, service)
	if err != nil {
		SetServiceReadyCondition(&instance.Status, false, fmt.Sprintf("Failed to get Service: %v", err))
		return
	}

	// Set the service URL in the status
	serviceURL := r.getServerURL(instance, "")
	instance.Status.ServiceURL = serviceURL.String()

	// Set the external URL if external access is enabled
	instance.Status.ExternalURL = r.getIngressURL(ctx, instance)

	SetServiceReadyCondition(&instance.Status, true, MessageServiceReady)
}

func (r *OGXServerReconciler) updateDistributionConfig(instance *ogxiov1beta1.OGXServer) {
	instance.Status.DistributionConfig.AvailableDistributions = r.ClusterInfo.DistributionImages
	var activeDistribution string
	if instance.Spec.Distribution.Name != "" {
		activeDistribution = instance.Spec.Distribution.Name
	} else if instance.Spec.Distribution.Image != "" {
		activeDistribution = "custom"
	}
	instance.Status.DistributionConfig.ActiveDistribution = activeDistribution
}

// reconcileOverrideConfigMap validates that the referenced override ConfigMap exists.
func (r *OGXServerReconciler) reconcileOverrideConfigMap(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	if !instance.HasOverrideConfig() {
		logger.V(1).Info("No override ConfigMap specified, skipping")
		return nil
	}

	configMapNamespace := instance.Namespace

	logger.V(1).Info("Validating referenced override ConfigMap exists",
		"configMapName", instance.Spec.OverrideConfig.Name,
		"configMapKey", instance.Spec.OverrideConfig.Key,
		"configMapNamespace", configMapNamespace)

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.OverrideConfig.Name,
		Namespace: configMapNamespace,
	}, configMap)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Error(err, "Referenced override ConfigMap not found",
				"configMapName", instance.Spec.OverrideConfig.Name,
				"configMapNamespace", configMapNamespace)
			return fmt.Errorf("failed to find referenced ConfigMap %s/%s", configMapNamespace, instance.Spec.OverrideConfig.Name)
		}
		return fmt.Errorf("failed to fetch ConfigMap %s/%s: %w", configMapNamespace, instance.Spec.OverrideConfig.Name, err)
	}
	if _, exists := configMap.Data[instance.Spec.OverrideConfig.Key]; !exists {
		return fmt.Errorf(
			"failed to find override ConfigMap key '%s' in ConfigMap %s/%s",
			instance.Spec.OverrideConfig.Key,
			configMapNamespace,
			instance.Spec.OverrideConfig.Name,
		)
	}

	logger.V(1).Info("Override ConfigMap found and validated",
		"configMap", configMap.Name,
		"namespace", configMap.Namespace,
		"key", instance.Spec.OverrideConfig.Key,
		"dataKeys", len(configMap.Data))
	return nil
}

// reconcileBaseConfigMap validates that the referenced declarative base ConfigMap exists.
func (r *OGXServerReconciler) reconcileBaseConfigMap(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	if instance.Spec.BaseConfig == nil {
		logger.V(1).Info("No base ConfigMap specified, skipping")
		return nil
	}

	ref := instance.Spec.BaseConfig
	logger.V(1).Info("Validating referenced base ConfigMap exists",
		"configMapName", ref.Name,
		"configMapKey", ref.Key,
		"configMapNamespace", instance.Namespace)

	if _, err := r.readReferencedConfigMapKey(ctx, instance.Namespace, *ref); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Error(err, "Referenced base ConfigMap not found",
				"configMapName", ref.Name,
				"configMapNamespace", instance.Namespace)
			return fmt.Errorf("failed to find referenced base ConfigMap %s/%s", instance.Namespace, ref.Name)
		}
		if errors.Is(err, errReferencedConfigMapKeyNotFound) {
			return fmt.Errorf("failed to find base ConfigMap key '%s' in ConfigMap %s/%s", ref.Key, instance.Namespace, ref.Name)
		}
		return fmt.Errorf("failed to fetch base ConfigMap %s/%s: %w", instance.Namespace, ref.Name, err)
	}

	logger.V(1).Info("Base ConfigMap found and validated",
		"configMap", ref.Name,
		"namespace", instance.Namespace,
		"key", ref.Key)
	return nil
}

// reconcileCABundleConfigMap validates that referenced CA certificate ConfigMaps exist.
func (r *OGXServerReconciler) reconcileCABundleConfigMap(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	if !r.hasCACertificates(instance) {
		logger.V(1).Info("No CA certificates specified, skipping")
		return nil
	}

	for _, ref := range instance.Spec.TLS.Trust.CACertificates {
		logger.V(1).Info("Validating referenced CA certificate ConfigMap exists",
			"configMapName", ref.Name,
			"configMapKey", ref.Key,
			"configMapNamespace", instance.Namespace)

		configMap := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      ref.Name,
			Namespace: instance.Namespace,
		}, configMap)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				logger.Error(err, "Referenced CA certificate ConfigMap not found",
					"configMapName", ref.Name,
					"configMapNamespace", instance.Namespace)
				return fmt.Errorf("failed to find referenced CA certificate ConfigMap %s/%s", instance.Namespace, ref.Name)
			}
			return fmt.Errorf("failed to fetch CA certificate ConfigMap %s/%s: %w", instance.Namespace, ref.Name, err)
		}

		if _, exists := configMap.Data[ref.Key]; !exists {
			errMissing := fmt.Errorf("failed to find CA certificate key %q in ConfigMap", ref.Key)
			logger.Error(errMissing, "CA certificate key not found in ConfigMap",
				"configMapName", ref.Name,
				"configMapNamespace", instance.Namespace,
				"key", ref.Key)
			return fmt.Errorf("failed to find CA certificate key '%s' in ConfigMap %s/%s", ref.Key, instance.Namespace, ref.Name)
		}

		logger.V(1).Info("CA certificate ConfigMap key found",
			"configMapName", ref.Name,
			"configMapNamespace", instance.Namespace,
			"key", ref.Key)
	}

	logger.V(1).Info("All CA certificate ConfigMaps validated",
		"count", len(instance.Spec.TLS.Trust.CACertificates))
	return nil
}

// getConfigMapHash calculates a hash of the ConfigMap data to detect changes.
func (r *OGXServerReconciler) getConfigMapHash(ctx context.Context, instance *ogxiov1beta1.OGXServer) (string, error) {
	if !instance.HasOverrideConfig() {
		return "", nil
	}

	configMapNamespace := instance.Namespace

	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Spec.OverrideConfig.Name,
		Namespace: configMapNamespace,
	}, configMap)
	if err != nil {
		return "", err
	}

	// Create a content-based hash that will change when the ConfigMap data changes
	return fmt.Sprintf("%s-%s", configMap.ResourceVersion, configMap.Name), nil
}

// getCABundleConfigMapHash calculates a hash of the managed CA bundle ConfigMap to detect changes.
func (r *OGXServerReconciler) getCABundleConfigMapHash(ctx context.Context, instance *ogxiov1beta1.OGXServer) (string, error) {
	// Check if any CA bundles are configured
	if !r.hasCACertificates(instance) && !r.hasODHTrustedCABundle(ctx, instance) {
		return "", nil
	}

	// Get the managed ConfigMap
	managedConfigMapName := getManagedCABundleConfigMapName(instance)
	configMap := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      managedConfigMapName,
		Namespace: instance.Namespace,
	}, configMap)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// ConfigMap doesn't exist yet, return empty hash
			return "", nil
		}
		return "", err
	}

	// Create a content-based hash that will change when the ConfigMap data changes
	return fmt.Sprintf("%s-%s", configMap.ResourceVersion, configMap.Name), nil
}

// hasODHTrustedCABundle checks if the ODH trusted CA bundle ConfigMap exists and has valid keys.
func (r *OGXServerReconciler) hasODHTrustedCABundle(ctx context.Context, instance *ogxiov1beta1.OGXServer) bool {
	_, keys, err := r.detectODHTrustedCABundle(ctx, instance)
	return err == nil && len(keys) > 0
}

// gatherCABundleData collects all CA certificate data from source ConfigMaps and concatenates them.
// This function implements security measures to prevent injection attacks:
// - Validates PEM structure and X.509 certificate format during processing.
// - Enforces size limits to prevent resource exhaustion.
// - Only extracts valid CERTIFICATE blocks using PEM decoder and X.509 parser.
func (r *OGXServerReconciler) gatherCABundleData(ctx context.Context, instance *ogxiov1beta1.OGXServer) (string, error) {
	logger := log.FromContext(ctx)
	collector := &certificateCollector{logger: logger}

	if err := r.gatherExplicitCABundle(ctx, instance, collector); err != nil {
		return "", err
	}

	if err := r.gatherODHCABundle(ctx, instance, collector); err != nil {
		return "", err
	}

	return collector.concatenate()
}

type certificateCollector struct {
	logger           logr.Logger
	certificates     []string
	totalSize        int
	certificateCount int
}

func (c *certificateCollector) add(certs []string, size, count int, configMapName, key string) error {
	c.totalSize += size
	c.certificateCount += count

	if c.totalSize > MaxCABundleSize {
		return fmt.Errorf("failed to process CA bundle: total size exceeds maximum allowed size of %d bytes", MaxCABundleSize)
	}

	if c.certificateCount > MaxCABundleCertificates {
		return fmt.Errorf("failed to process CA bundle: contains more than %d certificates (maximum allowed)", MaxCABundleCertificates)
	}

	c.certificates = append(c.certificates, certs...)
	c.logger.V(1).Info("Processed CA bundle key",
		"configMap", configMapName,
		"key", key,
		"certificates", count,
		"size", size)

	return nil
}

func (c *certificateCollector) concatenate() (string, error) {
	if len(c.certificates) == 0 {
		return "", errors.New("failed to find valid certificates in CA bundle ConfigMaps")
	}

	// Use strings.Builder for efficient memory usage with large bundles
	var builder strings.Builder
	builder.Grow(c.totalSize + len(c.certificates)) // Pre-allocate with space for newlines
	for i, cert := range c.certificates {
		if i > 0 {
			builder.WriteString("\n")
		}
		builder.WriteString(cert)
	}

	concatenated := builder.String()
	c.logger.V(1).Info("Successfully gathered CA bundle data",
		"totalCertificates", c.certificateCount,
		"totalSize", len(concatenated))

	return concatenated, nil
}

func (r *OGXServerReconciler) gatherExplicitCABundle(ctx context.Context, instance *ogxiov1beta1.OGXServer, collector *certificateCollector) error {
	if !r.hasCACertificates(instance) {
		return nil
	}

	for _, ref := range instance.Spec.TLS.Trust.CACertificates {
		configMap := &corev1.ConfigMap{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      ref.Name,
			Namespace: instance.Namespace,
		}, configMap)
		if err != nil {
			return fmt.Errorf("failed to get CA certificate ConfigMap %s/%s: %w",
				instance.Namespace, ref.Name, err)
		}

		if err := r.processConfigMapKeys(configMap, []string{ref.Key}, instance.Namespace, ref.Name, collector); err != nil {
			return err
		}
	}

	return nil
}

func (r *OGXServerReconciler) gatherODHCABundle(ctx context.Context, instance *ogxiov1beta1.OGXServer, collector *certificateCollector) error {
	configMap, keys, err := r.detectODHTrustedCABundle(ctx, instance)
	if err != nil {
		// Log but don't fail - ODH bundle is optional
		collector.logger.V(1).Info("Could not detect ODH trusted CA bundle", "error", err)
		return nil
	}
	if configMap == nil || len(keys) == 0 {
		return nil
	}

	return r.processODHConfigMapKeys(configMap, keys, collector)
}

func (r *OGXServerReconciler) processConfigMapKeys(configMap *corev1.ConfigMap, keys []string, namespace, name string, collector *certificateCollector) error {
	for _, key := range keys {
		data, exists := configMap.Data[key]
		if !exists {
			return fmt.Errorf("failed to find CA bundle key '%s' in ConfigMap %s/%s", key, namespace, name)
		}

		certs, size, count, err := extractValidCertificates([]byte(data), key)
		if err != nil {
			return fmt.Errorf("failed to process CA bundle key '%s' from ConfigMap %s/%s: %w", key, namespace, name, err)
		}

		if err := collector.add(certs, size, count, configMap.Name, key); err != nil {
			return err
		}
	}

	return nil
}

func (r *OGXServerReconciler) processODHConfigMapKeys(configMap *corev1.ConfigMap, keys []string, collector *certificateCollector) error {
	for _, key := range keys {
		data, exists := configMap.Data[key]
		if !exists {
			collector.logger.V(1).Info("ODH CA bundle key not found, skipping", "key", key)
			continue
		}

		certs, size, count, err := extractValidCertificates([]byte(data), key)
		if err != nil {
			collector.logger.Error(err, "Failed to process ODH CA bundle key, skipping",
				"configMap", configMap.Name,
				"key", key)
			continue
		}

		if err := collector.add(certs, size, count, configMap.Name, key); err != nil {
			return err
		}
	}

	return nil
}

// extractValidCertificates extracts only valid CERTIFICATE blocks from PEM data.
// This function validates PEM structure and X.509 certificate format for all blocks.
// It filters out non-certificate PEM blocks (e.g., private keys, public keys) and
// rejects invalid X.509 certificates.
// Returns: (certificates as strings, total size, certificate count, error).
func extractValidCertificates(data []byte, keyName string) ([]string, int, int, error) {
	// Trim whitespace to detect effectively empty data.
	// Empty or whitespace-only data is valid (e.g., ODH bundle with no custom CAs).
	trimmed := bytes.TrimSpace(data)
	if len(trimmed) == 0 {
		return nil, 0, 0, nil
	}

	var certificates []string
	totalSize := 0
	remaining := data

	for {
		block, rest := pem.Decode(remaining)
		if block == nil {
			break
		}

		// Only accept CERTIFICATE blocks, reject other PEM types
		if block.Type != "CERTIFICATE" {
			// Skip non-certificate blocks (could be private keys, etc.)
			remaining = rest
			continue
		}

		// Validate that this is actually a valid X.509 certificate
		if _, err := x509.ParseCertificate(block.Bytes); err != nil {
			return nil, 0, 0, fmt.Errorf("failed to parse X.509 certificate from key '%s': %w", keyName, err)
		}

		// Re-encode the certificate to ensure it's properly formatted
		certPEM := pem.EncodeToMemory(block)
		if certPEM == nil {
			return nil, 0, 0, fmt.Errorf("failed to encode certificate from key '%s'", keyName)
		}

		certificates = append(certificates, string(certPEM))
		totalSize += len(certPEM)
		remaining = rest
	}

	if len(certificates) == 0 {
		return nil, 0, 0, fmt.Errorf("failed to find valid certificates in CA bundle key '%s'", keyName)
	}

	return certificates, totalSize, len(certificates), nil
}

// reconcileManagedCABundleConfigMap creates or updates the managed CA bundle ConfigMap.
func (r *OGXServerReconciler) reconcileManagedCABundleConfigMap(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)

	// Gather all CA certificate data
	caBundleData, err := r.gatherCABundleData(ctx, instance)
	if err != nil {
		return fmt.Errorf("failed to gather CA bundle data: %w", err)
	}

	managedConfigMapName := getManagedCABundleConfigMapName(instance)

	// Check if the managed ConfigMap already exists
	existingConfigMap := &corev1.ConfigMap{}
	err = r.Get(ctx, types.NamespacedName{
		Name:      managedConfigMapName,
		Namespace: instance.Namespace,
	}, existingConfigMap)

	if err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to get managed CA bundle ConfigMap: %w", err)
	}

	// Create the desired ConfigMap
	desiredConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      managedConfigMapName,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				managedByLabelKey:             managedByLabelVal,
				"app.kubernetes.io/instance":  instance.Name,
				"app.kubernetes.io/component": "ca-bundle",
				WatchLabelKey:                 WatchLabelValue,
			},
		},
		Data: map[string]string{
			ManagedCABundleKey: caBundleData,
		},
	}

	// Set owner reference so the ConfigMap is deleted when the OGXServer is deleted
	if refErr := ctrl.SetControllerReference(instance, desiredConfigMap, r.Scheme); refErr != nil {
		return fmt.Errorf("failed to set controller reference on managed CA bundle ConfigMap: %w", refErr)
	}

	if k8serrors.IsNotFound(err) {
		// ConfigMap doesn't exist, create it
		logger.Info("Creating managed CA bundle ConfigMap", "configMap", managedConfigMapName)
		if err := r.Create(ctx, desiredConfigMap); err != nil {
			return fmt.Errorf("failed to create managed CA bundle ConfigMap: %w", err)
		}
		logger.Info("Successfully created managed CA bundle ConfigMap", "configMap", managedConfigMapName)
	} else {
		// ConfigMap exists, update it if the data has changed
		if existingConfigMap.Data[ManagedCABundleKey] != caBundleData {
			logger.Info("Updating managed CA bundle ConfigMap", "configMap", managedConfigMapName)
			// Use Patch instead of Update to avoid race conditions
			patch := client.MergeFrom(existingConfigMap.DeepCopy())
			existingConfigMap.Data = desiredConfigMap.Data
			existingConfigMap.Labels = desiredConfigMap.Labels
			if err := r.Patch(ctx, existingConfigMap, patch); err != nil {
				if k8serrors.IsConflict(err) {
					// Conflict detected, will be retried by controller
					return fmt.Errorf("failed to patch managed CA bundle ConfigMap (conflict): %w", err)
				}
				return fmt.Errorf("failed to patch managed CA bundle ConfigMap: %w", err)
			}
			logger.Info("Successfully updated managed CA bundle ConfigMap", "configMap", managedConfigMapName)
		} else {
			logger.V(1).Info("Managed CA bundle ConfigMap is up to date", "configMap", managedConfigMapName)
		}
	}

	return nil
}

// detectODHTrustedCABundle checks if the well-known ODH trusted CA bundle ConfigMap
// exists in the same namespace as the OGXServer and returns its available keys.
// Returns the ConfigMap and a list of data keys if found, or nil and empty slice if not found.
func (r *OGXServerReconciler) detectODHTrustedCABundle(ctx context.Context, instance *ogxiov1beta1.OGXServer) (*corev1.ConfigMap, []string, error) {
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

	// Extract available data keys
	// PEM data is validated in extractValidCertificates()
	// which properly validates all PEM blocks.
	keys := make([]string, 0, len(configMap.Data))

	for key := range configMap.Data {
		keys = append(keys, key)
		logger.V(1).Info("Auto-detected CA bundle key",
			"configMapName", odhTrustedCABundleConfigMap,
			"namespace", instance.Namespace,
			"key", key)
	}

	logger.V(1).Info("ODH trusted CA bundle ConfigMap detected",
		"configMapName", odhTrustedCABundleConfigMap,
		"namespace", instance.Namespace,
		"availableKeys", keys)

	return configMap, keys, nil
}

// NewOGXServerReconciler creates a new reconciler with default image mappings.
func NewOGXServerReconciler(cachedClient client.Client, scheme *runtime.Scheme,
	clusterInfo *cluster.ClusterInfo, imageMappingOverrides map[string]string,
	operatorNamespace string) *OGXServerReconciler {
	ociLabelFetcher := config.NewOCILabelFetcher()

	return &OGXServerReconciler{
		Client:                cachedClient,
		Scheme:                scheme,
		ImageMappingOverrides: imageMappingOverrides,
		ClusterInfo:           clusterInfo,
		httpClient:            &http.Client{Timeout: 5 * time.Second},
		OCILabelFetcher:       ociLabelFetcher,
		configResolver:        config.NewDefaultConfigResolver(ociLabelFetcher),
		operatorNamespace:     operatorNamespace,
	}
}

// InitializeOperatorConfigMap gets or creates the operator config ConfigMap.
func InitializeOperatorConfigMap(ctx context.Context, c client.Client, operatorNamespace string) (*corev1.ConfigMap, error) {
	configMap := &corev1.ConfigMap{}
	configMapName := types.NamespacedName{
		Name:      operatorConfigData,
		Namespace: operatorNamespace,
	}

	err := c.Get(ctx, configMapName, configMap)
	if err == nil {
		if configMap.Labels == nil || configMap.Labels[WatchLabelKey] != WatchLabelValue {
			if configMap.Labels == nil {
				configMap.Labels = make(map[string]string)
			}
			configMap.Labels[WatchLabelKey] = WatchLabelValue
			if updateErr := c.Update(ctx, configMap); updateErr != nil {
				return nil, fmt.Errorf("failed to add watch label to operator config ConfigMap: %w", updateErr)
			}
		}
		return configMap, nil
	}

	if !k8serrors.IsNotFound(err) {
		return nil, fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	configMap = &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName.Name,
			Namespace: configMapName.Namespace,
			Labels: map[string]string{
				WatchLabelKey: WatchLabelValue,
			},
		},
		Data: map[string]string{},
	}

	if err = c.Create(ctx, configMap); err != nil {
		return nil, fmt.Errorf("failed to create ConfigMap: %w", err)
	}

	return configMap, nil
}

func ParseImageMappingOverrides(ctx context.Context, configMapData map[string]string) map[string]string {
	imageMappingOverrides := make(map[string]string)
	logger := log.FromContext(ctx)

	// Look for the image-overrides key in the ConfigMap data
	if overridesYAML, exists := configMapData["image-overrides"]; exists {
		// Parse the YAML content
		var overrides map[string]string
		if err := yaml.Unmarshal([]byte(overridesYAML), &overrides); err != nil {
			// Log error but continue with empty overrides
			logger.V(1).Info("failed to parse image-overrides YAML", "error", err)
			return imageMappingOverrides
		}

		// Validate and copy the parsed overrides to our result map
		for version, image := range overrides {
			// Validate the image reference format
			if _, err := name.ParseReference(image); err != nil {
				logger.V(1).Info(
					"skipping invalid image override",
					"version", version,
					"image", image,
					"error", err,
				)
				continue
			}
			imageMappingOverrides[version] = image
		}
	}

	return imageMappingOverrides
}

// NewTestReconciler creates a reconciler for testing, allowing injection of a custom http client.
func NewTestReconciler(client client.Client, scheme *runtime.Scheme, clusterInfo *cluster.ClusterInfo,
	httpClient *http.Client) *OGXServerReconciler {
	return &OGXServerReconciler{
		Client:                client,
		Scheme:                scheme,
		ClusterInfo:           clusterInfo,
		httpClient:            httpClient,
		ImageMappingOverrides: make(map[string]string),
	}
}

// MapConfigMapToReconcileRequests is an exported wrapper for mapConfigMapToReconcileRequests, for testing.
func (r *OGXServerReconciler) MapConfigMapToReconcileRequests(ctx context.Context, obj client.Object) []reconcile.Request {
	return r.mapConfigMapToReconcileRequests(ctx, obj)
}

// UserConfigMapPredicate is an exported wrapper for userConfigMapPredicate, for testing.
func (r *OGXServerReconciler) UserConfigMapPredicate() predicate.Funcs {
	return r.userConfigMapPredicate()
}
