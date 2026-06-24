package compare_test

import (
	"testing"

	"github.com/ogx-ai/ogx-k8s-operator/pkg/compare"
	"github.com/stretchr/testify/assert"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/intstr"
)

// baseService is a helper to create a consistent Service object for tests.
func baseService() *corev1.Service {
	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:            "test-service",
			Namespace:       "default",
			ResourceVersion: "1",
			Labels: map[string]string{
				"app": "my-app",
			},
		},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       80,
					TargetPort: intstr.FromInt(8080),
				},
			},
			Selector: map[string]string{
				"app": "my-app",
			},
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: "10.0.0.1",
		},
		Status: corev1.ServiceStatus{
			LoadBalancer: corev1.LoadBalancerStatus{
				Ingress: []corev1.LoadBalancerIngress{
					{IP: "192.168.1.100"},
				},
			},
		},
	}
}

func TestHasUnexpectedServiceChanges(t *testing.T) {
	testCases := []struct {
		name         string
		modifier     func(s *corev1.Service) // A function to modify the base service
		expectChange bool
	}{
		{
			name:         "no changes detected",
			modifier:     func(s *corev1.Service) {}, // No changes
			expectChange: false,
		},
		{
			name: "only managed port changed",
			modifier: func(s *corev1.Service) {
				s.Spec.Ports[0].Port = 8081
			},
			expectChange: false,
		},
		{
			name: "only managed selector changed",
			modifier: func(s *corev1.Service) {
				s.Spec.Selector["version"] = "v2"
			},
			expectChange: false,
		},
		{
			name: "only cluster-managed metadata changed",
			modifier: func(s *corev1.Service) {
				s.ResourceVersion = "2"
			},
			expectChange: false,
		},
		{
			name: "only status changed",
			modifier: func(s *corev1.Service) {
				s.Status.LoadBalancer.Ingress[0].IP = "192.168.1.101"
			},
			expectChange: false,
		},
		{
			name: "unexpected immutable field changed - ClusterIP",
			modifier: func(s *corev1.Service) {
				s.Spec.ClusterIP = "10.0.0.2"
			},
			expectChange: true,
		},
		{
			name: "unexpected field changed - ServiceType",
			modifier: func(s *corev1.Service) {
				s.Spec.Type = corev1.ServiceTypeNodePort
			},
			expectChange: true,
		},
		{
			name: "combination of managed and unexpected changes",
			modifier: func(s *corev1.Service) {
				s.Spec.Ports[0].Port = 9000    // Managed change
				s.Spec.ClusterIP = "10.0.0.99" // Unexpected change
			},
			expectChange: true,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			current := baseService()
			desired := baseService()
			tc.modifier(desired)

			// Act
			hasChanged, diff := compare.HasUnexpectedServiceChanges(desired, current)

			// Assert
			assert.Equal(t, tc.expectChange, hasChanged)

			if tc.expectChange {
				assert.NotEmpty(t, diff, "expected a diff for an unexpected change, but it was empty")
			} else {
				assert.Empty(t, diff, "expected no diff, but changes were detected")
			}
		})
	}
}
