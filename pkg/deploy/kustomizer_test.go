package deploy

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	llamav1alpha1 "github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/deploy/plugins"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	"k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/kustomize/api/resmap"
	kresource "sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/filesys"
)

const manifestBasePath = "manifests/base"

func setupApplyResourcesTest(t *testing.T, ownerName string) (context.Context, string, *llamav1alpha1.LlamaStackDistribution) {
	t.Helper()

	ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second) // to avlid client rate limit due to too many test in parallel
	testNs := "test-apply-" + ownerName
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: testNs},
	}
	require.NoError(t, k8sClient.Create(ctx, ns))
	t.Cleanup(func() {
		cancel()
		require.NoError(t, k8sClient.Delete(context.Background(), ns))
	})

	owner := &llamav1alpha1.LlamaStackDistribution{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "llamastack.io/v1alpha1",
			Kind:       "LlamaStackDistribution",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ownerName,
			Namespace: testNs,
		},
	}
	ownerGVK := owner.GroupVersionKind()

	require.NoError(t, k8sClient.Create(ctx, owner))
	require.NotEmpty(t, owner.UID)

	createdOwner := &llamav1alpha1.LlamaStackDistribution{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: owner.Name, Namespace: owner.Namespace}, createdOwner))
	createdOwner.SetGroupVersionKind(ownerGVK)

	return ctx, testNs, createdOwner
}

// TestRenderManifest contains all unit tests for the RenderManifest function.
func TestRenderManifest(t *testing.T) {
	t.Run("should render correctly with a standard layout", func(t *testing.T) {
		// given an-memory filesystem with a standard kustomize layout
		fsys := filesys.MakeFsInMemory()
		require.NoError(t, fsys.MkdirAll(manifestBasePath))

		kustomizationContent := `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - pvc.yaml
`
		require.NoError(t, fsys.WriteFile(filepath.Join(manifestBasePath, "kustomization.yaml"), []byte(kustomizationContent)))

		pvcContent := `
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: pvc
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 5Gi
`
		require.NoError(t, fsys.WriteFile(filepath.Join(manifestBasePath, "pvc.yaml"), []byte(pvcContent)))

		// given an owner with an empty spec to verify that the default value logic
		// in the field transformer plugin is correctly triggered
		owner := &llamav1alpha1.LlamaStackDistribution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-instance",
				Namespace: "test-render-ns",
			},
			Spec: llamav1alpha1.LlamaStackDistributionSpec{},
		}

		// when we call RenderManifest
		resMap, err := RenderManifest(fsys, manifestBasePath, owner)

		// then we expect the resource to be rendered and transformed correctly
		require.NoError(t, err)
		require.Equal(t, 1, (*resMap).Size(), "ResMap should contain one resource")

		res := (*resMap).Resources()[0]
		require.Equal(t, "test-instance-pvc", res.GetName())
		assert.Equal(t, "test-render-ns", res.GetNamespace(), "PVC should have the correct namespace set by plugin")

		finalMap, err := res.Map()
		require.NoError(t, err)
		storage, found, err := unstructured.NestedString(finalMap, "spec", "resources", "requests", "storage")
		require.NoError(t, err)
		require.True(t, found, "storage field should exist")
		require.Equal(t, "10Gi", storage, "storage size should be updated to the default")
	})

	t.Run("should fall back to the default directory if kustomization.yaml is missing", func(t *testing.T) {
		// given a filesystem where the manifests are in a 'default' subdirectory
		fsys := filesys.MakeFsInMemory()
		require.NoError(t, fsys.Mkdir(manifestBasePath))

		defaultPath := filepath.Join(manifestBasePath, "default")
		require.NoError(t, fsys.Mkdir(defaultPath))

		kustomizationContent := `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - deployment.yaml
`
		require.NoError(t, fsys.WriteFile(filepath.Join(defaultPath, "kustomization.yaml"), []byte(kustomizationContent)))

		deploymentContent := `
apiVersion: apps/v1
kind: Deployment
metadata:
  name: deployment`
		require.NoError(t, fsys.WriteFile(filepath.Join(defaultPath, "deployment.yaml"), []byte(deploymentContent)))

		owner := &llamav1alpha1.LlamaStackDistribution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-instance",
				Namespace: "test-fallback-ns",
			},
		}

		// when we call RenderManifest on the root path
		resMap, err := RenderManifest(fsys, manifestBasePath, owner)

		// then it should find and render the resources from the 'default' subdirectory
		require.NoError(t, err)
		require.Equal(t, 1, (*resMap).Size())
		res := (*resMap).Resources()[0]
		require.Equal(t, "Deployment", res.GetKind())
		require.Equal(t, "test-instance", res.GetName())
		assert.Equal(t, "test-fallback-ns", res.GetNamespace(), "Deployment should have the correct namespace set by plugin")
	})

	t.Run("should return an error if a resource file is missing", func(t *testing.T) {
		// given a kustomization.yaml that references a file that does not exist
		fsys := filesys.MakeFsInMemory()
		require.NoError(t, fsys.MkdirAll(manifestBasePath))

		kustomizationContent := `
apiVersion: kustomize.config.k8s.io/v1beta1
kind: Kustomization
resources:
  - non-existent-pvc.yaml
`
		require.NoError(t, fsys.WriteFile(filepath.Join(manifestBasePath, "kustomization.yaml"), []byte(kustomizationContent)))

		owner := &llamav1alpha1.LlamaStackDistribution{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-instance",
				Namespace: "test-error-ns",
			},
		}

		// when we call RenderManifest
		resMap, err := RenderManifest(fsys, manifestBasePath, owner)

		// then it should propagate the error from the Kustomize engine
		require.Error(t, err)
		require.Nil(t, resMap)
		require.Contains(t, err.Error(), "non-existent-pvc.yaml")
	})
}

