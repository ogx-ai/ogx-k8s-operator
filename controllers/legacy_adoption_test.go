package controllers_test

import (
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

func TestAdoptStorage(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name     string
		setup    func(t *testing.T, ns string)
		instance func(ns string) *ogxiov1beta1.OGXServer
		assertFn func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer)
	}{
		{
			name: "happy path - adopt existing PVC",
			setup: func(t *testing.T, ns string) {
				t.Helper()
				createLegacyPVC(t, ns, "my-old-llsd")
			},
			instance: func(ns string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("my-server").
					WithNamespace(ns).
					WithStorage(DefaultTestStorage()).
					WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "my-old-llsd").
					Build()
			},
			assertFn: func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				pvc := &corev1.PersistentVolumeClaim{}
				key := types.NamespacedName{Name: "my-old-llsd-pvc", Namespace: ns}
				require.Eventually(t, func() bool {
					return k8sClient.Get(t.Context(), key, pvc) == nil &&
						pvc.Labels[ogxiov1beta1.AdoptedFromLabel] == "my-old-llsd"
				}, testTimeout, testInterval, "PVC should have adopted-from label")

				require.Nil(t, metav1.GetControllerOf(pvc),
					"Adopted PVC should not have a controller ownerRef")
				require.NotEmpty(t, pvc.Annotations[ogxiov1beta1.AdoptedAtAnnotation])

				assertConditionTrue(t, instance, "StorageAdopted")
			},
		},
		{
			name:  "PVC not found - skip adoption, no error",
			setup: func(t *testing.T, ns string) { t.Helper() },
			instance: func(ns string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("my-server").
					WithNamespace(ns).
					WithStorage(DefaultTestStorage()).
					WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "nonexistent").
					Build()
			},
			assertFn: func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				pvc := &corev1.PersistentVolumeClaim{}
				key := types.NamespacedName{Name: "nonexistent-pvc", Namespace: ns}
				require.True(t, apierrors.IsNotFound(k8sClient.Get(t.Context(), key, pvc)),
					"Legacy PVC should not exist")
			},
		},
		{
			name: "idempotency - second reconcile makes no changes",
			setup: func(t *testing.T, ns string) {
				t.Helper()
				createLegacyPVC(t, ns, "idempotent-llsd")
			},
			instance: func(ns string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("my-server").
					WithNamespace(ns).
					WithStorage(DefaultTestStorage()).
					WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "idempotent-llsd").
					Build()
			},
			assertFn: func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer) {
				t.Helper()

				pvc := &corev1.PersistentVolumeClaim{}
				key := types.NamespacedName{Name: "idempotent-llsd-pvc", Namespace: ns}
				require.Eventually(t, func() bool {
					return k8sClient.Get(t.Context(), key, pvc) == nil &&
						pvc.Labels[ogxiov1beta1.AdoptedFromLabel] == "idempotent-llsd"
				}, testTimeout, testInterval, "PVC should have adopted-from label after first reconcile")

				firstVersion := pvc.ResourceVersion

				// Reconcile again.
				ReconcileOGXServer(t, instance)

				require.NoError(t, k8sClient.Get(t.Context(), key, pvc))
				require.Equal(t, firstVersion, pvc.ResourceVersion,
					"PVC should not change on second reconcile (idempotent)")
			},
		},
		{
			name: "old Deployment already gone - proceed to adoption",
			setup: func(t *testing.T, ns string) {
				t.Helper()
				createLegacyPVC(t, ns, "gone-deploy")
			},
			instance: func(ns string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("my-server").
					WithNamespace(ns).
					WithStorage(DefaultTestStorage()).
					WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "gone-deploy").
					Build()
			},
			assertFn: func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				pvc := &corev1.PersistentVolumeClaim{}
				key := types.NamespacedName{Name: "gone-deploy-pvc", Namespace: ns}
				require.Eventually(t, func() bool {
					return k8sClient.Get(t.Context(), key, pvc) == nil &&
						pvc.Labels[ogxiov1beta1.AdoptedFromLabel] == "gone-deploy"
				}, testTimeout, testInterval, "PVC should be adopted even though Deployment is gone")

				require.Nil(t, metav1.GetControllerOf(pvc),
					"Adopted PVC should not have a controller ownerRef")
			},
		},
		{
			name: "old Deployment still running - scale to zero",
			setup: func(t *testing.T, ns string) {
				t.Helper()
				createLegacyPVC(t, ns, "running-llsd")
				createLegacyDeployment(t, ns, "running-llsd")
			},
			instance: func(ns string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("my-server").
					WithNamespace(ns).
					WithStorage(DefaultTestStorage()).
					WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "running-llsd").
					Build()
			},
			assertFn: func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				// After first reconcile, old Deployment should be scaled to zero.
				deployment := &appsv1.Deployment{}
				key := types.NamespacedName{Name: "running-llsd", Namespace: ns}
				require.Eventually(t, func() bool {
					if err := k8sClient.Get(t.Context(), key, deployment); err != nil {
						return false
					}
					return deployment.Spec.Replicas != nil && *deployment.Spec.Replicas == 0
				}, testTimeout, testInterval, "Legacy Deployment should be scaled to zero")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := createTestNamespace(t, "adopt-storage")
			tt.setup(t, namespace.Name)

			instance := tt.instance(namespace.Name)
			require.NoError(t, k8sClient.Create(t.Context(), instance))
			t.Cleanup(func() {
				if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
					t.Logf("Cleanup: %v", err)
				}
			})

			ReconcileOGXServer(t, instance)

			// Re-read instance for updated status.
			require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
				Name: instance.Name, Namespace: instance.Namespace,
			}, instance))

			if tt.assertFn != nil {
				tt.assertFn(t, namespace.Name, instance)
			}
		})
	}
}

