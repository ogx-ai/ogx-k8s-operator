//nolint:testpackage
package e2e

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
	"github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/yaml"
)

const (
	tlsTestTimeout = 5 * time.Minute
	llsTestNS      = "llama-stack-test"
)

func TestTLSSuite(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping TLS test suite")
	}

	// Generate certificates before running any tests
	t.Run("should generate certificates", func(t *testing.T) {
		generateCertificates(t)
	})

	t.Run("should create test namespace", func(t *testing.T) {
		testCreateNamespace(t)
	})

	t.Run("should create LlamaStackDistribution with CA bundle", func(t *testing.T) {
		testLlamaStackWithCABundle(t)
	})

	t.Run("should cleanup TLS resources", func(t *testing.T) {
		testTLSCleanup(t)
	})
}

func testCreateNamespace(t *testing.T) {
	t.Helper()

	// Create test namespace
	testNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: llsTestNS,
		},
	}
	err := TestEnv.Client.Create(TestEnv.Ctx, testNs)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	// Create CA bundle configmap in test namespace
	err = createCABundleConfigMap(t, llsTestNS)
	require.NoError(t, err)

	// Verify the CA bundle ConfigMap was created correctly
	err = verifyCABundleConfigMap(t, llsTestNS)
	require.NoError(t, err)
}

func testLlamaStackWithCABundle(t *testing.T) {
	t.Helper()

	// Deploy LlamaStackDistribution with CA bundle
	err := deployLlamaStackWithCABundle(t)
	require.NoError(t, err)

	// The YAML file creates a placeholder ConfigMap, so we need to update it with the actual CA bundle
	err = updateCABundleConfigMap(t, llsTestNS)
	require.NoError(t, err)

	// Verify the CA bundle ConfigMap has the correct content after update
	err = verifyCABundleConfigMap(t, llsTestNS)
	require.NoError(t, err)

	// Verify the LlamaStack distribution is configured with TLS
	err = verifyLlamaStackTLSConfig(t, llsTestNS, "llamastack-with-config")
	require.NoError(t, err)

	// Wait for the operator to process the LlamaStackDistribution and create the deployment
	err = waitForDeploymentCreation(t, llsTestNS, "llamastack-with-config", 3*time.Minute)
	require.NoError(t, err, "LlamaStack deployment should be created by operator")

	// Wait for pods to be running and ready
	err = WaitForPodsReady(t, TestEnv, llsTestNS, "llamastack-with-config", 5*time.Minute)
	require.NoError(t, err, "LlamaStack pods should be running and ready")

	// Verify certificate volumes are mounted correctly
	err = verifyCertificateMounts(t, llsTestNS, "llamastack-with-config")
	require.NoError(t, err, "Certificate volumes should be mounted correctly")

	// Verify environment variables are set correctly
	err = verifyEnvironmentVariables(t, llsTestNS, "llamastack-with-config")
	require.NoError(t, err, "Environment variables should be set correctly")
}

func testTLSCleanup(t *testing.T) {
	t.Helper()

	// Delete LlamaStackDistribution
	distribution := &v1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "llamastack-with-config",
			Namespace: llsTestNS,
		},
	}
	err := TestEnv.Client.Delete(TestEnv.Ctx, distribution)
	if err != nil && !k8serrors.IsNotFound(err) {
		require.NoError(t, err)
	}

	// Wait for LlamaStack resources to be cleaned up
	err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, "llamastack-with-config", llsTestNS, ResourceReadyTimeout)
	require.NoError(t, err, "LlamaStack deployment should be deleted")
}

// Helper functions

func generateCertificates(t *testing.T) {
	t.Helper()

	// Get the project root path
	projectRoot, err := filepath.Abs("../..")
	require.NoError(t, err, "Failed to get project root")

	// Run the certificate generation script
	scriptPath := filepath.Join(projectRoot, "config", "samples", "generate_certificates.sh")
	t.Logf("Running certificate generation script: %s", scriptPath)

	// Change to the project root directory to run the script
	t.Chdir(projectRoot)

	// Execute the script
	cmd := exec.CommandContext(t.Context(), "bash", scriptPath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Logf("Certificate generation script output: %s", string(output))
		require.NoError(t, err, "Failed to run certificate generation script")
	}

	t.Log("Certificates generated successfully")
}

