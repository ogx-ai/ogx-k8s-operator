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
	"time"

	legacyv1alpha1 "github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// adoptionResult carries the outcome of a single adoption pass.
type adoptionResult struct {
	requeue      bool
	requeueAfter time.Duration
}

// adoptLegacyResources processes adoption annotations and transfers ownership
// of legacy LlamaStackDistribution resources to this OGXServer CR.
// Returns (needsRequeue, error). A requeue is requested when the old Deployment
// has been scaled to zero but pods are still terminating.
func (r *OGXServerReconciler) adoptLegacyResources(ctx context.Context, instance *ogxiov1beta1.OGXServer) (adoptionResult, error) {
	logger := log.FromContext(ctx).WithValues("adoption", true)
	ctx = ctrl.LoggerInto(ctx, logger)

	result := adoptionResult{}

	storageSource := instance.GetAdoptStorageSource()
	networkingSource := instance.GetAdoptNetworkingSource()

	// Always clear stale invalid condition when there are no active annotations.
	if storageSource == "" && networkingSource == "" {
		clearAdoptionConfigInvalidCondition(&instance.Status)
		return result, nil
	}

	// Validate annotations before any adoption work.
	if !validateAdoptionSources(ctx, &instance.Status, instance.Name, storageSource, networkingSource) {
		return result, nil
	}

	// Clear any previous AdoptionConfigInvalid condition.
	clearAdoptionConfigInvalidCondition(&instance.Status)

	storageResult, err := r.adoptStorageSource(ctx, instance, storageSource)
	if err != nil {
		return result, err
	}
	if storageResult.requeue {
		result = storageResult
	}

	if err := r.adoptNetworkingSource(ctx, instance, networkingSource); err != nil {
		return result, err
	}

	return result, nil
}

func validateAdoptionSources(
	ctx context.Context,
	status *ogxiov1beta1.OGXServerStatus,
	instanceName, storageSource, networkingSource string,
) bool {
	if !validateAdoptionSource(ctx, status, "adopt-storage", storageSource, instanceName) {
		return false
	}

	return validateAdoptionSource(ctx, status, "adopt-networking", networkingSource, instanceName)
}

func validateAdoptionSource(
	ctx context.Context,
	status *ogxiov1beta1.OGXServerStatus,
	annotationName, value, instanceName string,
) bool {
	if value == "" {
		return true
	}

	if value == instanceName {
		logger := log.FromContext(ctx)
		logger.Info("adoption annotation value equals CR name, rejecting", "annotation", annotationName, "value", value)
		SetAdoptionConfigInvalidCondition(status, fmt.Sprintf(
			"%s: value %q must not equal the CR name; same-name adoption causes resource conflicts", annotationName, value))
		return false
	}

	if err := ogxiov1beta1.ValidateAdoptionAnnotation(value); err != nil {
		logger := log.FromContext(ctx)
		logger.Error(err, "invalid adoption annotation value", "annotation", annotationName, "value", value)
		SetAdoptionConfigInvalidCondition(status, fmt.Sprintf("%s: %v", annotationName, err))
		return false
	}

	return true
}

func (r *OGXServerReconciler) adoptStorageSource(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	storageSource string,
) (adoptionResult, error) {
	if storageSource == "" {
		return adoptionResult{}, nil
	}

	storageResult, err := r.adoptStorage(ctx, instance, storageSource)
	if err != nil {
		return adoptionResult{}, fmt.Errorf("failed to adopt storage from %q: %w", storageSource, err)
	}

	return storageResult, nil
}

func (r *OGXServerReconciler) adoptNetworkingSource(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	networkingSource string,
) error {
	if networkingSource == "" {
		return nil
	}

	if err := r.adoptNetworking(ctx, instance, networkingSource); err != nil {
		return fmt.Errorf("failed to adopt networking from %q: %w", networkingSource, err)
	}

	return nil
}