func TestAdoptNetworking(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name     string
		setup    func(t *testing.T, ns string)
		instance func(ns string) *ogxiov1beta1.OGXServer
		assertFn func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer)
	}{
		{
			name: "adopt Service - selectors updated",
			setup: func(t *testing.T, ns string) {
				t.Helper()
				createLegacyService(t, ns, "old-llsd")
			},
			instance: func(ns string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("new-server").
					WithNamespace(ns).
					WithAnnotation(ogxiov1beta1.AdoptNetworkingAnnotation, "old-llsd").
					Build()
			},
			assertFn: func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				svc := &corev1.Service{}
				key := types.NamespacedName{Name: "old-llsd-service", Namespace: ns}
				require.Eventually(t, func() bool {
					return k8sClient.Get(t.Context(), key, svc) == nil &&
						metav1.IsControlledBy(svc, instance)
				}, testTimeout, testInterval, "Service should be owned by new instance")

				require.Equal(t, ogxiov1beta1.DefaultLabelValue, svc.Spec.Selector[ogxiov1beta1.DefaultLabelKey],
					"Service selector should be updated to new label")
				require.Equal(t, instance.Name, svc.Spec.Selector["app.kubernetes.io/instance"],
					"Service selector should target the new instance")

				require.Equal(t, "old-llsd", svc.Labels[ogxiov1beta1.AdoptedFromLabel])
				require.NotEmpty(t, svc.Annotations[ogxiov1beta1.AdoptedAtAnnotation])

				assertConditionTrue(t, instance, "NetworkingAdopted")
			},
		},
		{
			name: "adopt Ingress - ownerRef transferred",
			setup: func(t *testing.T, ns string) {
				t.Helper()
				createLegacyIngress(t, ns, "old-llsd")
			},
			instance: func(ns string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("new-server").
					WithNamespace(ns).
					WithAnnotation(ogxiov1beta1.AdoptNetworkingAnnotation, "old-llsd").
					Build()
			},
			assertFn: func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				ingress := &networkingv1.Ingress{}
				key := types.NamespacedName{Name: "old-llsd-ingress", Namespace: ns}
				require.Eventually(t, func() bool {
					return k8sClient.Get(t.Context(), key, ingress) == nil &&
						metav1.IsControlledBy(ingress, instance)
				}, testTimeout, testInterval, "Ingress should be owned by new instance")

				require.Equal(t, "old-llsd", ingress.Labels[ogxiov1beta1.AdoptedFromLabel])
				require.NotEmpty(t, ingress.Annotations[ogxiov1beta1.AdoptedAtAnnotation])
			},
		},
		{
			name: "name mismatch - adopted and new resources coexist",
			setup: func(t *testing.T, ns string) {
				t.Helper()
				createLegacyService(t, ns, "legacy-name")
			},
			instance: func(ns string) *ogxiov1beta1.OGXServer {
				return NewOGXServerBuilder().
					WithName("new-name").
					WithNamespace(ns).
					WithAnnotation(ogxiov1beta1.AdoptNetworkingAnnotation, "legacy-name").
					Build()
			},
			assertFn: func(t *testing.T, ns string, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				// Adopted legacy Service should exist.
				legacySvc := &corev1.Service{}
				legacyKey := types.NamespacedName{Name: "legacy-name-service", Namespace: ns}
				require.Eventually(t, func() bool {
					return k8sClient.Get(t.Context(), legacyKey, legacySvc) == nil &&
						metav1.IsControlledBy(legacySvc, instance)
				}, testTimeout, testInterval, "Legacy Service should be adopted")

				// New Service created by kustomize pipeline should also exist.
				newSvc := &corev1.Service{}
				newKey := types.NamespacedName{Name: "new-name-service", Namespace: ns}
				require.Eventually(t, func() bool {
					return k8sClient.Get(t.Context(), newKey, newSvc) == nil
				}, testTimeout, testInterval, "New Service should also be created by kustomize pipeline")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := createTestNamespace(t, "adopt-net")
			tt.setup(t, namespace.Name)

			instance := tt.instance(namespace.Name)
			require.NoError(t, k8sClient.Create(t.Context(), instance))
			t.Cleanup(func() {
				if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
					t.Logf("Cleanup: %v", err)
				}
			})

			ReconcileOGXServer(t, instance)

			require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
				Name: instance.Name, Namespace: instance.Namespace,
			}, instance))

			if tt.assertFn != nil {
				tt.assertFn(t, namespace.Name, instance)
			}
		})
	}
}

