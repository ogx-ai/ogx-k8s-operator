//nolint:testpackage
package e2e

import (
	"context"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const (
	rolloutTestNS           = "ogx-rollout-test"
	rolloutTestTimeout      = 5 * time.Minute
	rolloutCRName           = "rollout-test"
	ollamaInferenceModelEnv = "OLLAMA_INFERENCE_MODEL"
)

// TestRolloutWithStorage verifies that updating an OGXServer
// with persistent storage completes without RWO PVC multi-attach deadlock.
func TestRolloutWithStorage(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping rollout test suite")
	}

	nodeList := &corev1.NodeList{}
	require.NoError(t, TestEnv.Client.List(TestEnv.Ctx, nodeList))
	if len(nodeList.Items) <= 1 {
		t.Skip("Skipping: requires multi-node cluster to test RWO PVC rollout behavior")
	}

	t.Run("should create test namespace", func(t *testing.T) {
		testCreateRolloutNamespace(t)
	})

	t.Run("should create OGXServer with storage", func(t *testing.T) {
		testCreateServerWithStorage(t)
	})

	t.Run("should use Recreate strategy when storage is configured", func(t *testing.T) {
		testDeploymentStrategy(t)
	})

	t.Run("should complete rollout after env var update", func(t *testing.T) {
		testRolloutAfterEnvVarUpdate(t)
	})

	t.Run("should have no FailedAttachVolume events", func(t *testing.T) {
		testNoFailedAttachVolumeEvents(t)
	})

	t.Run("should cleanup rollout test resources", func(t *testing.T) {
		testRolloutCleanup(t)
	})
}

func testCreateRolloutNamespace(t *testing.T) {
	t.Helper()

	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: rolloutTestNS},
	}
	err := TestEnv.Client.Create(TestEnv.Ctx, ns)
	if err != nil && !k8serrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
}

func testCreateServerWithStorage(t *testing.T) {
	t.Helper()

	replicas := int32(1)
	cr := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rolloutCRName,
			Namespace: rolloutTestNS,
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{
				Name: starterDistType,
			},
			Workload: &ogxiov1beta1.WorkloadSpec{
				Replicas: &replicas,
				Storage:  &ogxiov1beta1.PVCStorageSpec{},
				Overrides: &ogxiov1beta1.WorkloadOverrides{
					Env: []corev1.EnvVar{
						{Name: ollamaInferenceModelEnv, Value: "llama3.2:1b"},
						{Name: "OLLAMA_URL", Value: "http://ollama-server-service.ollama-dist.svc.cluster.local:11434"},
					},
				},
			},
		},
	}

	t.Log("Creating OGXServer with persistent storage")
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, cr))

	err := EnsureResourceReady(t, TestEnv, schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	}, rolloutCRName, rolloutTestNS, rolloutTestTimeout, isDeploymentReady)
	requireNoErrorWithDebugging(t, TestEnv, err,
		"Initial deployment should become ready", rolloutTestNS, rolloutCRName)

	err = WaitForPodsReady(t, TestEnv, rolloutTestNS, rolloutCRName, rolloutTestTimeout)
	requireNoErrorWithDebugging(t, TestEnv, err,
		"Initial pod should be running and ready", rolloutTestNS, rolloutCRName)
}

func testDeploymentStrategy(t *testing.T) {
	t.Helper()

	deployment := &appsv1.Deployment{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: rolloutTestNS, Name: rolloutCRName,
	}, deployment))

	assert.Equal(t, appsv1.RecreateDeploymentStrategyType,
		deployment.Spec.Strategy.Type,
		"Deployment with persistent storage should use Recreate strategy "+
			"to avoid RWO PVC multi-attach deadlock")
}

func testRolloutAfterEnvVarUpdate(t *testing.T) {
	t.Helper()

	initialPods, err := GetPodsForDeployment(TestEnv, TestEnv.Ctx, rolloutTestNS, rolloutCRName)
	require.NoError(t, err)
	require.Len(t, initialPods.Items, 1, "Should have exactly 1 pod before rollout")
	initialPodName := initialPods.Items[0].Name
	initialNodeName := initialPods.Items[0].Spec.NodeName
	t.Logf("Initial pod: %s on node: %s", initialPodName, initialNodeName)

	cordonNodeForTest(t, initialNodeName)

	t.Log("Updating OLLAMA_INFERENCE_MODEL to trigger rollout")
	updateServerEnvVar(t, rolloutTestNS, rolloutCRName, ollamaInferenceModelEnv, "llama3.2:3b")

	t.Log("Waiting for operator to update Deployment with new env var")
	waitForDeploymentEnvVar(t, rolloutTestNS, rolloutCRName, ollamaInferenceModelEnv, "llama3.2:3b")

	t.Log("Waiting for rollout to complete")
	err = EnsureResourceReady(t, TestEnv, schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	}, rolloutCRName, rolloutTestNS, rolloutTestTimeout, isDeploymentReady)
	requireNoErrorWithDebugging(t, TestEnv, err,
		"Deployment rollout should complete after env var update", rolloutTestNS, rolloutCRName)

	err = WaitForPodsReady(t, TestEnv, rolloutTestNS, rolloutCRName, rolloutTestTimeout)
	requireNoErrorWithDebugging(t, TestEnv, err,
		"New pod should be running and ready after rollout", rolloutTestNS, rolloutCRName)

	rolledPods, err := GetPodsForDeployment(TestEnv, TestEnv.Ctx, rolloutTestNS, rolloutCRName)
	require.NoError(t, err)
	require.Len(t, rolledPods.Items, 1, "Should have exactly 1 pod after rollout")

	newPodName := rolledPods.Items[0].Name
	t.Logf("New pod: %s", newPodName)
	assert.NotEqual(t, initialPodName, newPodName,
		"Pod name should change after rollout, confirming a new pod was created")

	deployment := &appsv1.Deployment{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Namespace: rolloutTestNS, Name: rolloutCRName,
	}, deployment))

	found := false
	for _, env := range deployment.Spec.Template.Spec.Containers[0].Env {
		if env.Name == ollamaInferenceModelEnv {
			assert.Equal(t, "llama3.2:3b", env.Value)
			found = true
			break
		}
	}
	require.True(t, found, "OLLAMA_INFERENCE_MODEL env var should be present in deployment")
}

