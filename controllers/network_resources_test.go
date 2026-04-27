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

	llamav1alpha1 "github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
	"github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

func TestBuildIngress(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, llamav1alpha1.AddToScheme(scheme))

	clusterInfo := &cluster.ClusterInfo{
		DistributionImages: map[string]string{"starter": "test-image:latest"},
	}

	reconciler := controllers.NewTestReconciler(nil, scheme, clusterInfo, nil, true)

	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llsd",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Replicas: 1,
			Server: llamav1alpha1.ServerSpec{
				Distribution: llamav1alpha1.DistributionType{
					Name: "starter",
				},
			},
			Network: &llamav1alpha1.NetworkSpec{
				ExposeRoute: true,
			},
		},
	}

	ingress, err := reconciler.BuildIngressForTest(instance)
	require.NoError(t, err)
	require.NotNil(t, ingress)

	// Verify Ingress metadata
	assert.Equal(t, "test-llsd-ingress", ingress.Name)
	assert.Equal(t, "test-ns", ingress.Namespace)

	// Verify Ingress spec
	require.Len(t, ingress.Spec.Rules, 1)
	require.NotNil(t, ingress.Spec.Rules[0].HTTP)
	require.Len(t, ingress.Spec.Rules[0].HTTP.Paths, 1)

	// Verify backend points to the service
	assert.Equal(t, "test-llsd-service", ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Name)
	assert.Equal(t, int32(8321), ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number)
}

func TestBuildIngress_CustomPort(t *testing.T) {
	scheme := runtime.NewScheme()
	require.NoError(t, llamav1alpha1.AddToScheme(scheme))

	clusterInfo := &cluster.ClusterInfo{
		DistributionImages: map[string]string{"starter": "test-image:latest"},
	}

	reconciler := controllers.NewTestReconciler(nil, scheme, clusterInfo, nil, true)

	instance := &llamav1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-llsd",
			Namespace: "test-ns",
			UID:       "test-uid",
		},
		Spec: llamav1alpha1.LlamaStackDistributionSpec{
			Replicas: 1,
			Server: llamav1alpha1.ServerSpec{
				Distribution: llamav1alpha1.DistributionType{
					Name: "starter",
				},
				ContainerSpec: llamav1alpha1.ContainerSpec{
					Port: 9000,
				},
			},
			Network: &llamav1alpha1.NetworkSpec{
				ExposeRoute: true,
			},
		},
	}

	ingress, err := reconciler.BuildIngressForTest(instance)
	require.NoError(t, err)
	require.NotNil(t, ingress)

	// Verify custom port is used
	assert.Equal(t, int32(9000), ingress.Spec.Rules[0].HTTP.Paths[0].Backend.Service.Port.Number)
}