func TestAdoptionAnnotationValidation(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))

	tests := []struct {
		name           string
		annotationKey  string
		annotationVal  string
		conditionCheck func(t *testing.T, instance *ogxiov1beta1.OGXServer)
	}{
		{
			name:          "empty value",
			annotationKey: ogxiov1beta1.AdoptStorageAnnotation,
			annotationVal: "",
			conditionCheck: func(t *testing.T, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				// Empty annotation value means GetAdoptStorageSource() returns "",
				// so adoption is not triggered at all. No condition should be set.
			},
		},
		{
			name:          "uppercase characters",
			annotationKey: ogxiov1beta1.AdoptStorageAnnotation,
			annotationVal: "MyUpperCase",
			conditionCheck: func(t *testing.T, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				assertConditionTrue(t, instance, "AdoptionConfigInvalid")
			},
		},
		{
			name:          "special characters",
			annotationKey: ogxiov1beta1.AdoptStorageAnnotation,
			annotationVal: "name_with_underscores",
			conditionCheck: func(t *testing.T, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				assertConditionTrue(t, instance, "AdoptionConfigInvalid")
			},
		},
		{
			name:          "too long (>63 chars)",
			annotationKey: ogxiov1beta1.AdoptStorageAnnotation,
			annotationVal: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa",
			conditionCheck: func(t *testing.T, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				assertConditionTrue(t, instance, "AdoptionConfigInvalid")
			},
		},
		{
			name:          "invalid networking annotation",
			annotationKey: ogxiov1beta1.AdoptNetworkingAnnotation,
			annotationVal: "INVALID",
			conditionCheck: func(t *testing.T, instance *ogxiov1beta1.OGXServer) {
				t.Helper()
				assertConditionTrue(t, instance, "AdoptionConfigInvalid")
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			namespace := createTestNamespace(t, "adopt-validate")

			builder := NewOGXServerBuilder().
				WithName("validation-test").
				WithNamespace(namespace.Name)

			builder = builder.WithAnnotation(tt.annotationKey, tt.annotationVal)

			instance := builder.Build()
			require.NoError(t, k8sClient.Create(t.Context(), instance))
			t.Cleanup(func() {
				if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
					t.Logf("Cleanup: %v", err)
				}
			})

			ReconcileOGXServer(t, instance)

			require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
				Name: instance.Name, Namespace: instance.Namespace,
			}, instance))

			tt.conditionCheck(t, instance)
		})
	}
}

