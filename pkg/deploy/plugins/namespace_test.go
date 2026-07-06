package plugins

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
)

func TestNamespacePlugin(t *testing.T) {
	testNamespace := "my-test-ns"
	t.Run("apply namespace to namespaced resource", func(t *testing.T) {
		resMap := resmap.New()
		// create a deployment with no initial namespace
		dep := newTestResource(t, "apps/v1", "Deployment", "my-app", "", nil)
		require.NoError(t, resMap.Append(dep))

		plugin, err := CreateNamespacePlugin(testNamespace)
		require.NoError(t, err)
		err = plugin.Transform(resMap)
		require.NoError(t, err)

		var transformedDep *resource.Resource
		for _, r := range resMap.Resources() {
			if r.CurId() == dep.CurId() {
				transformedDep = r
				break
			}
		}
		require.NotNil(t, transformedDep, "transformed deployment not found in resMap")
		assert.Equal(t, testNamespace, transformedDep.GetNamespace())
	})

	t.Run("skips cluster scoped resource", func(t *testing.T) {
		resMap := resmap.New()
		// create a cluster role with no initial namespace (as it's cluster-scoped)
		clusterRole := newTestResource(t, "rbac.authorization.k8s.io/v1", "ClusterRole", "admin-role", "", nil)
		require.NoError(t, resMap.Append(clusterRole))

		plugin, err := CreateNamespacePlugin(testNamespace)
		require.NoError(t, err)
		err = plugin.Transform(resMap)
		require.NoError(t, err)

		var transformedClusterRole *resource.Resource
		for _, r := range resMap.Resources() {
			if r.CurId() == clusterRole.CurId() {
				transformedClusterRole = r
				break
			}
		}
		require.NotNil(t, transformedClusterRole, "transformed ClusterRole not found in resMap")
		assert.Empty(t, transformedClusterRole.GetNamespace(), "ClusterRole should remain cluster-scoped")
	})

	t.Run("apply namespace to service", func(t *testing.T) {
		resMap := resmap.New()
		// create a service with no initial namespace
		svc := newTestResource(t, "v1", "Service", "my-service", "", nil)
		require.NoError(t, resMap.Append(svc))

		plugin, err := CreateNamespacePlugin(testNamespace)
		require.NoError(t, err)
		err = plugin.Transform(resMap)
		require.NoError(t, err)

		var transformedSvc *resource.Resource
		for _, r := range resMap.Resources() {
			if r.CurId() == svc.CurId() {
				transformedSvc = r
				break
			}
		}
		require.NotNil(t, transformedSvc, "transformed service not found in resMap")
		assert.Equal(t, testNamespace, transformedSvc.GetNamespace())
	})

	t.Run("fails with empty namespace", func(t *testing.T) {
		resMap := resmap.New()
		// create any namespaced resource
		dep := newTestResource(t, "apps/v1", "Deployment", "my-app", "", nil)
		require.NoError(t, resMap.Append(dep))

		_, err := CreateNamespacePlugin("")
		require.Error(t, err)
		assert.Contains(t, err.Error(), "namespace")
		assert.Contains(t, err.Error(), "empty")
	})

	t.Run("overwrites namespace", func(t *testing.T) {
		resMap := resmap.New()
		// resource with a pre-existing namespace
		pvc := newTestResource(t, "v1", "PersistentVolumeClaim", "my-pvc", "old-namespace", nil)
		require.NoError(t, resMap.Append(pvc))

		plugin, err := CreateNamespacePlugin(testNamespace)
		require.NoError(t, err)
		err = plugin.Transform(resMap)
		require.NoError(t, err)

		var transformedPvc *resource.Resource
		for _, r := range resMap.Resources() {
			if r.CurId() == pvc.CurId() {
				transformedPvc = r
				break
			}
		}
		require.NotNil(t, transformedPvc, "transformed pvc not found in resMap")
		assert.Equal(t, testNamespace, transformedPvc.GetNamespace())
	})
}
