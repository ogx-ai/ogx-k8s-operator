package plugins

import (
	"maps"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/api/resource"
	yaml "sigs.k8s.io/kustomize/kyaml/yaml" // Corrected and aliased import
)

// newTestResource is a helper function to create a kustomize resource for testing.
// It is the central helper for all tests in the 'plugins' package.
func newTestResource(t *testing.T, apiVersion, kind, name, namespace string, content map[string]any) *resource.Resource {
	t.Helper()

	obj := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}

	// Logic to construct the object body based on Kind.
	// This ensures resources like Deployment and ClusterRole are valid.
	switch kind {
	case "Deployment":
		baseDeploymentContent := map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{"app": name},
			},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"app": name}},
				"spec": map[string]any{
					"containers": []any{
						map[string]any{"name": "test-container", "image": "nginx"},
					},
				},
			},
		}
		// Overlay caller's content (e.g., "replicas") onto the base Deployment structure.
		maps.Copy(baseDeploymentContent, content)
		obj["spec"] = baseDeploymentContent
	case "ClusterRole":
		// For ClusterRole, its main content (rules) is at the top level, not under 'spec'.
		maps.Copy(obj, content)
	default:
		// For other simple types (like Service, PVC), assume content is the 'spec'.
		obj["spec"] = content
	}

	yamlBytes, err := yaml.Marshal(obj)
	require.NoError(t, err)

	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes(yamlBytes)
	require.NoError(t, err)

	return res
}

func TestValidateK8sLabelName(t *testing.T) {
	// Since the upstream k8svalidation.IsDNS1123Label is already thoroughly
	// tested, we only need to test that our wrapper function correctly
	// handles the success and error formatting cases.
	testCases := []struct {
		name          string
		inputName     string
		expectError   bool
		expectedCause string
	}{
		{
			name:        "valid name passes validation",
			inputName:   "a-valid-name",
			expectError: false,
		},
		{
			name:          "invalid name fails validation",
			inputName:     "Invalid-Uppercase",
			expectError:   true,
			expectedCause: "failed to validate name \"Invalid-Uppercase\"",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			err := ValidateK8sLabelName(tc.inputName)

			if tc.expectError && err == nil {
				t.Errorf("expected an error for name %q, but got none", tc.inputName)
			}

			if !tc.expectError && err != nil {
				t.Errorf("expected no error for name %q, but got: %v", tc.inputName, err)
			}

			if tc.expectError && err != nil {
				if !strings.Contains(err.Error(), tc.expectedCause) {
					t.Errorf("for name %q, expected error to contain %q, but got: %v",
						tc.inputName, tc.expectedCause, err)
				}
			}
		})
	}
}