func TestNoAdoptionWithoutAnnotations(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := createTestNamespace(t, "no-adopt")

	instance := NewOGXServerBuilder().
		WithName("clean-install").
		WithNamespace(namespace.Name).
		WithStorage(DefaultTestStorage()).
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup: %v", err)
		}
	})

	ReconcileOGXServer(t, instance)

	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name: instance.Name, Namespace: instance.Namespace,
	}, instance))

	// No adoption conditions should be set.
	assertConditionAbsent(t, instance, "StorageAdopted")
	assertConditionAbsent(t, instance, "NetworkingAdopted")
	assertConditionAbsent(t, instance, "AdoptionConfigInvalid")

	// Default PVC should be created with the instance name.
	pvc := &corev1.PersistentVolumeClaim{}
	key := types.NamespacedName{Name: "clean-install-pvc", Namespace: namespace.Name}
	require.Eventually(t, func() bool {
		return k8sClient.Get(t.Context(), key, pvc) == nil
	}, testTimeout, testInterval, "Default PVC should be created for clean install")
}

func TestInvalidAdoptionStorageFallsBackToDefaultPVC(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := createTestNamespace(t, "invalid-adopt-fallback")

	instance := NewOGXServerBuilder().
		WithName("fallback-server").
		WithNamespace(namespace.Name).
		WithStorage(DefaultTestStorage()).
		WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "INVALID_NAME").
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup: %v", err)
		}
	})

	ReconcileOGXServer(t, instance)

	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name: instance.Name, Namespace: instance.Namespace,
	}, instance))

	assertConditionTrue(t, instance, "AdoptionConfigInvalid")

	// Invalid annotation should not suppress default PVC behavior.
	defaultPVC := &corev1.PersistentVolumeClaim{}
	defaultKey := types.NamespacedName{Name: "fallback-server-pvc", Namespace: namespace.Name}
	require.Eventually(t, func() bool {
		return k8sClient.Get(t.Context(), defaultKey, defaultPVC) == nil
	}, testTimeout, testInterval, "Default PVC should be created even when adoption annotation is invalid")
}

func TestAdoptionConfigInvalidClearsWhenAnnotationRemoved(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := createTestNamespace(t, "adopt-invalid-clear")

	instance := NewOGXServerBuilder().
		WithName("clear-invalid").
		WithNamespace(namespace.Name).
		WithStorage(DefaultTestStorage()).
		WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "INVALID_NAME").
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup: %v", err)
		}
	})

	ReconcileOGXServer(t, instance)
	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name: instance.Name, Namespace: instance.Namespace,
	}, instance))
	assertConditionTrue(t, instance, "AdoptionConfigInvalid")

	// Remove annotation and reconcile again; invalid condition should clear.
	delete(instance.Annotations, ogxiov1beta1.AdoptStorageAnnotation)
	require.NoError(t, k8sClient.Update(t.Context(), instance))

	ReconcileOGXServer(t, instance)
	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name: instance.Name, Namespace: instance.Namespace,
	}, instance))
	assertConditionFalse(t, instance, "AdoptionConfigInvalid")
}

func TestCleanupAdoptedNetworkingOnAnnotationRemoval(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := createTestNamespace(t, "adopt-cleanup-net")
	createLegacyService(t, namespace.Name, "legacy-net")

	instance := NewOGXServerBuilder().
		WithName("new-net").
		WithNamespace(namespace.Name).
		WithAnnotation(ogxiov1beta1.AdoptNetworkingAnnotation, "legacy-net").
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup: %v", err)
		}
	})

	// First reconcile adopts the legacy service.
	ReconcileOGXServer(t, instance)
	legacyKey := types.NamespacedName{Name: "legacy-net-service", Namespace: namespace.Name}
	legacySvc := &corev1.Service{}
	require.Eventually(t, func() bool {
		return k8sClient.Get(t.Context(), legacyKey, legacySvc) == nil &&
			metav1.IsControlledBy(legacySvc, instance)
	}, testTimeout, testInterval, "Legacy Service should be adopted")

	// Remove annotation and reconcile again; adopted legacy service should be deleted.
	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name: instance.Name, Namespace: instance.Namespace,
	}, instance))
	delete(instance.Annotations, ogxiov1beta1.AdoptNetworkingAnnotation)
	require.NoError(t, k8sClient.Update(t.Context(), instance))

	ReconcileOGXServer(t, instance)
	require.Eventually(t, func() bool {
		return apierrors.IsNotFound(k8sClient.Get(t.Context(), legacyKey, &corev1.Service{}))
	}, testTimeout, testInterval, "Adopted legacy Service should be deleted when annotation is removed")

	// New service for current instance should exist.
	newKey := types.NamespacedName{Name: "new-net-service", Namespace: namespace.Name}
	require.Eventually(t, func() bool {
		return k8sClient.Get(t.Context(), newKey, &corev1.Service{}) == nil
	}, testTimeout, testInterval, "Current instance Service should exist after cleanup")
}

