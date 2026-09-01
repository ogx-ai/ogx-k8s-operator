package controllers

import (
	"context"
	"fmt"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// updatePraxisPreflightStatus sets the Praxis-mode readiness conditions on the instance status.
// Because OGX and Praxis are deployed independently (the operator does not manage Praxis), unmet
// preconditions are surfaced as status conditions rather than admission rejections: a Praxis
// instance may legitimately come up after OGX, and a TLS Secret may be created out of band.
// Callers must only invoke this in Praxis-fronted mode.
func (r *OGXServerReconciler) updatePraxisPreflightStatus(ctx context.Context, instance *ogxiov1beta1.OGXServer) {
	r.updatePraxisReachableCondition(ctx, instance)
	r.updateTLSConfiguredCondition(ctx, instance)
}

// updatePraxisReachableCondition sets ConditionTypePraxisReachable based on whether the effective
// Praxis selector (spec.praxisMode.praxisSelector, or the fail-safe default) resolves to at least
// one Ready Praxis pod. Without a reachable Praxis instance, internal-only OGX has no valid path.
func (r *OGXServerReconciler) updatePraxisReachableCondition(ctx context.Context, instance *ogxiov1beta1.OGXServer) {
	logger := log.FromContext(ctx)
	namespace, selector := resolvePraxisPodSelector(instance)

	labelSelector, err := metav1.LabelSelectorAsSelector(selector)
	if err != nil {
		SetPraxisReachableCondition(&instance.Status, false, ReasonPraxisSelectorInvalid,
			fmt.Sprintf("Invalid spec.praxisMode.praxisSelector: %v", err))
		return
	}

	// Praxis pods are not cached by the operator, so read them via the non-cached API reader
	// when available (falling back to the cached client, e.g. in tests using a fake client).
	var podReader client.Reader = r.Client
	if r.APIReader != nil {
		podReader = r.APIReader
	}

	podList := &corev1.PodList{}
	if err := podReader.List(ctx, podList,
		client.InNamespace(namespace),
		client.MatchingLabelsSelector{Selector: labelSelector},
	); err != nil {
		logger.Error(err, "failed to list Praxis pods for preflight check", "namespace", namespace)
		SetPraxisReachableCondition(&instance.Status, false, ReasonPraxisUnreachable,
			fmt.Sprintf("Failed to list Praxis pods in namespace %q: %v", namespace, err))
		return
	}

	readyCount := 0
	for i := range podList.Items {
		if isPodReady(&podList.Items[i]) {
			readyCount++
		}
	}

	if readyCount == 0 {
		SetPraxisReachableCondition(&instance.Status, false, ReasonPraxisUnreachable,
			fmt.Sprintf("No Ready Praxis pods match the selector in namespace %q; OGX is internal-only "+
				"and has no valid path until Praxis is available", namespace))
		return
	}

	SetPraxisReachableCondition(&instance.Status, true, ReasonPraxisReachable,
		fmt.Sprintf("%d Ready Praxis pod(s) match the selector in namespace %q", readyCount, namespace))
}

// updateTLSConfiguredCondition sets ConditionTypeTLSConfigured based on whether
// spec.network.tls.secretName is set and the referenced Secret exists. In Praxis mode OGX is
// expected to serve its internal endpoint over TLS, so a missing secret conditions readiness.
func (r *OGXServerReconciler) updateTLSConfiguredCondition(ctx context.Context, instance *ogxiov1beta1.OGXServer) {
	secretName := ""
	if instance.Spec.Network != nil && instance.Spec.Network.TLS != nil {
		secretName = instance.Spec.Network.TLS.SecretName
	}

	if secretName == "" {
		SetTLSConfiguredCondition(&instance.Status, false, ReasonTLSNotConfigured,
			"spec.network.tls.secretName is not set; Praxis-fronted OGX is expected to serve its "+
				"internal endpoint over TLS")
		return
	}

	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: secretName, Namespace: instance.Namespace}, secret); err != nil {
		if k8serrors.IsNotFound(err) {
			SetTLSConfiguredCondition(&instance.Status, false, ReasonTLSSecretMissing,
				fmt.Sprintf("TLS Secret %q not found in namespace %q (it must exist and carry the "+
					"label %s=%s to be detected by the operator)", secretName, instance.Namespace,
					WatchLabelKey, WatchLabelValue))
			return
		}
		SetTLSConfiguredCondition(&instance.Status, false, ReasonTLSSecretMissing,
			fmt.Sprintf("Failed to get TLS Secret %q in namespace %q: %v", secretName, instance.Namespace, err))
		return
	}

	SetTLSConfiguredCondition(&instance.Status, true, ReasonTLSConfigured,
		fmt.Sprintf("TLS Secret %q found", secretName))
}

// resolvePraxisPodSelector returns the namespace and Pod label selector that identify the Praxis
// instance fronting this OGXServer. It mirrors BuildPraxisPeer: spec.praxisMode.praxisSelector is
// honored when set, otherwise the fail-safe default (app: payload-processing in the default Praxis
// namespace) is used.
func resolvePraxisPodSelector(instance *ogxiov1beta1.OGXServer) (string, *metav1.LabelSelector) {
	if instance.Spec.PraxisMode != nil && instance.Spec.PraxisMode.PraxisSelector != nil {
		sel := instance.Spec.PraxisMode.PraxisSelector
		podSelector := sel.PodSelector
		return sel.Namespace, &podSelector
	}
	return ogxiov1beta1.DefaultPraxisNamespace, &metav1.LabelSelector{
		MatchLabels: map[string]string{
			ogxiov1beta1.DefaultLabelKey: ogxiov1beta1.DefaultPraxisPodLabelValue,
		},
	}
}

// isPodReady reports whether a pod is Running, not terminating, and has a True Ready condition.
func isPodReady(pod *corev1.Pod) bool {
	if pod.DeletionTimestamp != nil || pod.Status.Phase != corev1.PodRunning {
		return false
	}
	for _, cond := range pod.Status.Conditions {
		if cond.Type == corev1.PodReady {
			return cond.Status == corev1.ConditionTrue
		}
	}
	return false
}
