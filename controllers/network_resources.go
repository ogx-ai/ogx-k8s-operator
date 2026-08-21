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

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	// IngressNameSuffix is the suffix for the Ingress name.
	IngressNameSuffix = "-ingress"
)

// reconcileIngress enforces internal-only access for OGX. In the Praxis-fronted topology OGX
// is an internal backend reached only from Praxis, so the operator never creates external
// exposure regardless of spec.network.externalAccess. Any operator-owned Ingress previously
// created for this instance is removed. The operator only ever created Ingress resources for
// external access (there is no OpenShift Route or Gateway API HTTPRoute to remove).
func (r *OGXServerReconciler) reconcileIngress(
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

// ReconcileIngressForTest exposes reconcileIngress for unit testing.
func (r *OGXServerReconciler) ReconcileIngressForTest(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	return r.reconcileIngress(ctx, instance)
}