func TestSelfAdoptionRejectedByController(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := createTestNamespace(t, "self-adopt")

	instance := NewOGXServerBuilder().
		WithName("same-name").
		WithNamespace(namespace.Name).
		WithStorage(DefaultTestStorage()).
		WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "same-name").
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup: %v", err)
		}
	})

	ReconcileOGXServer(t, instance)

	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name: instance.Name, Namespace: instance.Namespace,
	}, instance))

	assertConditionTrue(t, instance, "AdoptionConfigInvalid")
}

func TestAdoptedPVCLifecycle(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := createTestNamespace(t, "adopt-pvc-lifecycle")
	createLegacyPVC(t, namespace.Name, "disc-legacy")

	// --- arrange ---
	pvcKey := types.NamespacedName{Name: "disc-legacy-pvc", Namespace: namespace.Name}
	defaultKey := types.NamespacedName{Name: "disc-server-pvc", Namespace: namespace.Name}
	deployKey := types.NamespacedName{Name: "disc-server", Namespace: namespace.Name}

	instance := NewOGXServerBuilder().
		WithName("disc-server").
		WithNamespace(namespace.Name).
		WithStorage(DefaultTestStorage()).
		WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "disc-legacy").
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))

	// --- act: adopt PVC, then remove annotation ---
	ReconcileOGXServer(t, instance)

	pvc := &corev1.PersistentVolumeClaim{}
	require.Eventually(t, func() bool {
		return k8sClient.Get(t.Context(), pvcKey, pvc) == nil &&
			pvc.Labels[ogxiov1beta1.AdoptedFromLabel] == "disc-legacy"
	}, testTimeout, testInterval, "PVC should be adopted with label")

	deployment := &appsv1.Deployment{}
	waitForResource(t, k8sClient, namespace.Name, "disc-server", deployment)

	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name: instance.Name, Namespace: instance.Namespace,
	}, instance))
	delete(instance.Annotations, ogxiov1beta1.AdoptStorageAnnotation)
	require.NoError(t, k8sClient.Update(t.Context(), instance))

	ReconcileOGXServer(t, instance)

	// --- assert: adopted PVC discovered by label, Deployment uses it ---
	require.NoError(t, k8sClient.Get(t.Context(), pvcKey, pvc),
		"Adopted PVC must survive annotation removal")
	require.True(t, apierrors.IsNotFound(k8sClient.Get(t.Context(), defaultKey, &corev1.PersistentVolumeClaim{})),
		"Default PVC should NOT be created when adopted PVC is discovered by label")

	require.NoError(t, k8sClient.Get(t.Context(), deployKey, deployment))
	AssertDeploymentUsesPVCStorage(t, deployment, "disc-legacy-pvc")

	// --- act: delete OGXServer ---
	require.NoError(t, k8sClient.Delete(t.Context(), instance))
	require.Eventually(t, func() bool {
		return apierrors.IsNotFound(k8sClient.Get(t.Context(), types.NamespacedName{
			Name: "disc-server", Namespace: namespace.Name,
		}, &ogxiov1beta1.OGXServer{}))
	}, testTimeout, testInterval, "OGXServer should be deleted")

	// --- assert: adopted PVC survives CR deletion with labels intact ---
	require.NoError(t, k8sClient.Get(t.Context(), pvcKey, pvc),
		"Adopted PVC must survive OGXServer deletion")
	require.Equal(t, "disc-legacy", pvc.Labels[ogxiov1beta1.AdoptedFromLabel],
		"Adopted-from label must be unchanged after CR deletion")
	require.Equal(t, "disc-server", pvc.Labels["app.kubernetes.io/instance"],
		"Instance label must be unchanged after CR deletion")

	// --- act: recreate OGXServer with same name ---
	instance = NewOGXServerBuilder().
		WithName("disc-server").
		WithNamespace(namespace.Name).
		WithStorage(DefaultTestStorage()).
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup: %v", err)
		}
	})

	ReconcileOGXServer(t, instance)

	// --- assert: adopted PVC discovered, no default PVC created ---
	require.NoError(t, k8sClient.Get(t.Context(), pvcKey, pvc),
		"Adopted PVC must still exist after recreate")
	require.True(t, apierrors.IsNotFound(k8sClient.Get(t.Context(), defaultKey, &corev1.PersistentVolumeClaim{})),
		"Default PVC should NOT be created when adopted PVC is discovered by label")

	require.NoError(t, k8sClient.Get(t.Context(), deployKey, deployment))
	AssertDeploymentUsesPVCStorage(t, deployment, "disc-legacy-pvc")
}

