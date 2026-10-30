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
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func newIngressTestReconciler(t *testing.T, objs ...client.Object) (*controllers.OGXServerReconciler, client.Client) {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))

	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	clusterInfo := &cluster.ClusterInfo{
		DistributionImages: map[string]string{"starter": "test-image:latest"},
	}
	return controllers.NewTestReconciler(c, scheme, clusterInfo, nil), c
}

func newInternalOnlyInstance() *ogxiov1beta1.OGXServer {
	praxisOn := true
	return &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ogx",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
			PraxisMode:   &ogxiov1beta1.PraxisModeSpec{Enabled: &praxisOn},
			// External access requested, but OGX is internal-only so it must not be honored.
			Network: &ogxiov1beta1.NetworkSpec{
				ExternalAccess: &ogxiov1beta1.ExternalAccessConfig{
					Enabled:  true,
					Hostname: "ogx.example.com",
					TLS:      &ogxiov1beta1.TLSSpec{SecretName: "ogx-tls"},
				},
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
	require.NoError(t, corev1.AddToScheme(scheme))
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

// TestReconcileIngress_LegacyModeCreatesIngress verifies that with praxisMode explicitly
// disabled the operator preserves the legacy behavior and creates an Ingress when external
// access is enabled.
func TestReconcileIngress_LegacyModeCreatesIngress(t *testing.T) {
	instance := newInternalOnlyInstance()
	legacy := false
	instance.Spec.PraxisMode = &ogxiov1beta1.PraxisModeSpec{Enabled: &legacy}

	reconciler, c := newIngressTestReconciler(t)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))

	var ing networkingv1.Ingress
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      instance.Name + controllers.IngressNameSuffix,
		Namespace: instance.Namespace,
	}, &ing)
	require.NoError(t, err, "legacy mode with externalAccess enabled should create an Ingress")
	require.Len(t, ing.Spec.TLS, 1)
	require.Equal(t, "ogx-tls", ing.Spec.TLS[0].SecretName)
	require.Equal(t, []string{"ogx.example.com"}, ing.Spec.TLS[0].Hosts)
}

func TestReconcileIngress_EnabledUnsetUsesPraxisMode(t *testing.T) {
	instance := newInternalOnlyInstance()
	instance.Spec.PraxisMode = &ogxiov1beta1.PraxisModeSpec{}

	reconciler, c := newIngressTestReconciler(t)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))

	var ing networkingv1.Ingress
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      instance.Name + controllers.IngressNameSuffix,
		Namespace: instance.Namespace,
	}, &ing)
	require.True(t, apierrors.IsNotFound(err), "praxisMode with enabled omitted should use Praxis mode")
}

// TestReconcileIngress_UnsetModeUsesLegacy verifies that an unset praxisMode resolves to legacy
// mode.
func TestReconcileIngress_UnsetModeUsesLegacy(t *testing.T) {
	instance := newInternalOnlyInstance()
	instance.Spec.PraxisMode = nil

	reconciler, c := newIngressTestReconciler(t)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))

	var ing networkingv1.Ingress
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      instance.Name + controllers.IngressNameSuffix,
		Namespace: instance.Namespace,
	}, &ing)
	require.NoError(t, err, "an unset praxisMode should default to legacy mode and honor external access")
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

func newLegacyInstance() *ogxiov1beta1.OGXServer {
	instance := newInternalOnlyInstance()
	legacy := false
	instance.Spec.PraxisMode = &ogxiov1beta1.PraxisModeSpec{Enabled: &legacy}
	return instance
}

func ownedTestIngress(t *testing.T, instance *ogxiov1beta1.OGXServer) *networkingv1.Ingress {
	t.Helper()
	existing := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + controllers.IngressNameSuffix,
			Namespace: instance.Namespace,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{Host: "ogx.example.com"}},
		},
	}
	scheme := runtime.NewScheme()
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))
	require.NoError(t, networkingv1.AddToScheme(scheme))
	require.NoError(t, ctrl.SetControllerReference(instance, existing, scheme))
	return existing
}

func assertNoIngress(t *testing.T, c client.Client, instance *ogxiov1beta1.OGXServer) {
	t.Helper()
	var ing networkingv1.Ingress
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      instance.Name + controllers.IngressNameSuffix,
		Namespace: instance.Namespace,
	}, &ing)
	require.True(t, apierrors.IsNotFound(err), "expected no Ingress, got %v", err)
}