// TestApplyResources contains tests for applying resources to the cluster.
func TestApplyResources(t *testing.T) {
	t.Run("happy path", func(t *testing.T) {
		// given
		ctx, testNs, owner := setupApplyResourcesTest(t, "happy-path-owner")

		existingSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-service",
				Namespace: testNs,
				Labels:    map[string]string{"state": "initial"},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(owner, owner.GroupVersionKind()),
				},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "web", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(80)}}},
		}
		require.NoError(t, k8sClient.Create(ctx, existingSvc))

		// Create resources with the newTestResource helper, providing namespace
		desiredDeployment := newTestResource(t, "apps/v1", "Deployment", "my-deployment", testNs, map[string]any{"replicas": 1})
		desiredSvcSpec := map[string]any{
			"ports": []any{
				map[string]any{"name": "web", "protocol": "TCP", "port": 80, "targetPort": 8080},
			},
		}
		desiredSvc := newTestResource(t, "v1", "Service", "my-service", testNs, desiredSvcSpec)
		desiredSvc.SetLabels(map[string]string{"state": "updated"}) // labels are at ObjectMeta level

		resMap := resmap.New()
		require.NoError(t, resMap.Append(desiredDeployment))
		require.NoError(t, resMap.Append(desiredSvc))

		// when
		require.NoError(t, ApplyResources(ctx, k8sClient, scheme.Scheme, owner, &resMap)) // Pass address of resMap

		// then
		// verify deployment created correctly
		createdDeployment := &appsv1.Deployment{}
		deploymentKey := types.NamespacedName{Name: "my-deployment", Namespace: testNs}
		require.NoError(t, k8sClient.Get(ctx, deploymentKey, createdDeployment))
		require.Len(t, createdDeployment.GetOwnerReferences(), 1, "created deployment should have an owner reference")
		require.Equal(t, owner.UID, createdDeployment.GetOwnerReferences()[0].UID, "owner reference UID should match")

		// verify service patched correctly
		updatedService := &corev1.Service{}
		serviceKey := types.NamespacedName{Name: "my-service", Namespace: testNs}
		require.NoError(t, k8sClient.Get(ctx, serviceKey, updatedService))
		require.Equal(t, intstr.FromInt(8080), updatedService.Spec.Ports[0].TargetPort, "service target port should be updated")
		require.Equal(t, "updated", updatedService.Labels["state"], "service label should be updated")
	})

	t.Run("skips owner", func(t *testing.T) {
		// given
		ctx, testNs, owner := setupApplyResourcesTest(t, "skip-owner")

		existingSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-service",
				Namespace: testNs,
				Labels:    map[string]string{"state": "initial"},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(owner, owner.GroupVersionKind()),
				},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "web", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(80)}}},
		}
		require.NoError(t, k8sClient.Create(ctx, existingSvc))

		// Create resources with the newTestResource helper, providing namespace
		desiredDeployment := newTestResource(t, "apps/v1", "Deployment", "my-deployment", testNs, map[string]any{"replicas": 1})
		desiredSvcSpec := map[string]any{
			"ports": []any{
				map[string]any{"name": "web", "protocol": "TCP", "port": 80, "targetPort": 8080},
			},
		}
		desiredSvc := newTestResource(t, "v1", "Service", "my-service", testNs, desiredSvcSpec)
		desiredSvc.SetLabels(map[string]string{"state": "updated"})

		ownerGVK := owner.GroupVersionKind()
		ownerResrc := newTestResource(t,
			ownerGVK.GroupVersion().String(),
			ownerGVK.Kind,
			owner.Name,
			owner.Namespace,
			nil,
		)

		resMap := resmap.New()
		require.NoError(t, resMap.Append(desiredDeployment))
		require.NoError(t, resMap.Append(desiredSvc))
		require.NoError(t, resMap.Append(ownerResrc))

		// when
		require.NoError(t, ApplyResources(ctx, k8sClient, scheme.Scheme, owner, &resMap))

		// then
		// verify deployment created correctly
		createdDeployment := &appsv1.Deployment{}
		deploymentKey := types.NamespacedName{Name: "my-deployment", Namespace: testNs}
		require.NoError(t, k8sClient.Get(ctx, deploymentKey, createdDeployment))
		require.Len(t, createdDeployment.GetOwnerReferences(), 1, "created deployment should have an owner reference")
		require.Equal(t, owner.UID, createdDeployment.GetOwnerReferences()[0].UID, "owner reference UID should match")

		// verify service patched correctly
		updatedService := &corev1.Service{}
		serviceKey := types.NamespacedName{Name: "my-service", Namespace: testNs}
		require.NoError(t, k8sClient.Get(ctx, serviceKey, updatedService))
		require.Equal(t, intstr.FromInt(8080), updatedService.Spec.Ports[0].TargetPort, "service target port should be updated")
		require.Equal(t, "updated", updatedService.Labels["state"], "service label should be updated")
	})

	t.Run("but does not steal", func(t *testing.T) {
		// given
		ctx, testNs, owner := setupApplyResourcesTest(t, "does-not-steal-owner")

		ownerOther := &llamav1alpha1.LlamaStackDistribution{
			TypeMeta: metav1.TypeMeta{
				APIVersion: "llamastack.io/v1alpha1",
				Kind:       "LlamaStackDistribution",
			},
			ObjectMeta: metav1.ObjectMeta{
				Name:      "test-owner-other",
				Namespace: testNs,
			},
		}
		require.NoError(t, k8sClient.Create(ctx, ownerOther))

		createdOwnerOther := &llamav1alpha1.LlamaStackDistribution{}
		require.NoError(t, k8sClient.Get(ctx, client.ObjectKeyFromObject(ownerOther), createdOwnerOther))
		createdOwnerOther.SetGroupVersionKind(llamav1alpha1.GroupVersion.WithKind("LlamaStackDistribution"))

		existingSvc := &corev1.Service{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "my-service",
				Namespace: testNs,
				Labels:    map[string]string{"state": "initial"},
				OwnerReferences: []metav1.OwnerReference{
					*metav1.NewControllerRef(createdOwnerOther, createdOwnerOther.GroupVersionKind()),
				},
			},
			Spec: corev1.ServiceSpec{Ports: []corev1.ServicePort{{Name: "web", Protocol: corev1.ProtocolTCP, Port: 80, TargetPort: intstr.FromInt(80)}}},
		}
		require.NoError(t, k8sClient.Create(ctx, existingSvc))

		desiredSvcSpec := map[string]any{
			"ports": []any{
				map[string]any{"name": "web", "protocol": "TCP", "port": 80, "targetPort": 8080},
			},
		}
		desiredSvc := newTestResource(t, "v1", "Service", "my-service", testNs, desiredSvcSpec)
		desiredSvc.SetLabels(map[string]string{"state": "updated"})

		ownerGVK := owner.GroupVersionKind()
		ownerResrc := newTestResource(t,
			ownerGVK.GroupVersion().String(),
			ownerGVK.Kind,
			owner.Name,
			owner.Namespace,
			nil,
		)

		ownerOtherGVK := createdOwnerOther.GroupVersionKind()
		ownerOtherResrc := newTestResource(t,
			ownerOtherGVK.GroupVersion().String(),
			ownerOtherGVK.Kind,
			createdOwnerOther.Name,
			createdOwnerOther.Namespace,
			nil,
		)

		resMap := resmap.New()
		// desiredDeployment is not defined in this scope. Assuming it's meant to be.
		desiredDeployment := newTestResource(t, "apps/v1", "Deployment", "dummy-deployment", testNs, map[string]any{"replicas": 1})
		require.NoError(t, resMap.Append(desiredDeployment))
		require.NoError(t, resMap.Append(desiredSvc))
		require.NoError(t, resMap.Append(ownerResrc))
		require.NoError(t, resMap.Append(ownerOtherResrc))

		// when
		err := ApplyResources(ctx, k8sClient, scheme.Scheme, owner, &resMap)
		require.NoError(t, err, "should not error when encountering resources owned by other instances")

		// then verify the existing service was not modified (still owned by the other instance)
		unchangedService := &corev1.Service{}
		serviceKey := types.NamespacedName{Name: "my-service", Namespace: testNs}
		require.NoError(t, k8sClient.Get(ctx, serviceKey, unchangedService))
		require.Equal(t, intstr.FromInt(80), unchangedService.Spec.Ports[0].TargetPort, "service target port should remain unchanged")
		require.Equal(t, "initial", unchangedService.Labels["state"], "service label should remain unchanged")

		// verify it's still owned by the other instance
		require.Len(t, unchangedService.GetOwnerReferences(), 1, "service should still have exactly one owner reference")
		require.Equal(t, createdOwnerOther.UID, unchangedService.GetOwnerReferences()[0].UID, "service should still be owned by the other instance")
	})

	t.Run("creates cluster-scoped objects without owner reference", func(t *testing.T) {
		// given a namespaced owner (its namespace is irrelevant for this test)
		ctx, _, owner := setupApplyResourcesTest(t, "cluster-scope-owner")

		// and a desired cluster-scoped resource (ClusterRole)
		desiredClusterRole := newTestResource(t, "rbac.authorization.k8s.io/v1", "ClusterRole", "my-test-cluster-role", "" /* No namespace */, map[string]any{
			"rules": []any{
				map[string]any{
					"apiGroups": []any{""},
					"resources": []any{"nodes"},
					"verbs":     []any{"get", "list"},
				},
			},
		})

		resMap := resmap.New()
		require.NoError(t, resMap.Append(desiredClusterRole))

		// when we apply the resources
		require.NoError(t, ApplyResources(ctx, k8sClient, scheme.Scheme, owner, &resMap))

		// then verify the cluster role was created correctly
		createdClusterRole := &rbacv1.ClusterRole{}
		// for cluster-scoped resources, the key only has a name
		clusterRoleKey := types.NamespacedName{Name: "my-test-cluster-role"}
		require.NoError(t, k8sClient.Get(ctx, clusterRoleKey, createdClusterRole))

		// verify it has NO owner reference
		require.Empty(t, createdClusterRole.GetOwnerReferences(), "cluster-scoped resource should not have an owner reference from a namespaced owner")

		// cleanup the clusterrole
		require.NoError(t, k8sClient.Delete(t.Context(), createdClusterRole))
	})
}