func testNoFailedAttachVolumeEvents(t *testing.T) {
	t.Helper()

	eventList := &corev1.EventList{}
	require.NoError(t, TestEnv.Client.List(TestEnv.Ctx, eventList, client.InNamespace(rolloutTestNS)))

	for _, event := range eventList.Items {
		if event.Reason == "FailedAttachVolume" {
			t.Logf("Transient FailedAttachVolume (expected): %s", event.Message)
		}
	}
}

func testRolloutCleanup(t *testing.T) {
	t.Helper()

	server := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rolloutCRName,
			Namespace: rolloutTestNS,
		},
	}
	err := TestEnv.Client.Delete(TestEnv.Ctx, server)
	if err != nil && !k8serrors.IsNotFound(err) {
		require.NoError(t, err)
	}

	err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	}, rolloutCRName, rolloutTestNS, ResourceReadyTimeout)
	require.NoError(t, err, "Deployment should be deleted")
}

// cordonNodeForTest marks a node as unschedulable and registers cleanup.
func cordonNodeForTest(t *testing.T, nodeName string) {
	t.Helper()

	t.Logf("Cordoning node %s to force cross-node scheduling", nodeName)
	node := &corev1.Node{}
	require.NoError(t, TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
		Name: nodeName,
	}, node))
	node.Spec.Unschedulable = true
	require.NoError(t, TestEnv.Client.Update(TestEnv.Ctx, node))

	t.Cleanup(func() {
		t.Logf("Uncordoning node %s", nodeName)
		uncordonNode := &corev1.Node{}
		if getErr := TestEnv.Client.Get(TestEnv.Ctx, client.ObjectKey{
			Name: nodeName,
		}, uncordonNode); getErr == nil {
			uncordonNode.Spec.Unschedulable = false
			if updateErr := TestEnv.Client.Update(TestEnv.Ctx, uncordonNode); updateErr != nil {
				t.Logf("WARNING: Failed to uncordon node %s: %v", nodeName, updateErr)
			}
		}
	})
}

// updateServerEnvVar updates an env var on an OGXServer, retrying on conflict.
func updateServerEnvVar(t *testing.T, namespace, name, envName, newValue string) {
	t.Helper()

	err := wait.PollUntilContextTimeout(TestEnv.Ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		latest := &ogxiov1beta1.OGXServer{}
		if getErr := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: namespace, Name: name,
		}, latest); getErr != nil {
			return false, getErr
		}

		if latest.Spec.Workload == nil {
			latest.Spec.Workload = &ogxiov1beta1.WorkloadSpec{}
		}
		if latest.Spec.Workload.Overrides == nil {
			latest.Spec.Workload.Overrides = &ogxiov1beta1.WorkloadOverrides{}
		}

		for i, env := range latest.Spec.Workload.Overrides.Env {
			if env.Name == envName {
				latest.Spec.Workload.Overrides.Env[i].Value = newValue
				break
			}
		}

		if updateErr := TestEnv.Client.Update(ctx, latest); updateErr != nil {
			if k8serrors.IsConflict(updateErr) {
				return false, nil
			}
			return false, updateErr
		}
		return true, nil
	})
	require.NoError(t, err, "Failed to update CR env var "+envName)
}

// waitForDeploymentEnvVar polls until the Deployment reflects the expected env var value.
func waitForDeploymentEnvVar(t *testing.T, namespace, name, envName, expectedValue string) {
	t.Helper()

	err := wait.PollUntilContextTimeout(TestEnv.Ctx, pollInterval, rolloutTestTimeout, true, func(ctx context.Context) (bool, error) {
		dep := &appsv1.Deployment{}
		if getErr := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: namespace, Name: name,
		}, dep); getErr != nil {
			return false, getErr
		}
		for _, env := range dep.Spec.Template.Spec.Containers[0].Env {
			if env.Name == envName && env.Value == expectedValue {
				t.Log("Deployment updated with new env var")
				return true, nil
			}
		}
		return false, nil
	})
	requireNoErrorWithDebugging(t, TestEnv, err,
		"Operator should propagate env var change to Deployment", namespace, name)
}