func TestAdoptionRejectsUnexpectedControllerOwner(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := createTestNamespace(t, "adopt-owner-guard")
	createLegacyPVCWithControllerOwner(t, namespace.Name, "legacy-safe", metav1.OwnerReference{
		APIVersion: "apps/v1",
		Kind:       "Deployment",
		Name:       "not-legacy-safe",
		UID:        types.UID("foreign-owner"),
		Controller: boolPtrAdoptionTest(true),
	})

	instance := NewOGXServerBuilder().
		WithName("owner-check").
		WithNamespace(namespace.Name).
		WithStorage(DefaultTestStorage()).
		WithAnnotation(ogxiov1beta1.AdoptStorageAnnotation, "legacy-safe").
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup: %v", err)
		}
	})

	reconciler := createTestReconciler()
	_, err := reconciler.Reconcile(t.Context(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	})
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to adopt")
}

func TestMultipleAdoptedPVCsReturnsTerminalError(t *testing.T) {
	ctrl.SetLogger(zap.New(zap.UseDevMode(true)))
	namespace := createTestNamespace(t, "adopt-multi-pvc")

	// --- arrange: create two PVCs with matching adoption labels ---
	for _, pvcName := range []string{"first-pvc", "second-pvc"} {
		storageSize := resource.MustParse("1Gi")
		pvc := &corev1.PersistentVolumeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name:      pvcName,
				Namespace: namespace.Name,
				Labels: map[string]string{
					ogxiov1beta1.AdoptedFromLabel: "some-legacy",
					"app.kubernetes.io/instance":  "multi-test",
				},
			},
			Spec: corev1.PersistentVolumeClaimSpec{
				AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
				Resources: corev1.VolumeResourceRequirements{
					Requests: corev1.ResourceList{
						corev1.ResourceStorage: storageSize,
					},
				},
			},
		}
		require.NoError(t, k8sClient.Create(t.Context(), pvc))
		t.Cleanup(func() {
			if err := k8sClient.Delete(t.Context(), pvc); err != nil && !apierrors.IsNotFound(err) {
				t.Logf("Cleanup PVC: %v", err)
			}
		})
	}

	instance := NewOGXServerBuilder().
		WithName("multi-test").
		WithNamespace(namespace.Name).
		WithStorage(DefaultTestStorage()).
		Build()

	require.NoError(t, k8sClient.Create(t.Context(), instance))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), instance); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup: %v", err)
		}
	})

	// --- act ---
	reconciler := createTestReconciler()
	result, err := reconciler.Reconcile(t.Context(), ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      instance.Name,
			Namespace: instance.Namespace,
		},
	})

	// --- assert: no error returned (terminal), no requeue ---
	require.NoError(t, err, "terminal error should not propagate as a reconcile error")
	require.Zero(t, result.RequeueAfter, "should not requeue on terminal error")

	require.NoError(t, k8sClient.Get(t.Context(), types.NamespacedName{
		Name: instance.Name, Namespace: instance.Namespace,
	}, instance))
	assertConditionTrue(t, instance, "AdoptionConfigInvalid")
}

// --- Helpers ---