func createCABundleConfigMap(t *testing.T, targetNS string) error {
	t.Helper()

	// Get the project root path
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	// Read CA bundle
	caBundle, err := os.ReadFile(filepath.Join(projectRoot, "config", "samples", "vllm-ca-certs", controllers.DefaultCABundleKey))
	if err != nil {
		return fmt.Errorf("failed to read CA bundle: %w", err)
	}

	caBundleConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-ca-bundle",
			Namespace: targetNS,
		},
		Data: map[string]string{
			controllers.DefaultCABundleKey: string(caBundle),
		},
	}

	// Try to create, if it exists, update it
	err = TestEnv.Client.Create(TestEnv.Ctx, caBundleConfigMap)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			// ConfigMap exists, update it
			existingConfigMap := &corev1.ConfigMap{}
			err = TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
				Namespace: targetNS,
				Name:      "custom-ca-bundle",
			}, existingConfigMap)
			if err != nil {
				return fmt.Errorf("failed to get existing ConfigMap: %w", err)
			}

			existingConfigMap.Data[controllers.DefaultCABundleKey] = string(caBundle)
			err = TestEnv.Client.Update(TestEnv.Ctx, existingConfigMap)
			if err != nil {
				return fmt.Errorf("failed to update existing ConfigMap: %w", err)
			}
		} else {
			return fmt.Errorf("failed to create CA bundle configmap: %w", err)
		}
	} else {
		t.Logf("Created CA bundle ConfigMap with %d bytes", len(caBundle))
	}

	return nil
}

func verifyCABundleConfigMap(t *testing.T, targetNS string) error {
	t.Helper()

	// Get the ConfigMap
	configMap := &corev1.ConfigMap{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: targetNS,
		Name:      "custom-ca-bundle",
	}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get CA bundle ConfigMap: %w", err)
	}

	// Verify the CA bundle content exists
	caBundle, exists := configMap.Data[controllers.DefaultCABundleKey]
	if !exists {
		return fmt.Errorf("failed to find %s CA bundle key in ConfigMap", controllers.DefaultCABundleKey)
	}

	if len(caBundle) == 0 {
		return fmt.Errorf("failed to find any keys in CA bundle ConfigMap %s", controllers.DefaultCABundleKey)
	}

	// Check if CA bundle appears to be a placeholder
	if len(caBundle) < 100 || !strings.Contains(caBundle, "BEGIN CERTIFICATE") {
		t.Logf("WARNING: CA bundle appears to be a placeholder or invalid")
		t.Logf("CA bundle content: %s", caBundle)

		// Try to update the ConfigMap with the actual CA bundle from the file
		err := updateCABundleConfigMap(t, targetNS)
		if err != nil {
			t.Logf("Failed to update CA bundle ConfigMap: %v", err)
		}
	}

	return nil
}

func verifyLlamaStackTLSConfig(t *testing.T, namespace, name string) error {
	t.Helper()

	// Get the LlamaStack distribution
	distribution := &v1alpha1.LlamaStackDistribution{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, distribution)
	if err != nil {
		return fmt.Errorf("failed to get LlamaStack distribution: %w", err)
	}

	// Verify TLS configuration is present
	if distribution.Spec.Server.TLSConfig == nil {
		return errors.New("LlamaStack distribution does not have TLS config")
	}

	if distribution.Spec.Server.TLSConfig.CABundle == nil {
		return errors.New("LlamaStack distribution TLS config does not have CA bundle")
	}

	return nil
}

func updateCABundleConfigMap(t *testing.T, targetNS string) error {
	t.Helper()

	// Get the project root path
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	// Read the actual CA bundle from the file
	actualCABundle, err := os.ReadFile(filepath.Join(projectRoot, "config", "samples", "vllm-ca-certs", controllers.DefaultCABundleKey))
	if err != nil {
		return fmt.Errorf("failed to read CA bundle file: %w", err)
	}

	// Get the existing ConfigMap
	configMap := &corev1.ConfigMap{}
	err = TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: targetNS,
		Name:      "custom-ca-bundle",
	}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	// Update the ConfigMap with the actual CA bundle
	configMap.Data[controllers.DefaultCABundleKey] = string(actualCABundle)

	err = TestEnv.Client.Update(TestEnv.Ctx, configMap)
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	return nil
}

func deployLlamaStackWithCABundle(t *testing.T) error {
	t.Helper()

	// Read LlamaStack TLS test configuration
	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	llamaStackConfigPath := filepath.Join(projectRoot, "config", "samples", "example-with-ca-bundle.yaml")
	llamaStackConfigData, err := os.ReadFile(llamaStackConfigPath)
	if err != nil {
		return fmt.Errorf("failed to read LlamaStack config: %w", err)
	}

	// Apply LlamaStack configuration
	objects, err := parseKubernetesYAML(llamaStackConfigData)
	if err != nil {
		return fmt.Errorf("failed to parse LlamaStack config: %w", err)
	}

	for _, obj := range objects {
		// Set the namespace for namespaced resources
		if obj.GetNamespace() == "" {
			obj.SetNamespace(llsTestNS)
		}

		err = TestEnv.Client.Create(TestEnv.Ctx, obj)
		if err != nil && !k8serrors.IsAlreadyExists(err) {
			return fmt.Errorf("failed to create LlamaStack resource: %w", err)
		}
	}

	return nil
}

