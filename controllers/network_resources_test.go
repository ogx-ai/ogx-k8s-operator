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

package controllers_test

import (
	"context"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/stretchr/testify/require"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newIngressTestReconciler(t *testing.T, objs ...client.Object) (*controllers.OGXServerReconciler, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	clusterInfo := &cluster.ClusterInfo{
		DistributionImages: map[string]string{"starter": "test-image:latest"},
	}
	return controllers.NewTestReconciler(c, scheme, clusterInfo, nil), c
}

func newInternalOnlyInstance() *ogxiov1beta1.OGXServer {
	return &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ogx",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
			// External access requested, but OGX is internal-only so it must not be honored.
			Network: &ogxiov1beta1.NetworkSpec{
				ExternalAccess: &ogxiov1beta1.ExternalAccessConfig{Enabled: true},
			},
		},
	}
}

// TestReconcileIngress_DoesNotCreateWhenExternalAccessEnabled verifies OGX is enforced
// internal-only: even with externalAccess.enabled=true, no Ingress is created.
func TestReconcileIngress_DoesNotCreateWhenExternalAccessEnabled(t *testing.T) {
	instance := newInternalOnlyInstance()
	reconciler, c := newIngressTestReconciler(t)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))

	var ing networkingv1.Ingress
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      instance.Name + controllers.IngressNameSuffix,
		Namespace: instance.Namespace,
	}, &ing)
	require.True(t, apierrors.IsNotFound(err), "no Ingress should be created for an internal-only OGX")
}

// TestReconcileIngress_DeletesPreviouslyCreatedIngress verifies an operator-owned Ingress
// left over from a prior (externally-exposed) deployment is removed on reconcile.
func TestReconcileIngress_DeletesPreviouslyCreatedIngress(t *testing.T) {
	instance := newInternalOnlyInstance()

	existing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + controllers.IngressNameSuffix,
			Namespace: instance.Namespace,
		},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, ctrl.SetControllerReference(instance, existing, scheme))

	reconciler, c := newIngressTestReconciler(t, existing)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))

	var ing networkingv1.Ingress
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      existing.Name,
		Namespace: existing.Namespace,
	}, &ing)
	require.True(t, apierrors.IsNotFound(err), "previously-created owned Ingress should be deleted")
}

// TestReconcileIngress_LeavesUnownedIngress verifies the operator does not delete an Ingress
// it does not own (no controller reference to this instance).
func TestReconcileIngress_LeavesUnownedIngress(t *testing.T) {
	instance := newInternalOnlyInstance()

	unowned := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + controllers.IngressNameSuffix,
			Namespace: instance.Namespace,
		},
	}

	reconciler, c := newIngressTestReconciler(t, unowned)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))

	var ing networkingv1.Ingress
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      unowned.Name,
		Namespace: unowned.Namespace,
	}, &ing)
	require.NoError(t, err, "an Ingress not owned by this instance must not be deleted")
}
