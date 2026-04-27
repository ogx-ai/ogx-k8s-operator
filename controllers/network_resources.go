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
	llamav1alpha1 "github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
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

// buildIngress creates an Ingress for external access to the LlamaStackDistribution.
func (r *LlamaStackDistributionReconciler) buildIngress(
	instance *llamav1alpha1.LlamaStackDistribution,
) (*networkingv1.Ingress, error) {
	servicePort := deploy.GetServicePort(instance)
	serviceName := deploy.GetServiceName(instance)

	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + IngressNameSuffix,
			Namespace: instance.Namespace,
			Labels: map[string]string{
				"app.kubernetes.io/managed-by": "llama-stack-operator",
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

// reconcileIngress creates, updates, or deletes the Ingress based on exposeRoute setting.
func (r *LlamaStackDistributionReconciler) reconcileIngress(
	ctx context.Context,
	instance *llamav1alpha1.LlamaStackDistribution,
) error {
	logger := log.FromContext(ctx)
	ingressName := instance.Name + IngressNameSuffix

	existing := &networkingv1.Ingress{}
	err := r.Get(ctx, types.NamespacedName{Name: ingressName, Namespace: instance.Namespace}, existing)
	existsAlready := err == nil

	exposeRoute := instance.Spec.Network != nil && instance.Spec.Network.ExposeRoute

	if !exposeRoute {
		return r.handleDisabledIngress(ctx, instance, existing, existsAlready, ingressName)
	}

	return r.handleEnabledIngress(ctx, instance, existing, err, existsAlready, ingressName, logger)
}

// handleDisabledIngress handles Ingress deletion when exposeRoute is false.
func (r *LlamaStackDistributionReconciler) handleDisabledIngress(
	ctx context.Context,
	instance *llamav1alpha1.LlamaStackDistribution,
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

	logger.Info("Deleting Ingress as exposeRoute is disabled", "name", ingressName)
	if err := r.Delete(ctx, existing); err != nil && !k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete Ingress: %w", err)
	}

	return nil
}

// handleEnabledIngress handles Ingress creation/update when exposeRoute is true.
func (r *LlamaStackDistributionReconciler) handleEnabledIngress(
	ctx context.Context,
	instance *llamav1alpha1.LlamaStackDistribution,
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
func (r *LlamaStackDistributionReconciler) getIngressURL(
	ctx context.Context,
	instance *llamav1alpha1.LlamaStackDistribution,
) *string {
	if instance.Spec.Network == nil || !instance.Spec.Network.ExposeRoute {
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
func (r *LlamaStackDistributionReconciler) BuildIngressForTest(
	instance *llamav1alpha1.LlamaStackDistribution,
) (*networkingv1.Ingress, error) {
	return r.buildIngress(instance)
}