func verifyCertificateMounts(t *testing.T, namespace, name string) error {
	t.Helper()

	// Get the deployment
	deployment := &appsv1.Deployment{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, deployment)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Check if CA bundle volume is defined
	if !hasCABundleVolume(deployment.Spec.Template.Spec.Volumes) {
		return errors.New("CA bundle volume not found in deployment")
	}

	// Check if CA bundle is mounted in any container
	if !hasCABundleMount(deployment.Spec.Template.Spec.Containers) {
		return errors.New("CA bundle mount not found in any container")
	}

	return nil
}

func hasCABundleVolume(volumes []corev1.Volume) bool {
	for _, volume := range volumes {
		// Check for the managed CA bundle ConfigMap (named {instance-name}-ca-bundle)
		// or the source CA bundle ConfigMap
		if volume.ConfigMap != nil &&
			(strings.HasSuffix(volume.ConfigMap.Name, "-ca-bundle") ||
				volume.ConfigMap.Name == "custom-ca-bundle") {
			return true
		}
	}
	return false
}

func hasCABundleMount(containers []corev1.Container) bool {
	for _, container := range containers {
		if hasCABundleMountInContainer(container.VolumeMounts) {
			return true
		}
	}
	return false
}

func hasCABundleMountInContainer(mounts []corev1.VolumeMount) bool {
	for _, mount := range mounts {
		if mount.MountPath == controllers.ManagedCABundleMountPath ||
			strings.Contains(mount.MountPath, "ca-bundle") ||
			strings.Contains(mount.MountPath, "ca-certificates") {
			return true
		}
	}
	return false
}

func verifyEnvironmentVariables(t *testing.T, namespace, name string) error {
	t.Helper()

	// Get the deployment
	deployment := &appsv1.Deployment{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, deployment)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	// Check for TLS-related environment variables
	tlsEnvVarsFound := 0
	expectedEnvVars := map[string]string{
		"SSL_CERT_FILE": controllers.ManagedCABundleFilePath,
	}

	for _, container := range deployment.Spec.Template.Spec.Containers {
		for _, env := range container.Env {
			if expectedValue, exists := expectedEnvVars[env.Name]; exists {
				if env.Value == expectedValue {
					tlsEnvVarsFound++
				} else {
					t.Logf("Found env var with unexpected value: %s=%s (expected: %s)",
						env.Name, env.Value, expectedValue)
				}
			}
		}
	}

	if tlsEnvVarsFound == 0 {
		return errors.New("no expected TLS-related environment variables found")
	}

	return nil
}

func parseKubernetesYAML(data []byte) ([]client.Object, error) {
	// Split YAML documents
	docs := yamlSplit(data)

	// Pre-allocate slice with expected capacity
	objects := make([]client.Object, 0, len(docs))

	for _, doc := range docs {
		if len(doc) == 0 {
			continue
		}

		obj := &unstructured.Unstructured{}
		err := yaml.Unmarshal(doc, obj)
		if err != nil {
			return nil, fmt.Errorf("failed to unmarshal YAML: %w", err)
		}

		if obj.GetKind() == "" {
			continue
		}

		objects = append(objects, obj)
	}

	return objects, nil
}

func yamlSplit(data []byte) [][]byte {
	var docs [][]byte
	var currentDoc []byte

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		if strings.TrimSpace(line) == "---" {
			if len(currentDoc) > 0 {
				docs = append(docs, currentDoc)
				currentDoc = nil
			}
		} else {
			currentDoc = append(currentDoc, []byte(line+"\n")...)
		}
	}

	if len(currentDoc) > 0 {
		docs = append(docs, currentDoc)
	}

	return docs
}

func waitForDeploymentCreation(t *testing.T, namespace, name string, timeout time.Duration) error {
	t.Helper()

	return wait.PollUntilContextTimeout(TestEnv.Ctx, 10*time.Second, timeout, true, func(ctx context.Context) (bool, error) {
		// First check if the LlamaStackDistribution is being processed
		distribution := &v1alpha1.LlamaStackDistribution{}
		err := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, distribution)
		if err != nil {
			t.Logf("LlamaStackDistribution not found yet: %v", err)
			return false, nil
		}

		t.Logf("LlamaStackDistribution status: Phase=%s", distribution.Status.Phase)

		// Then check if the deployment has been created
		deployment := &appsv1.Deployment{}
		err = TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, deployment)
		if err != nil {
			if k8serrors.IsNotFound(err) {
				t.Logf("Deployment %s not created yet by operator, continuing to wait...", name)
				return false, nil
			}
			t.Logf("Error getting deployment: %v", err)
			return false, err
		}

		t.Logf("Deployment %s found, created by operator", name)
		return true, nil
	})
}