// adoptStorage transfers ownership of a legacy PVC to this OGXServer CR.
// If the old Deployment is still running, it is scaled to zero first
// (requeue is requested to wait for pod termination).
func (r *OGXServerReconciler) adoptStorage(ctx context.Context, instance *ogxiov1beta1.OGXServer, legacyName string) (adoptionResult, error) {
	logger := log.FromContext(ctx)
	result := adoptionResult{}

	pvcName := legacyName + "-pvc"
	pvc := &corev1.PersistentVolumeClaim{}
	pvcKey := types.NamespacedName{Name: pvcName, Namespace: instance.Namespace}

	if err := r.Get(ctx, pvcKey, pvc); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("Legacy PVC not found, skipping storage adoption", "pvc", pvcName)
			return result, nil
		}
		return result, fmt.Errorf("failed to get legacy PVC %s: %w", pvcName, err)
	}

	// Idempotency: already owned by this CR.
	if metav1.IsControlledBy(pvc, instance) {
		logger.V(1).Info("Legacy PVC already adopted", "pvc", pvcName)
		SetStorageAdoptedCondition(&instance.Status, true, fmt.Sprintf("PVC %s adopted", pvcName))
		return result, nil
	}

	// Scale legacy Deployment to zero if it still exists, to release the RWO PVC.
	requeue, err := r.scaleDownLegacyDeployment(ctx, instance.Namespace, legacyName)
	if err != nil {
		return result, err
	}
	if requeue {
		result.requeue = true
		result.requeueAfter = 5 * time.Second
		logger.Info("Waiting for legacy pods to terminate before PVC adoption", "deployment", legacyName)
		return result, nil
	}

	// Transfer ownership: remove any existing controller ownerRef, then set ours.
	if err := r.transferOwnership(ctx, instance, pvc, legacyName); err != nil {
		return result, err
	}

	SetStorageAdoptedCondition(&instance.Status, true, fmt.Sprintf("PVC %s adopted", pvcName))
	logger.Info("Successfully adopted legacy PVC", "pvc", pvcName)
	return result, nil
}

// adoptNetworking transfers ownership of a legacy Service and Ingress to this OGXServer CR.
func (r *OGXServerReconciler) adoptNetworking(ctx context.Context, instance *ogxiov1beta1.OGXServer, legacyName string) error {
	serviceAdopted, err := r.adoptLegacyService(ctx, instance, legacyName)
	if err != nil {
		return err
	}
	ingressAdopted, err := r.adoptLegacyIngress(ctx, instance, legacyName)
	if err != nil {
		return err
	}

	if serviceAdopted || ingressAdopted {
		SetNetworkingAdoptedCondition(&instance.Status, true, fmt.Sprintf("Networking adopted from %s", legacyName))
	}

	return nil
}

func (r *OGXServerReconciler) adoptLegacyService(ctx context.Context, instance *ogxiov1beta1.OGXServer, legacyName string) (bool, error) {
	logger := log.FromContext(ctx)
	serviceName := legacyName + "-service"
	svc := &corev1.Service{}
	svcKey := types.NamespacedName{Name: serviceName, Namespace: instance.Namespace}

	if err := r.Get(ctx, svcKey, svc); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("Legacy Service not found, skipping", "service", serviceName)
			return false, nil
		}
		return false, fmt.Errorf("failed to get legacy Service %s: %w", serviceName, err)
	}

	if metav1.IsControlledBy(svc, instance) {
		logger.V(1).Info("Legacy Service already adopted", "service", serviceName)
		return true, nil
	}

	// Update selectors to route traffic to new pods.
	if svc.Spec.Selector == nil {
		svc.Spec.Selector = make(map[string]string)
	}
	svc.Spec.Selector[ogxiov1beta1.DefaultLabelKey] = ogxiov1beta1.DefaultLabelValue
	svc.Spec.Selector[instanceLabelKey] = instance.Name

	if err := r.transferOwnership(ctx, instance, svc, legacyName); err != nil {
		return false, err
	}
	logger.Info("Successfully adopted legacy Service", "service", serviceName)

	return true, nil
}

func (r *OGXServerReconciler) adoptLegacyIngress(ctx context.Context, instance *ogxiov1beta1.OGXServer, legacyName string) (bool, error) {
	logger := log.FromContext(ctx)
	ingressName := legacyName + "-ingress"
	ingress := &networkingv1.Ingress{}
	ingressKey := types.NamespacedName{Name: ingressName, Namespace: instance.Namespace}

	if err := r.Get(ctx, ingressKey, ingress); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.Info("Legacy Ingress not found, skipping", "ingress", ingressName)
			return false, nil
		}
		return false, fmt.Errorf("failed to get legacy Ingress %s: %w", ingressName, err)
	}

	if metav1.IsControlledBy(ingress, instance) {
		logger.V(1).Info("Legacy Ingress already adopted", "ingress", ingressName)
		return true, nil
	}

	if err := r.transferOwnership(ctx, instance, ingress, legacyName); err != nil {
		return false, err
	}
	logger.Info("Successfully adopted legacy Ingress", "ingress", ingressName)

	return true, nil
}

