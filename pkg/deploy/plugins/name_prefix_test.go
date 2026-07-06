package plugins

import (
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/kustomize/api/resmap"
	"sigs.k8s.io/kustomize/api/resource"
)

func TestNamePrefixTransformer(t *testing.T) {
	testCases := []struct {
		name               string
		transformer        *namePrefixTransformer
		initialResources   []*resource.Resource
		expectedFinalNames []string
		expectError        bool
		expectedErrStr     string
	}{
		{
			name: "apply prefix to all resources when no filters are set",
			transformer: CreateNamePrefixPlugin(NamePrefixConfig{
				Prefix: "my-prefix",
			}),
			initialResources: []*resource.Resource{
				newTestResource(t, "apps/v1", "Deployment", "backend", "", nil),
				newTestResource(t, "v1", "Service", "frontend", "", nil),
			},
			expectedFinalNames: []string{"my-prefix-backend", "my-prefix-frontend"},
		},
		{
			name: "apply prefix only to included kinds",
			transformer: CreateNamePrefixPlugin(NamePrefixConfig{
				Prefix:       "my-prefix",
				IncludeKinds: []string{"Deployment"},
			}),
			initialResources: []*resource.Resource{
				newTestResource(t, "apps/v1", "Deployment", "backend", "", nil),
				newTestResource(t, "v1", "Service", "frontend", "", nil),
			},
			expectedFinalNames: []string{"my-prefix-backend", "frontend"},
		},
		{
			name: "do not apply prefix to excluded kinds",
			transformer: CreateNamePrefixPlugin(NamePrefixConfig{
				Prefix:       "my-prefix",
				ExcludeKinds: []string{"Service"},
			}),
			initialResources: []*resource.Resource{
				newTestResource(t, "apps/v1", "Deployment", "backend", "", nil),
				newTestResource(t, "v1", "Service", "frontend", "", nil),
			},
			expectedFinalNames: []string{"my-prefix-backend", "frontend"},
		},
		{
			name: "do not apply prefix if already present",
			transformer: CreateNamePrefixPlugin(NamePrefixConfig{
				Prefix: "my-prefix",
			}),
			initialResources: []*resource.Resource{
				newTestResource(t, "apps/v1", "Deployment", "my-prefix-backend", "", nil),
				newTestResource(t, "v1", "Service", "frontend", "", nil),
			},
			expectedFinalNames: []string{"my-prefix-backend", "my-prefix-frontend"},
		},
		{
			name: "error on invalid prefixed name",
			transformer: CreateNamePrefixPlugin(NamePrefixConfig{
				Prefix: "my.prefix",
			}),
			initialResources: []*resource.Resource{
				newTestResource(t, "apps/v1", "Deployment", "backend", "", nil),
			},
			expectError:    true,
			expectedErrStr: "failed to make valid prefixed name",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// setup
			resMap := resmap.New()
			for _, res := range tc.initialResources {
				err := resMap.Append(res)
				require.NoError(t, err)
			}

			// action
			err := tc.transformer.Transform(resMap)

			// assertion
			if tc.expectError {
				require.Error(t, err)
				require.Contains(t, err.Error(), tc.expectedErrStr)
			} else {
				require.NoError(t, err)
				require.Equal(t, len(tc.initialResources), resMap.Size())

				actualFinalNames := make([]string, 0, resMap.Size())
				for _, r := range resMap.Resources() {
					actualFinalNames = append(actualFinalNames, r.GetName())
				}
				require.ElementsMatch(t, tc.expectedFinalNames, actualFinalNames)
			}
		})
	}
}
