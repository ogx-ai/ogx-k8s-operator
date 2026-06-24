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

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
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
	ogxTestNS      = "ogx-test"
)

func TestTLSSuite(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping TLS test suite")
	}

	t.Run("should generate certificates", func(t *testing.T) {
		generateCertificates(t)
	})

	t.Run("should create test namespace", func(t *testing.T) {
		testCreateNamespace(t)
	})

	t.Run("should create OGXServer with CA bundle", func(t *testing.T) {
		testOGXServerWithCABundle(t)
	})

	t.Run("should cleanup TLS resources", func(t *testing.T) {
		testTLSCleanup(t)
	})
}

func testCreateNamespace(t *testing.T) {
	t.Helper()

	testNs := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: ogxTestNS,
		},
	}
	err := TestEnv.Client.Create(TestEnv.Ctx, testNs)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	ensureStarterConfigMap(t, ogxTestNS)

	err = createCABundleConfigMap(t, ogxTestNS)
	require.NoError(t, err)

	err = verifyCABundleConfigMap(t, ogxTestNS)
	require.NoError(t, err)
}

func ensureStarterConfigMap(t *testing.T, namespace string) {
	t.Helper()

	projectRoot, err := filepath.Abs("../..")
	require.NoError(t, err)

	configMapPath := filepath.Join(projectRoot, "config", "samples", "starter-config-configmap.yaml")
	yamlFile, err := os.ReadFile(configMapPath)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	require.NoError(t, yaml.Unmarshal(yamlFile, cm))

	cm.Namespace = namespace
	err = TestEnv.Client.Create(TestEnv.Ctx, cm)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
}

func testOGXServerWithCABundle(t *testing.T) {
	t.Helper()

	err := deployOGXServerWithCABundle(t)
	require.NoError(t, err)

	err = updateCABundleConfigMap(t, ogxTestNS)
	require.NoError(t, err)

	err = verifyCABundleConfigMap(t, ogxTestNS)
	require.NoError(t, err)

	err = verifyOGXServerCABundleConfig(t, ogxTestNS, "ogxserver-with-ca-bundle")
	require.NoError(t, err)

	err = waitForDeploymentCreation(t, ogxTestNS, "ogxserver-with-ca-bundle", 3*time.Minute)
	require.NoError(t, err, "OGXServer deployment should be created by operator")

	err = WaitForPodsReady(t, TestEnv, ogxTestNS, "ogxserver-with-ca-bundle", 5*time.Minute)
	require.NoError(t, err, "OGXServer pods should be running and ready")

	err = verifyCertificateMounts(t, ogxTestNS, "ogxserver-with-ca-bundle")
	require.NoError(t, err, "Certificate volumes should be mounted correctly")

	err = verifyEnvironmentVariables(t, ogxTestNS, "ogxserver-with-ca-bundle")
	require.NoError(t, err, "Environment variables should be set correctly")
}

func testTLSCleanup(t *testing.T) {
	t.Helper()

	server := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "ogxserver-with-ca-bundle",
			Namespace: ogxTestNS,
		},
	}
	err := TestEnv.Client.Delete(TestEnv.Ctx, server)
	if err != nil && !k8serrors.IsNotFound(err) {
		require.NoError(t, err)
	}

	err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
		Group:   "apps",
		Version: "v1",
		Kind:    "Deployment",
	}, "ogxserver-with-ca-bundle", ogxTestNS, ResourceReadyTimeout)
	require.NoError(t, err, "OGXServer deployment should be deleted")
}

func generateCertificates(t *testing.T) {
	t.Helper()

	projectRoot, err := filepath.Abs("../..")
	require.NoError(t, err, "Failed to get project root")

	scriptPath := filepath.Join(projectRoot, "config", "samples", "generate_certificates.sh")
	t.Logf("Running certificate generation script: %s", scriptPath)

	t.Chdir(projectRoot)

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

	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	caBundle, err := os.ReadFile(filepath.Join(projectRoot, "config", "samples", "vllm-ca-certs", controllers.DefaultCABundleKey))
	if err != nil {
		return fmt.Errorf("failed to read CA bundle: %w", err)
	}

	caBundleConfigMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "custom-ca-bundle",
			Namespace: targetNS,
			Labels: map[string]string{
				controllers.WatchLabelKey: controllers.WatchLabelValue,
			},
		},
		Data: map[string]string{
			controllers.DefaultCABundleKey: string(caBundle),
		},
	}

	err = TestEnv.Client.Create(TestEnv.Ctx, caBundleConfigMap)
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
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

	configMap := &corev1.ConfigMap{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: targetNS,
		Name:      "custom-ca-bundle",
	}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get CA bundle ConfigMap: %w", err)
	}

	caBundle, exists := configMap.Data[controllers.DefaultCABundleKey]
	if !exists {
		return fmt.Errorf("failed to find %s CA bundle key in ConfigMap", controllers.DefaultCABundleKey)
	}

	if len(caBundle) == 0 {
		return fmt.Errorf("failed to find any keys in CA bundle ConfigMap %s", controllers.DefaultCABundleKey)
	}

	if len(caBundle) < 100 || !strings.Contains(caBundle, "BEGIN CERTIFICATE") {
		t.Logf("WARNING: CA bundle appears to be a placeholder or invalid")
		t.Logf("CA bundle content: %s", caBundle)

		err := updateCABundleConfigMap(t, targetNS)
		if err != nil {
			t.Logf("Failed to update CA bundle ConfigMap: %v", err)
		}
	}

	return nil
}

