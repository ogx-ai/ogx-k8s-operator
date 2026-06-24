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
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildIngress(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))

	clusterInfo := &cluster.ClusterInfo{
		DistributionImages: map[string]string{"starter": "test-image:latest"},
	}

	reconciler := controllers.NewTestReconciler(nil, scheme, clusterInfo, nil)

	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ogx",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
			Network: &ogxiov1beta1.NetworkSpec{
				ExternalAccess: &ogxiov1beta1.ExternalAccessConfig{Enabled: true},
			},
		},
	}

	ingress, err := reconciler.BuildIngressForTest(instance)
	require.NoError(t, err)
	require.NotNil(t, ingress)

	assert.Equal(t, "test-ogx-ingress", ingress.Name)
	assert.Equal(t, "test-ns", ingress.Namespace)

	require.Len(t, ingress.Spec.Rules, 1)
	require.NotNil(t, ingress.Spec.Rules[0].HTTP)
	require.Len(t, ingress.Spec.Rules[0].HTTP.Paths, 1)

	assert.Equal(t, "test-ogx-service", ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name)
	assert.Equal(t, int32(8321), ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number)
}

func TestBuildIngress_CustomPort(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))

	clusterInfo := &cluster.ClusterInfo{
		DistributionImages: map[string]string{"starter": "test-image:latest"},
	}

	reconciler := controllers.NewTestReconciler(nil, scheme, clusterInfo, nil)

	instance := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-ogx",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
			Network: &ogxiov1beta1.NetworkSpec{
				Port:           9000,
				ExternalAccess: &ogxiov1beta1.ExternalAccessConfig{Enabled: true},
			},
		},
	}

	ingress, err := reconciler.BuildIngressForTest(instance)
	require.NoError(t, err)
	require.NotNil(t, ingress)

	assert.Equal(t, int32(9000), ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number)
}