// scaleDownLegacyDeployment scales the legacy Deployment to zero and returns
// true if a requeue is needed to wait for pod termination.
func (r *OGXServerReconciler) scaleDownLegacyDeployment(ctx context.Context, namespace, legacyName string) (bool, error) {
	logger := log.FromContext(ctx)

	deployment := &appsv1.Deployment{}
	key := types.NamespacedName{Name: legacyName, Namespace: namespace}

	if err := r.Get(ctx, key, deployment); err != nil {
		if k8serrors.IsNotFound(err) {
			logger.V(1).Info("Legacy Deployment not found, proceeding with PVC adoption", "deployment", legacyName)
			return false, nil
		}
		return false, fmt.Errorf("failed to get legacy Deployment %s: %w", legacyName, err)
	}

	// Scale to zero if not already.
	zero := int32(0)
	if deployment.Spec.Replicas == nil || *deployment.Spec.Replicas != 0 {
		logger.Info("Scaling legacy Deployment to zero", "deployment", legacyName)
		patch := client.MergeFrom(deployment.DeepCopy())
		deployment.Spec.Replicas = &zero
		if err := r.Patch(ctx, deployment, patch); err != nil {
			return false, fmt.Errorf("failed to scale down legacy Deployment %s: %w", legacyName, err)
		}
	}

	// Check if pods are still running.
	podList := &corev1.PodList{}
	if err := r.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabels{instanceLabelKey: legacyName},
	); err != nil {
		return false, fmt.Errorf("failed to list pods for legacy Deployment %s: %w", legacyName, err)
	}

	if len(podList.Items) > 0 {
		logger.Info("Legacy pods still terminating", "deployment", legacyName, "podCount", len(podList.Items))
		return true, nil
	}

	return false, nil
}

// transferOwnership removes the existing controller ownerRef (if any) and sets
// this OGXServer as the new controller owner. Also annotates the resource with
// adoption audit metadata. legacySource is the LLSD name the resource was
// adopted from (used in the ogx.io/adopted-from annotation).
func (r *OGXServerReconciler) transferOwnership(ctx context.Context, instance *ogxiov1beta1.OGXServer, obj client.Object, legacySource string) error {
	resourceName := obj.GetName()
	controllerRef := metav1.GetControllerOf(obj)

	// Safety check: only take over resources that are currently controlled by
	// the expected legacy LLSD controller. This avoids accidental takeover of
	// unrelated resources that happen to share names.
	if controllerRef != nil && !isExpectedLegacyOwnerRef(controllerRef, legacySource) {
		return fmt.Errorf(
			"failed to adopt %s: unexpected controller owner %s/%s %q",
			resourceName, controllerRef.APIVersion, controllerRef.Kind, controllerRef.Name,
		)
	}

	// Remove existing controller ownerRef.
	ownerRefs := obj.GetOwnerReferences()
	filtered := make([]metav1.OwnerReference, 0, len(ownerRefs))
	for i := range ownerRefs {
		if ownerRefs[i].Controller != nil && *ownerRefs[i].Controller {
			continue
		}
		filtered = append(filtered, ownerRefs[i])
	}
	obj.SetOwnerReferences(filtered)

	// Set adoption audit annotations.
	annotations := obj.GetAnnotations()
	if annotations == nil {
		annotations = make(map[string]string)
	}
	annotations[ogxiov1beta1.AdoptedFromAnnotation] = legacySource
	annotations[ogxiov1beta1.AdoptedAtAnnotation] = metav1.Now().UTC().Format(time.RFC3339)
	obj.SetAnnotations(annotations)

	// Set new controller ownerRef.
	if err := ctrl.SetControllerReference(instance, obj, r.Scheme); err != nil {
		return fmt.Errorf("failed to set controller reference on %s: %w", resourceName, err)
	}

	if err := r.Update(ctx, obj); err != nil {
		return fmt.Errorf("failed to update %s after ownership transfer: %w", resourceName, err)
	}

	return nil
}