// TestApplyResources_PVCImmutability verifies that PVCs are not patched to maintain immutability.
func TestApplyResources_PVCImmutability(t *testing.T) {
	// given
	ctx, testNs, owner := setupApplyResourcesTest(t, "pvc-immutable")

	// create an existing PVC owned by our operator instance
	existingPVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "my-pvc",
			Namespace: testNs,
			Labels:    map[string]string{"state": "original"},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(owner, owner.GroupVersionKind()),
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("10Gi"),
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, existingPVC))

	// create a desired PVC with modified labels (this would normally trigger a patch)
	expStorageSize := "10Gi"
	desiredPVCSpec := map[string]any{
		"accessModes": []any{"ReadWriteOnce"},
		"resources": map[string]any{
			"requests": map[string]any{
				"storage": expStorageSize,
			},
		},
	}
	desiredPVC := newTestResource(t, "v1", "PersistentVolumeClaim", "my-pvc", testNs, desiredPVCSpec)
	desiredPVC.SetLabels(map[string]string{"state": "modified"}) // Different labels

	resMap := resmap.New()
	require.NoError(t, resMap.Append(desiredPVC))

	// when
	require.NoError(t, ApplyResources(ctx, k8sClient, scheme.Scheme, owner, &resMap))

	// then
	// the PVC was NOT modified
	unchangedPVC := &corev1.PersistentVolumeClaim{}
	pvcKey := types.NamespacedName{Name: "my-pvc", Namespace: testNs}
	require.NoError(t, k8sClient.Get(ctx, pvcKey, unchangedPVC))
	// labels were NOT updated
	require.Equal(t, "original", unchangedPVC.Labels["state"], "PVC labels should remain unchanged")
	// it's still owned by our instance
	require.Len(t, unchangedPVC.GetOwnerReferences(), 1, "PVC should still have exactly one owner reference")
	require.Equal(t, owner.UID, unchangedPVC.GetOwnerReferences()[0].UID, "PVC should still be owned by our instance")
	// spec remains the same
	storageRequest := unchangedPVC.Spec.Resources.Requests[corev1.ResourceStorage]
	require.Equal(t, expStorageSize, storageRequest.String(), "PVC storage spec should remain unchanged")
}