func verifyOGXServerCABundleConfig(t *testing.T, namespace, name string) error {
	t.Helper()

	server := &ogxiov1beta1.OGXServer{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, server)
	if err != nil {
		return fmt.Errorf("failed to get OGXServer: %w", err)
	}

	if server.Spec.TLS == nil || server.Spec.TLS.Trust == nil || len(server.Spec.TLS.Trust.CACertificates) == 0 {
		return errors.New("OGXServer does not have CA bundle config (spec.tls.trust.caCertificates)")
	}

	return nil
}

func updateCABundleConfigMap(t *testing.T, targetNS string) error {
	t.Helper()

	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	actualCABundle, err := os.ReadFile(filepath.Join(projectRoot, "config", "samples", "vllm-ca-certs", controllers.DefaultCABundleKey))
	if err != nil {
		return fmt.Errorf("failed to read CA bundle file: %w", err)
	}

	configMap := &corev1.ConfigMap{}
	err = TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: targetNS,
		Name:      "custom-ca-bundle",
	}, configMap)
	if err != nil {
		return fmt.Errorf("failed to get ConfigMap: %w", err)
	}

	configMap.Data[controllers.DefaultCABundleKey] = string(actualCABundle)

	err = TestEnv.Client.Update(TestEnv.Ctx, configMap)
	if err != nil {
		return fmt.Errorf("failed to update ConfigMap: %w", err)
	}

	return nil
}

func deployOGXServerWithCABundle(t *testing.T) error {
	t.Helper()

	logOperatorPodStatus(t, TestEnv, TestOpts.OperatorNS)

	if err := WaitForWebhookReady(t, TestEnv, TestOpts.OperatorNS, 2*time.Minute); err != nil {
		logOperatorPodStatus(t, TestEnv, TestOpts.OperatorNS)
		return fmt.Errorf("failed to wait for webhook readiness: %w", err)
	}

	projectRoot, err := filepath.Abs("../..")
	if err != nil {
		return fmt.Errorf("failed to get project root: %w", err)
	}

	configPath := filepath.Join(projectRoot, "config", "samples", "example-with-ca-bundle.yaml")
	configData, err := os.ReadFile(configPath)
	if err != nil {
		return fmt.Errorf("failed to read OGXServer config: %w", err)
	}

	objects, err := parseKubernetesYAML(configData)
	if err != nil {
		return fmt.Errorf("failed to parse OGXServer config: %w", err)
	}

	for _, obj := range objects {
		if obj.GetNamespace() == "" {
			obj.SetNamespace(ogxTestNS)
		}
		if err := createObjectWithWebhookRetry(t, obj); err != nil {
			return fmt.Errorf("failed to create OGXServer resource: %w", err)
		}
	}

	return nil
}

func createObjectWithWebhookRetry(t *testing.T, obj client.Object) error {
	t.Helper()

	return wait.PollUntilContextTimeout(TestEnv.Ctx, 5*time.Second, 2*time.Minute, true, func(ctx context.Context) (bool, error) {
		err := TestEnv.Client.Create(ctx, obj)
		if err == nil || k8serrors.IsAlreadyExists(err) {
			return true, nil
		}
		if strings.Contains(err.Error(), "connection refused") || strings.Contains(err.Error(), "webhook") {
			t.Logf("Webhook not ready yet, retrying: %v", err)
			return false, nil
		}
		return false, err
	})
}

func verifyCertificateMounts(t *testing.T, namespace, name string) error {
	t.Helper()

	deployment := &appsv1.Deployment{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, deployment)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

	if !hasCABundleVolume(deployment.Spec.Template.Spec.Volumes) {
		return errors.New("CA bundle volume not found in deployment")
	}

	if !hasCABundleMount(deployment.Spec.Template.Spec.Containers) {
		return errors.New("CA bundle mount not found in any container")
	}

	return nil
}

func hasCABundleVolume(volumes []corev1.Volume) bool {
	for _, volume := range volumes {
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

	deployment := &appsv1.Deployment{}
	err := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: namespace,
		Name:      name,
	}, deployment)
	if err != nil {
		return fmt.Errorf("failed to get deployment: %w", err)
	}

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
	docs := yamlSplit(data)
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
		server := &ogxiov1beta1.OGXServer{}
		err := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: namespace,
			Name:      name,
		}, server)
		if err != nil {
			t.Logf("OGXServer not found yet: %v", err)
			return false, nil
		}

		t.Logf("OGXServer status: Phase=%s", server.Status.Phase)

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