// cleanupAdoptedNetworking deletes adopted legacy networking resources when the
// adopt-networking annotation is removed. Since same-name adoption is rejected
// at admission time, adopted resources always have different names from the
// kustomize-created ones and must be explicitly deleted.
func (r *OGXServerReconciler) cleanupAdoptedNetworking(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	// Only clean up if the annotation has been removed.
	if instance.GetAdoptNetworkingSource() != "" {
		return nil
	}
	// if this instance never reported networking adoption, there is
	// nothing to clean up.
	if !IsConditionTrue(&instance.Status, ConditionTypeNetworkingAdopted) {
		return nil
	}

	if err := r.cleanupAdoptedServices(ctx, instance); err != nil {
		return err
	}

	if err := r.cleanupAdoptedIngresses(ctx, instance); err != nil {
		return err
	}

	// Cleanup pass completed successfully and adoption annotation is absent;
	// clear condition to avoid repeated full namespace scans on steady-state
	// reconciliations.
	SetNetworkingAdoptedCondition(&instance.Status, false, "Adoption annotation removed")

	return nil
}

func (r *OGXServerReconciler) cleanupAdoptedServices(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)
	ownedServices := &corev1.ServiceList{}
	if err := r.List(ctx, ownedServices, client.InNamespace(instance.Namespace)); err != nil {
		return fmt.Errorf("failed to list services for adoption cleanup: %w", err)
	}

	for i := range ownedServices.Items {
		svc := &ownedServices.Items[i]
		if !shouldDeleteAdoptedResource(instance, svc.GetAnnotations(), svc) {
			continue
		}
		logger.Info("Deleting adopted legacy Service after annotation removal", "service", svc.Name)
		if err := r.Delete(ctx, svc); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete adopted Service %s: %w", svc.Name, err)
		}
	}

	return nil
}

func (r *OGXServerReconciler) cleanupAdoptedIngresses(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	logger := log.FromContext(ctx)
	ownedIngresses := &networkingv1.IngressList{}
	if err := r.List(ctx, ownedIngresses, client.InNamespace(instance.Namespace)); err != nil {
		return fmt.Errorf("failed to list ingresses for adoption cleanup: %w", err)
	}

	for i := range ownedIngresses.Items {
		ing := &ownedIngresses.Items[i]
		if !shouldDeleteAdoptedResource(instance, ing.GetAnnotations(), ing) {
			continue
		}
		logger.Info("Deleting adopted legacy Ingress after annotation removal", "ingress", ing.Name)
		if err := r.Delete(ctx, ing); err != nil && !k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to delete adopted Ingress %s: %w", ing.Name, err)
		}
	}

	return nil
}

func shouldDeleteAdoptedResource(
	instance *ogxiov1beta1.OGXServer,
	annotations map[string]string,
	obj metav1.Object,
) bool {
	if !metav1.IsControlledBy(obj, instance) {
		return false
	}
	_, hasAdopted := annotations[ogxiov1beta1.AdoptedFromAnnotation]
	return hasAdopted
}

// --- Condition helpers for adoption ---

// SetStorageAdoptedCondition sets or updates the StorageAdopted condition.
func SetStorageAdoptedCondition(status *ogxiov1beta1.OGXServerStatus, adopted bool, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeStorageAdopted,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonStorageAdopted,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !adopted {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SetNetworkingAdoptedCondition sets or updates the NetworkingAdopted condition.
func SetNetworkingAdoptedCondition(status *ogxiov1beta1.OGXServerStatus, adopted bool, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeNetworkingAdopted,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonNetworkingAdopted,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !adopted {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SetAdoptionConfigInvalidCondition sets the AdoptionConfigInvalid condition to True.
func SetAdoptionConfigInvalidCondition(status *ogxiov1beta1.OGXServerStatus, message string) {
	SetCondition(status, metav1.Condition{
		Type:               ConditionTypeAdoptionConfigInvalid,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonAdoptionConfigInvalid,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	})
}

// clearAdoptionConfigInvalidCondition sets AdoptionConfigInvalid to False
// when annotations are valid, removing a previously-set warning.
func clearAdoptionConfigInvalidCondition(status *ogxiov1beta1.OGXServerStatus) {
	existing := GetCondition(status, ConditionTypeAdoptionConfigInvalid)
	if existing == nil || existing.Status == metav1.ConditionFalse {
		return
	}
	SetCondition(status, metav1.Condition{
		Type:               ConditionTypeAdoptionConfigInvalid,
		Status:             metav1.ConditionFalse,
		Reason:             "AnnotationsValid",
		Message:            "Adoption annotations are valid",
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	})
}

func isExpectedLegacyOwnerRef(ownerRef *metav1.OwnerReference, legacyName string) bool {
	if ownerRef == nil {
		return false
	}
	return ownerRef.APIVersion == legacyv1alpha1.GroupVersion.String() &&
		ownerRef.Kind == legacyv1alpha1.LlamaStackDistributionKind &&
		ownerRef.Name == legacyName
}