// TestFilterExcludeKinds tests the filtering functionality.
func TestFilterExcludeKinds(t *testing.T) {
	t.Run("excludes specified kinds", func(t *testing.T) {
		pvc := newTestResource(t, "v1", "PersistentVolumeClaim", "test-pvc", "test-ns", nil)
		svc := newTestResource(t, "v1", "Service", "test-svc", "test-ns", nil)

		resMap := resmap.New()
		require.NoError(t, resMap.Append(pvc))
		require.NoError(t, resMap.Append(svc))

		filtered, err := FilterExcludeKinds(&resMap, []string{"PersistentVolumeClaim"})
		require.NoError(t, err)
		require.Equal(t, 1, (*filtered).Size())
		require.Equal(t, "Service", (*filtered).Resources()[0].GetKind())
	})

	t.Run("includes all when no exclusions", func(t *testing.T) {
		svc := newTestResource(t, "v1", "Service", "test-svc", "test-ns", nil)
		resMap := resmap.New()
		require.NoError(t, resMap.Append(svc))

		filtered, err := FilterExcludeKinds(&resMap, []string{})
		require.NoError(t, err)
		require.Equal(t, 1, (*filtered).Size())
		require.Equal(t, "Service", (*filtered).Resources()[0].GetKind())
	})

	t.Run("handles empty inputs", func(t *testing.T) {
		emptyResMap := resmap.New()

		filtered, err := FilterExcludeKinds(&emptyResMap, []string{"PersistentVolumeClaim"})
		require.NoError(t, err)
		require.Equal(t, 0, (*filtered).Size())
	})

	t.Run("excludes multiple kinds", func(t *testing.T) {
		pvc := newTestResource(t, "v1", "PersistentVolumeClaim", "test-pvc", "test-ns", nil)
		svc := newTestResource(t, "v1", "Service", "test-svc", "test-ns", nil)
		deployment := newTestResource(t, "apps/v1", "Deployment", "test-deployment", "test-ns", nil)

		resMap := resmap.New()
		require.NoError(t, resMap.Append(pvc))
		require.NoError(t, resMap.Append(svc))
		require.NoError(t, resMap.Append(deployment))

		filtered, err := FilterExcludeKinds(&resMap, []string{"PersistentVolumeClaim", "Service"})
		require.NoError(t, err)
		require.Equal(t, 1, (*filtered).Size())
		require.Equal(t, "Deployment", (*filtered).Resources()[0].GetKind())
	})
}

