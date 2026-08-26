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
	"net/url"

	"github.com/go-logr/logr"
	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/deploy"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// IngressNameSuffix is the suffix for the Ingress name.
	IngressNameSuffix = "-ingress"
)

// resolvePraxisMode reports whether this instance runs in the internal-only, Praxis-fronted
// posture. The mode is driven solely by spec.praxisMode.enabled:
//   - enabled: true  → Praxis-fronted (internal-only).
//   - enabled: false → legacy behavior.
//   - spec.praxisMode or spec.praxisMode.enabled unset → legacy behavior.
//
// A brand-new (greenfield) CR gets spec.praxisMode.enabled defaulted to true by the mutating
// admission webhook on create (see api/v1beta1/ogxserver_webhook.go). An unset value here therefore
// means a CR created before the webhook existed (an operator upgrade) or one for which the webhook
// did not run; both are treated as legacy so an upgrade never silently cuts off co-located workloads.
func (r *OGXServerReconciler) resolvePraxisMode(instance *ogxiov1beta1.OGXServer) bool {
	return instance.Spec.PraxisMode != nil &&
		instance.Spec.PraxisMode.Enabled != nil && *instance.Spec.PraxisMode.Enabled
}

// buildIngress creates an Ingress for external access to the OGXServer.
func (r *OGXServerReconciler) buildIngress(
	instance *ogxiov1beta1.OGXServer,
) (*networkingv1.Ingress, error) {
	servicePort := deploy.GetServicePort(instance)
	serviceName := deploy.GetServiceName(instance)

	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + IngressNameSuffix,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "ogx-operator",
				"app.kubernetes.io/instance":   instance.Name,
			},
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{
				{
					IngressRuleValue: networkingv1.IngressRuleValue{
						HTTP: &networkingv1.HTTPIngressRuleValue{
							Paths: []networkingv1.HTTPIngressPath{
								{
									Path:     "/",
									PathType: &pathType,
									Backend: networkingv1.IngressBackend{
										Service: &networkingv1.IngressServiceBackend{
											Name: serviceName,
											Port: networkingv1.ServiceBackendPort{
												Number: servicePort,
											},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := ctrl.SetControllerReference(instance, ingress, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set controller reference: %w", err)
	}

	return ingress, nil
}

// reconcileIngress reconciles the Ingress according to the deployment mode. In Praxis mode OGX
// is an internal backend reached only from Praxis, so external exposure is enforced off. In
// legacy mode the Ingress is created/updated/deleted based on spec.network.externalAccess.
func (r *OGXServerReconciler) reconcileIngress(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
) error {
	if r.resolvePraxisMode(instance) {
		return r.enforceInternalOnlyIngress(ctx, instance)
	}
	return r.reconcileLegacyIngress(ctx, instance)
}

// enforceInternalOnlyIngress removes any operator-owned Ingress for this instance. In the
// Praxis-fronted topology OGX is reached only from Praxis, so the operator never creates
// external exposure. The operator only ever created Ingress resources for external access
// (there is no OpenShift Route or Gateway API HTTPRoute to remove).
func (r *OGXServerReconciler) enforceInternalOnlyIngress(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
) error {
	logger := log.FromContext(ctx)
	ingressName := instance.Name + IngressNameSuffix

	existing := &networkingv1.Ingress{}
	if err := r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: instance.Namespace}, existing); err != nil {
		if k8serrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to get Ingress: %w", err)
	}

	if !metav1.IsControlledBy(existing, instance) {
		logger.V(1).Info("Ingress not owned by this instance, skipping deletion", "name", ingressName)
		return nil
	}

	logger.Info("Deleting Ingress: OGX is internal-only (Praxis-fronted), external access is not exposed",
		"name", ingressName)
	if err := r.Delete(ctx, existing); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Ingress: %w", err)
	}

	return nil
}

// reconcileLegacyIngress creates, updates, or deletes the Ingress based on the expose setting
// (pre-Praxis behavior).
func (r *OGXServerReconciler) reconcileLegacyIngress(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
) error {
	logger := log.FromContext(ctx)
	ingressName := instance.Name + IngressNameSuffix

	existing := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: instance.Namespace}, existing)
	existsAlready := err == nil

	expose := instance.Spec.Network != nil && instance.Spec.Network.ExternalAccess != nil && instance.Spec.Network.ExternalAccess.Enabled

	if !expose {
		return r.handleDisabledIngress(ctx, instance, existing, existsAlready, ingressName)
	}

	return r.handleEnabledIngress(ctx, instance, existing, err, existsAlready, ingressName, logger)
}

// handleDisabledIngress handles Ingress deletion when expose is not set.
func (r *OGXServerReconciler) handleDisabledIngress(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	existing *networkingv1.Ingress,
	existsAlready bool,
	ingressName string,
) error {
	logger := log.FromContext(ctx)

	if !existsAlready {
		return nil
	}

	if !metav1.IsControlledBy(existing, instance) {
		logger.V(1).Info("Ingress not owned by this instance, skipping deletion", "name", ingressName)
		return nil
	}

	logger.Info("Deleting Ingress as expose is disabled", "name", ingressName)
	if err := r.Delete(ctx, existing); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Ingress: %w", err)
	}

	return nil
}

// handleEnabledIngress handles Ingress creation/update when expose is set.
func (r *OGXServerReconciler) handleEnabledIngress(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	existing *networkingv1.Ingress,
	getErr error,
	existsAlready bool,
	ingressName string,
	logger logr.Logger,
) error {
	ingress, buildErr := r.buildIngress(instance)
	if buildErr != nil {
		return buildErr
	}

	if !existsAlready {
		if k8serrors.IsNotFound(getErr) {
			logger.Info("Creating Ingress for external access", "name", ingressName)
			if createErr := r.Create(ctx, ingress); createErr != nil {
				return fmt.Errorf("failed to create Ingress: %w", createErr)
			}
			return nil
		}
		return fmt.Errorf("failed to get Ingress: %w", getErr)
	}

	if !metav1.IsControlledBy(existing, instance) {
		logger.V(1).Info("Ingress not owned by this instance, skipping update", "name", ingressName)
		return nil
	}

	ingress.ResourceVersion = existing.ResourceVersion
	if err := r.Update(ctx, ingress); err != nil {
		return fmt.Errorf("failed to update Ingress: %w", err)
	}
	logger.V(1).Info("Updated Ingress", "name", ingressName)

	return nil
}

// getIngressURL returns the external URL from an Ingress if available.
func (r *OGXServerReconciler) getIngressURL(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
) *string {
	if instance.Spec.Network == nil || instance.Spec.Network.ExternalAccess == nil || !instance.Spec.Network.ExternalAccess.Enabled {
		return nil
	}

	ingress := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{
		Name:      instance.Name + IngressNameSuffix,
		Namespace: instance.Namespace,
	}, ingress)
	if err != nil {
		empty := ""
		return &empty // Ingress not ready yet
	}

	// Check for LoadBalancer ingress
	if len(ingress.Status.LoadBalancer.Ingress) > 0 {
		lb := ingress.Status.LoadBalancer.Ingress[0]
		if lb.Hostname != "" {
			return buildURLString(lb.Hostname)
		}
		if lb.IP != "" {
			return buildURLString(lb.IP)
		}
	}

	// Check for host in rules
	if len(ingress.Spec.Rules) > 0 && ingress.Spec.Rules[0].Host != "" {
		return buildURLString(ingress.Spec.Rules[0].Host)
	}

	empty := ""
	return &empty
}

// buildURLString constructs an HTTP URL from a host and returns a pointer to it.
func buildURLString(host string) *string {
	u := &url.URL{
		Scheme: "http",
		Host:   host,
	}
	s := u.String()
	return &s
}

// BuildIngressForTest is a test helper that exposes buildIngress for unit testing.
func (r *OGXServerReconciler) BuildIngressForTest(
	instance *ogxiov1beta1.OGXServer,
) (*networkingv1.Ingress, error) {
	return r.buildIngress(instance)
}

// ReconcileIngressForTest exposes reconcileIngress for unit testing.
func (r *OGXServerReconciler) ReconcileIngressForTest(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	return r.reconcileIngress(ctx, instance)
}