func TestReconcileIngress_MissingTLSDoesNotCreate(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.TLS = nil
	reconciler, c := newIngressTestReconciler(t)
	rec := events.NewFakeRecorder(10)
	reconciler.Recorder = rec

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))
	assertNoIngress(t, c, instance)
	require.NotEmpty(t, rec.Events)
	require.Contains(t, <-rec.Events, controllers.ReasonInsecureExternalAccess)
}

func TestReconcileIngress_EmptySecretNameDoesNotCreate(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.TLS = &ogxiov1beta1.TLSSpec{SecretName: ""}
	reconciler, c := newIngressTestReconciler(t)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))
	assertNoIngress(t, c, instance)
}

func TestReconcileIngress_NoHostnameDoesNotCreate(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.Hostname = ""
	reconciler, c := newIngressTestReconciler(t)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))
	assertNoIngress(t, c, instance)
}

func TestReconcileIngress_DeletesInsecureOwnedIngress(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.TLS = nil
	existing := ownedTestIngress(t, instance)
	reconciler, c := newIngressTestReconciler(t, existing)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))
	assertNoIngress(t, c, instance)
}

func TestReconcileIngress_PreservesUnownedInsecureIngress(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.TLS = nil
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

func TestReconcileIngress_ValidTLSCreatesIngressWithoutSecretLookup(t *testing.T) {
	instance := newLegacyInstance()
	reconciler, c := newIngressTestReconciler(t)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))

	var ing networkingv1.Ingress
	err := c.Get(context.Background(), types.NamespacedName{
		Name:      instance.Name + controllers.IngressNameSuffix,
		Namespace: instance.Namespace,
	}, &ing)
	require.NoError(t, err)
	require.Len(t, ing.Spec.TLS, 1)
	require.Equal(t, "ogx-tls", ing.Spec.TLS[0].SecretName)
}

func TestReconcileIngress_ValidTLSKeepsIngressWhenSecretMissing(t *testing.T) {
	instance := newLegacyInstance()
	existing := ownedTestIngress(t, instance)
	existing.Spec.TLS = []networkingv1.IngressTLS{{
		Hosts:      []string{"ogx.example.com"},
		SecretName: "ogx-tls",
	}}
	reconciler, c := newIngressTestReconciler(t, existing)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))

	var ing networkingv1.Ingress
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{
		Name:      existing.Name,
		Namespace: existing.Namespace,
	}, &ing))
	require.Len(t, ing.Spec.TLS, 1)
	require.Equal(t, "ogx-tls", ing.Spec.TLS[0].SecretName)
}

func TestReconcileIngress_DisabledAccessDeletesOwnedIngress(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.Enabled = false
	existing := ownedTestIngress(t, instance)
	reconciler, c := newIngressTestReconciler(t, existing)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))
	assertNoIngress(t, c, instance)
}

func TestReconcileIngress_DisabledAccessPreservesUnownedIngress(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.Enabled = false
	unowned := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      instance.Name + controllers.IngressNameSuffix,
			Namespace: instance.Namespace,
		},
	}
	reconciler, c := newIngressTestReconciler(t, unowned)

	require.NoError(t, reconciler.ReconcileIngressForTest(context.Background(), instance))
	var ing networkingv1.Ingress
	require.NoError(t, c.Get(context.Background(), types.NamespacedName{
		Name:      unowned.Name,
		Namespace: unowned.Namespace,
	}, &ing))
}

func TestReconcileResources_CleansInsecureOwnedIngressDespiteManifestFailure(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.TLS = nil
	instance.Spec.OverrideConfig = &ogxiov1beta1.ConfigMapKeyRef{
		Name: "missing-override",
		Key:  "config.yaml",
	}
	existing := ownedTestIngress(t, instance)
	reconciler, c := newIngressTestReconciler(t, existing)

	err := reconciler.ReconcileResourcesForTest(context.Background(), instance)
	require.Error(t, err)
	require.Contains(t, err.Error(), "missing-override")
	assertNoIngress(t, c, instance)
}

func TestBuildIngress_RejectsMissingTLS(t *testing.T) {
	instance := newLegacyInstance()
	instance.Spec.Network.ExternalAccess.TLS = nil
	reconciler, _ := newIngressTestReconciler(t)

	ing, err := reconciler.BuildIngressForTest(instance)
	require.Error(t, err)
	require.Nil(t, ing)
	require.Contains(t, err.Error(), "TLS secretName")
}