func TestSetDefaultPort(t *testing.T) {
	// arrange
	// instance with no custom port and service with empty port values
	instance := &llamav1alpha1.LlamaStackDistribution{
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Server: llamav1alpha1.ServerSpec{
				ContainerSpec: llamav1alpha1.ContainerSpec{
					Port: 0, // no port configured
				},
			},
		},
	}

	service := newTestResource(t, "v1", "Service", "test-service", "test-ns", map[string]any{
		"ports": []any{
			map[string]any{"port": nil}, // empty port to trigger default
		},
	})

	fieldMutator := plugins.CreateFieldMutator(plugins.FieldMutatorConfig{
		Mappings: []plugins.FieldMapping{
			{
				SourceValue:       getServicePort(instance), // tests getServicePort() integration with kustomizer
				DefaultValue:      llamav1alpha1.DefaultServerPort,
				TargetField:       "/spec/ports/0/port",
				TargetKind:        "Service",
				CreateIfNotExists: true,
			},
		},
	})

	resMap := resmap.New()
	require.NoError(t, resMap.Append(service))

	// act
	// apply field transformation
	require.NoError(t, fieldMutator.Transform(resMap))

	// assert
	// port should be set to default value
	transformedService := resMap.Resources()[0]
	serviceMap, err := transformedService.Map()
	require.NoError(t, err)
	ports, ok := serviceMap["spec"].(map[string]any)["ports"].([]any)
	require.True(t, ok)
	actualPort, ok := ports[0].(map[string]any)["port"]
	require.True(t, ok)
	require.Equal(t, int(llamav1alpha1.DefaultServerPort), actualPort)
}

func TestRemoveDeploymentReplicas(t *testing.T) {
	t.Parallel()

	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "example",
			Namespace: "llama",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Server: llamav1alpha1.ServerSpec{
				Autoscaling: &llamav1alpha1.AutoscalingSpec{
					MaxReplicas: 5,
				},
			},
		},
	}

	resMap := resmap.New()
	require.NoError(t, resMap.Append(newTestResource(t, "apps/v1", "Deployment", "deployment", "llama", map[string]any{
		"replicas": int32(1),
	})))
	require.NoError(t, resMap.Append(newTestResource(t, "apps/v1", "Deployment", "deployment-no-replicas", "llama", map[string]any{})))

	require.NoError(t, applyPlugins(&resMap, instance))

	resources := resMap.Resources()
	require.Len(t, resources, 2)

	var hasReplicas bool
	for _, res := range resources {
		if res.GetKind() != "Deployment" {
			continue
		}
		spec, err := res.Map()
		require.NoError(t, err)
		specMap, ok := spec["spec"].(map[string]any)
		require.True(t, ok, "deployment should have a spec map")
		if _, ok := specMap["replicas"]; ok {
			hasReplicas = true
		}
	}

	require.False(t, hasReplicas, "replicas should be removed from all deployments when autoscaling is enabled")
}