func (b *OGXServerBuilder) WithAnnotation(key, value string) *OGXServerBuilder {
	if b.instance.Annotations == nil {
		b.instance.Annotations = make(map[string]string)
	}
	b.instance.Annotations[key] = value
	return b
}

func createLegacyPVC(t *testing.T, ns, legacyName string) {
	t.Helper()
	storageSize := resource.MustParse("1Gi")
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyName + "-pvc",
			Namespace: ns,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), pvc))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), pvc); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup PVC: %v", err)
		}
	})
}

func createLegacyPVCWithControllerOwner(t *testing.T, ns, legacyName string, ownerRef metav1.OwnerReference) {
	t.Helper()
	storageSize := resource.MustParse("1Gi")
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:            legacyName + "-pvc",
			Namespace:       ns,
			OwnerReferences: []metav1.OwnerReference{ownerRef},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: storageSize,
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), pvc))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), pvc); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup PVC: %v", err)
		}
	})
}

func createLegacyDeployment(t *testing.T, ns, legacyName string) {
	t.Helper()
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyName,
			Namespace: ns,
			Labels:    map[string]string{"app.kubernetes.io/instance": legacyName},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app.kubernetes.io/instance": legacyName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app.kubernetes.io/instance": legacyName},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{{
						Name:  "llama-stack",
						Image: "registry.example.com/llama-stack:latest",
					}},
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), deployment))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), deployment); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup Deployment: %v", err)
		}
	})
}

func createLegacyService(t *testing.T, ns, legacyName string) {
	t.Helper()
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyName + "-service",
			Namespace: ns,
			Labels:    map[string]string{"app": "llama-stack"},
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app":                        "llama-stack",
				"app.kubernetes.io/instance": legacyName,
			},
			Ports: []corev1.ServicePort{{
				Name:     "http",
				Port:     8321,
				Protocol: corev1.ProtocolTCP,
			}},
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), svc))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), svc); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup Service: %v", err)
		}
	})
}

func createLegacyIngress(t *testing.T, ns, legacyName string) {
	t.Helper()
	pathType := networkingv1.PathTypePrefix
	ingress := &networkingv1.Ingress{
		ObjectMeta: metav1.ObjectMeta{
			Name:      legacyName + "-ingress",
			Namespace: ns,
		},
		Spec: networkingv1.IngressSpec{
			Rules: []networkingv1.IngressRule{{
				Host: "legacy.example.com",
				IngressRuleValue: networkingv1.IngressRuleValue{
					HTTP: &networkingv1.HTTPIngressRuleValue{
						Paths: []networkingv1.HTTPIngressPath{{
							Path:     "/",
							PathType: &pathType,
							Backend: networkingv1.IngressBackend{
								Service: &networkingv1.IngressServiceBackend{
									Name: legacyName + "-service",
									Port: networkingv1.ServiceBackendPort{
										Number: 8321,
									},
								},
							},
						}},
					},
				},
			}},
		},
	}
	require.NoError(t, k8sClient.Create(t.Context(), ingress))
	t.Cleanup(func() {
		if err := k8sClient.Delete(t.Context(), ingress); err != nil && !apierrors.IsNotFound(err) {
			t.Logf("Cleanup Ingress: %v", err)
		}
	})
}

func assertConditionTrue(t *testing.T, instance *ogxiov1beta1.OGXServer, condType string) {
	t.Helper()
	for _, c := range instance.Status.Conditions {
		if c.Type == condType {
			require.Equal(t, metav1.ConditionTrue, c.Status,
				"condition %s should be True", condType)
			return
		}
	}
	t.Errorf("condition %s not found", condType)
}

func assertConditionAbsent(t *testing.T, instance *ogxiov1beta1.OGXServer, condType string) {
	t.Helper()
	for _, c := range instance.Status.Conditions {
		if c.Type == condType {
			t.Errorf("condition %s should not be present", condType)
			return
		}
	}
}

func assertConditionFalse(t *testing.T, instance *ogxiov1beta1.OGXServer, condType string) {
	t.Helper()
	for _, c := range instance.Status.Conditions {
		if c.Type == condType {
			require.Equal(t, metav1.ConditionFalse, c.Status,
				"condition %s should be False", condType)
			return
		}
	}
	t.Errorf("condition %s not found", condType)
}

func boolPtrAdoptionTest(v bool) *bool {
	return &v
}
