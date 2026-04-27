package deploy

import (
	"maps"
	"os"
	"path/filepath"
	"testing"

	llamav1alpha1 "github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
	"github.com/stretchr/testify/require"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	"sigs.k8s.io/kustomize/api/resource"
	"sigs.k8s.io/kustomize/kyaml/yaml"
)

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.WriteTo(os.Stdout), zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		logf.Log.Error(err, "failed to start test environment")
		os.Exit(1)
	}

	err = llamav1alpha1.AddToScheme(scheme.Scheme)
	if err != nil {
		logf.Log.Error(err, "failed to add scheme")
		os.Exit(1)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create client")
		os.Exit(1)
	}

	code := m.Run()

	err = testEnv.Stop()
	if err != nil {
		logf.Log.Error(err, "failed to stop test environment")
		os.Exit(1)
	}

	os.Exit(code)
}

// newTestResource is a helper function to create a kustomize resource for testing.
func newTestResource(t *testing.T, apiVersion, kind, name, namespace string, content map[string]any) *resource.Resource {
	t.Helper()

	obj := map[string]any{
		"apiVersion": apiVersion,
		"kind":       kind,
		"metadata":   map[string]any{"name": name, "namespace": namespace},
	}

	switch kind {
	case deploymentKind:
		baseDeploymentContent := map[string]any{
			"selector": map[string]any{
				"matchLabels": map[string]any{"app": name},
			},
			"template": map[string]any{
				"metadata": map[string]any{"labels": map[string]any{"app": name}},
				"spec": map[string]any{
					"containers": []map[string]any{
						{"name": "test-container", "image": "nginx"},
					},
				},
			},
		}

		// Overlay the caller's content on top of the base spec.
		maps.Copy(baseDeploymentContent, content)
		obj["spec"] = baseDeploymentContent
	case "ClusterRole":
		// For ClusterRole, the content is at the top level, not under 'spec'.
		maps.Copy(obj, content)
	default:
		// For other simple types, assume the content is the spec.
		obj["spec"] = content
	}

	yamlBytes, err := yaml.Marshal(obj)
	require.NoError(t, err)

	rf := resource.NewFactory(nil)
	res, err := rf.FromBytes(yamlBytes)
	require.NoError(t, err)

	return res
}