// TestHasLegacyCABundleVolumes tests the detection of legacy CA bundle volumes.
func TestHasLegacyCABundleVolumes(t *testing.T) {
	ctx := t.Context()

	t.Run("detects legacy emptyDir ca-bundle volume", func(t *testing.T) {
		deployment := newTestResource(t, "apps/v1", "Deployment", "test-deploy", "test-ns", map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"volumes": []any{
						map[string]any{
							"name":     "ca-bundle",
							"emptyDir": map[string]any{},
						},
					},
				},
			},
		})

		u, err := resourceToUnstructured(t, deployment)
		require.NoError(t, err)

		result := hasLegacyCABundleVolumes(ctx, u)
		require.True(t, result, "should detect legacy ca-bundle emptyDir volume")
	})

	t.Run("detects legacy ca-bundle-source volume", func(t *testing.T) {
		deployment := newTestResource(t, "apps/v1", "Deployment", "test-deploy", "test-ns", map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"volumes": []any{
						map[string]any{
							"name": "ca-bundle-source",
							"configMap": map[string]any{
								"name": "odh-trusted-ca-bundle",
							},
						},
					},
				},
			},
		})

		u, err := resourceToUnstructured(t, deployment)
		require.NoError(t, err)

		result := hasLegacyCABundleVolumes(ctx, u)
		require.True(t, result, "should detect legacy ca-bundle-source volume")
	})

	t.Run("does not detect new-style ca-bundle configMap", func(t *testing.T) {
		deployment := newTestResource(t, "apps/v1", "Deployment", "test-deploy", "test-ns", map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"volumes": []any{
						map[string]any{
							"name": "ca-bundle",
							"configMap": map[string]any{
								"name": "managed-ca-bundle",
							},
						},
					},
				},
			},
		})

		u, err := resourceToUnstructured(t, deployment)
		require.NoError(t, err)

		result := hasLegacyCABundleVolumes(ctx, u)
		require.False(t, result, "should not detect new-style ca-bundle ConfigMap as legacy")
	})

	t.Run("does not detect unrelated volumes", func(t *testing.T) {
		deployment := newTestResource(t, "apps/v1", "Deployment", "test-deploy", "test-ns", map[string]any{
			"template": map[string]any{
				"spec": map[string]any{
					"volumes": []any{
						map[string]any{
							"name":     "data",
							"emptyDir": map[string]any{},
						},
						map[string]any{
							"name": "config",
							"configMap": map[string]any{
								"name": "app-config",
							},
						},
					},
				},
			},
		})

		u, err := resourceToUnstructured(t, deployment)
		require.NoError(t, err)

		result := hasLegacyCABundleVolumes(ctx, u)
		require.False(t, result, "should not detect unrelated volumes as legacy")
	})

	t.Run("returns false when no volumes present", func(t *testing.T) {
		deployment := newTestResource(t, "apps/v1", "Deployment", "test-deploy", "test-ns", map[string]any{})

		u, err := resourceToUnstructured(t, deployment)
		require.NoError(t, err)

		result := hasLegacyCABundleVolumes(ctx, u)
		require.False(t, result, "should return false when no volumes present")
	})
}

// TestHasStaleUserConfigVolume tests detection of a stale user-config volume
// (present in existing Deployment but absent from desired).
func TestHasStaleUserConfigVolume(t *testing.T) {
	makeDeployment := func(volumes ...corev1.Volume) *appsv1.Deployment {
		return &appsv1.Deployment{
			Spec: appsv1.DeploymentSpec{
				Template: corev1.PodTemplateSpec{
					Spec: corev1.PodSpec{Volumes: volumes},
				},
			},
		}
	}

	userConfigVol := corev1.Volume{
		Name: "user-config",
		VolumeSource: corev1.VolumeSource{
			ConfigMap: &corev1.ConfigMapVolumeSource{
				LocalObjectReference: corev1.LocalObjectReference{Name: "my-config"},
			},
		},
	}
	storageVol := corev1.Volume{
		Name:         "lls-storage",
		VolumeSource: corev1.VolumeSource{EmptyDir: &corev1.EmptyDirVolumeSource{}},
	}

	t.Run("returns true when existing has user-config and desired does not", func(t *testing.T) {
		existing := makeDeployment(storageVol, userConfigVol)
		desired := makeDeployment(storageVol)
		require.True(t, hasStaleUserConfigVolume(desired, existing))
	})

	t.Run("returns false when both existing and desired have user-config", func(t *testing.T) {
		existing := makeDeployment(storageVol, userConfigVol)
		desired := makeDeployment(storageVol, userConfigVol)
		require.False(t, hasStaleUserConfigVolume(desired, existing))
	})

	t.Run("returns false when neither has user-config", func(t *testing.T) {
		existing := makeDeployment(storageVol)
		desired := makeDeployment(storageVol)
		require.False(t, hasStaleUserConfigVolume(desired, existing))
	})

	t.Run("returns false when only desired has user-config", func(t *testing.T) {
		existing := makeDeployment(storageVol)
		desired := makeDeployment(storageVol, userConfigVol)
		require.False(t, hasStaleUserConfigVolume(desired, existing))
	})

	t.Run("returns false when no volumes present in either", func(t *testing.T) {
		existing := makeDeployment()
		desired := makeDeployment()
		require.False(t, hasStaleUserConfigVolume(desired, existing))
	})
}

// TestUserConfigVolumeRemoval tests that removing spec.server.userConfig from the LLSD
// causes the "user-config" volume to be removed from the Deployment.
func TestUserConfigVolumeRemoval(t *testing.T) {
	ctx, testNs, owner := setupApplyResourcesTest(t, "userconfig-removal")

	// Create an existing Deployment that has a user-config volume (simulates operator
	// creating the Deployment via cli.Create when userConfig was set, without SSA
	// field manager tracking).
	existingDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: testNs,
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(owner, owner.GroupVersionKind()),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{Name: "main", Image: "test:v1"},
					},
					Volumes: []corev1.Volume{
						{
							Name: "lls-storage",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						// This volume was set when userConfig was configured;
						// it should be removed when userConfig is cleared.
						{
							Name: "user-config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "my-llama-config",
									},
								},
							},
						},
					},
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, existingDeployment))

	// Desired Deployment reflects the LLSD after userConfig has been removed —
	// only the storage volume remains.
	desiredDeployment := newTestResource(t, "apps/v1", "Deployment", "test-deployment", testNs, map[string]any{
		"replicas": int32(1),
		"selector": map[string]any{
			"matchLabels": map[string]any{"app": "test"},
		},
		"template": map[string]any{
			"metadata": map[string]any{
				"labels": map[string]any{"app": "test"},
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{"name": "main", "image": "test:v2"},
				},
				"volumes": []any{
					map[string]any{
						"name":     "lls-storage",
						"emptyDir": map[string]any{},
					},
				},
			},
		},
	})

	resMap := resmap.New()
	require.NoError(t, resMap.Append(desiredDeployment))

	require.NoError(t, ApplyResources(ctx, k8sClient, scheme.Scheme, owner, &resMap))

	updated := &appsv1.Deployment{}
	require.NoError(t, k8sClient.Get(ctx, types.NamespacedName{Name: "test-deployment", Namespace: testNs}, updated))

	// Verify the user-config volume was removed.
	for _, vol := range updated.Spec.Template.Spec.Volumes {
		require.NotEqual(t, "user-config", vol.Name, "stale user-config volume should have been removed from the Deployment")
	}
	// Verify the storage volume still exists.
	found := false
	for _, vol := range updated.Spec.Template.Spec.Volumes {
		if vol.Name == "lls-storage" {
			found = true
			break
		}
	}
	require.True(t, found, "lls-storage volume should still be present")
}

// TestLegacyCABundleUpgrade tests that deployments with legacy CA bundle volumes
// are replaced instead of patched to avoid SSA conflicts.
func TestLegacyCABundleUpgrade(t *testing.T) {
	ctx, testNs, owner := setupApplyResourcesTest(t, "legacy-ca-upgrade")

	// Create an existing deployment with legacy CA bundle volumes (old operator pattern)
	existingDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-deployment",
			Namespace: testNs,
			Labels:    map[string]string{"version": "old"},
			OwnerReferences: []metav1.OwnerReference{
				*metav1.NewControllerRef(owner, owner.GroupVersionKind()),
			},
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: ptr(int32(1)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "test"},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": "test"},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "main",
							Image: "test:v1",
						},
					},
					// Legacy CA bundle volumes (old operator pattern)
					Volumes: []corev1.Volume{
						{
							Name: "data",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "ca-bundle",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "ca-bundle-source",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: "odh-trusted-ca-bundle",
									},
								},
							},
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:  "ca-bundle-init",
							Image: "busybox",
						},
					},
				},
			},
		},
	}
	require.NoError(t, k8sClient.Create(ctx, existingDeployment))

	// Create desired deployment with new CA bundle pattern (new operator pattern)
	// Must use same selector as existing deployment (selector is immutable)
	desiredDeployment := newTestResource(t, "apps/v1", "Deployment", "test-deployment", testNs, map[string]any{
		"replicas": int32(1),
		"selector": map[string]any{
			"matchLabels": map[string]any{
				"app": "test",
			},
		},
		"template": map[string]any{
			"metadata": map[string]any{
				"labels": map[string]any{
					"app": "test",
				},
			},
			"spec": map[string]any{
				"containers": []any{
					map[string]any{
						"name":  "main",
						"image": "test:v2",
					},
				},
				"volumes": []any{
					map[string]any{
						"name":     "data",
						"emptyDir": map[string]any{},
					},
					// New CA bundle pattern - single ConfigMap volume
					map[string]any{
						"name": "ca-bundle",
						"configMap": map[string]any{
							"name": "managed-ca-bundle",
						},
					},
				},
			},
		},
	})
	desiredDeployment.SetLabels(map[string]string{"version": "new"})

	resMap := resmap.New()
	require.NoError(t, resMap.Append(desiredDeployment))

	// Apply the resources (should trigger replacement instead of patch)
	require.NoError(t, ApplyResources(ctx, k8sClient, scheme.Scheme, owner, &resMap))

	// Verify the deployment was updated
	updatedDeployment := &appsv1.Deployment{}
	deploymentKey := types.NamespacedName{Name: "test-deployment", Namespace: testNs}
	require.NoError(t, k8sClient.Get(ctx, deploymentKey, updatedDeployment))

	// Verify labels were updated (proves Update was used, not just patch)
	require.Equal(t, "new", updatedDeployment.Labels["version"], "deployment labels should be updated")

	// Verify container image was updated
	require.Equal(t, "test:v2", updatedDeployment.Spec.Template.Spec.Containers[0].Image, "container image should be updated")

	// Verify legacy volumes are removed
	volumeNames := make([]string, len(updatedDeployment.Spec.Template.Spec.Volumes))
	for i, vol := range updatedDeployment.Spec.Template.Spec.Volumes {
		volumeNames[i] = vol.Name
	}
	require.NotContains(t, volumeNames, "ca-bundle-source", "legacy ca-bundle-source volume should be removed")

	// Verify new ca-bundle is a ConfigMap (not emptyDir)
	var caBundleVolume *corev1.Volume
	for i := range updatedDeployment.Spec.Template.Spec.Volumes {
		if updatedDeployment.Spec.Template.Spec.Volumes[i].Name == "ca-bundle" {
			caBundleVolume = &updatedDeployment.Spec.Template.Spec.Volumes[i]
			break
		}
	}
	require.NotNil(t, caBundleVolume, "ca-bundle volume should exist")
	require.NotNil(t, caBundleVolume.ConfigMap, "ca-bundle should be a ConfigMap volume")
	require.Nil(t, caBundleVolume.EmptyDir, "ca-bundle should not be an emptyDir volume")
	require.Equal(t, "managed-ca-bundle", caBundleVolume.ConfigMap.Name, "ca-bundle ConfigMap name should be correct")

	// Verify init containers are removed
	require.Empty(t, updatedDeployment.Spec.Template.Spec.InitContainers, "legacy init containers should be removed")
}

// ptr is a helper function to get a pointer to a value.
func ptr[T any](v T) *T {
	return &v
}

func TestGetFieldMappings_RecreateStrategyWithStorage(t *testing.T) {
	t.Run("includes Recreate strategy when storage is configured", func(t *testing.T) {
		owner := &llamav1alpha1.LlamaStackDistribution{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec: llamav1alpha1.LlamaStackDistributionSpec{
				Replicas: 1,
				Server: llamav1alpha1.ServerSpec{
					Storage: &llamav1alpha1.StorageSpec{},
				},
			},
		}

		mappings := getFieldMappings(owner)

		var found bool
		for _, m := range mappings {
			if m.TargetField == "/spec/strategy/type" && m.TargetKind == "Deployment" {
				assert.Equal(t, "Recreate", m.SourceValue)
				assert.True(t, m.CreateIfNotExists)
				found = true
				break
			}
		}
		require.True(t, found, "should include Recreate strategy mapping when storage is configured")
	})

	t.Run("does not include strategy when storage is nil", func(t *testing.T) {
		owner := &llamav1alpha1.LlamaStackDistribution{
			ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "default"},
			Spec: llamav1alpha1.LlamaStackDistributionSpec{
				Replicas: 1,
				Server:   llamav1alpha1.ServerSpec{},
			},
		}

		mappings := getFieldMappings(owner)

		for _, m := range mappings {
			if m.TargetField == "/spec/strategy/type" {
				t.Fatal("should not include strategy mapping when storage is nil")
			}
		}
	})
}

// resourceToUnstructured converts a kustomize resource to an unstructured object.
func resourceToUnstructured(t *testing.T, res *kresource.Resource) (*unstructured.Unstructured, error) {
	t.Helper()

	yamlBytes, err := res.AsYAML()
	if err != nil {
		return nil, err
	}

	u := &unstructured.Unstructured{}
	if err := yaml.Unmarshal(yamlBytes, u); err != nil {
		return nil, err
	}

	return u, nil
}
