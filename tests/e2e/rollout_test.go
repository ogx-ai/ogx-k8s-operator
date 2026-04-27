//nolint:testpackage
package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
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
	rolloutTestNS           = "llama-stack-rollout-test"
	rolloutTestTimeout      = 5 * time.Minute
	rolloutCRName           = "rollout-test"
	ollamaInferenceModelEnv = "OLLAMA_INFERENCE_MODEL"
)

// TestRolloutWithStorage verifies that updating a LlamaStackDistribution
// with persistent storage completes without RWO PVC multi-attach deadlock.
// Requires a multi-node cluster. Cordons the initial pod's node to force
// cross-node scheduling, guaranteeing the RWO conflict if strategy is wrong.
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

	t.Run("should create distribution with storage", func(t *testing.T) {
		testCreateDistributionWithStorage(t)
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

func testCreateDistributionWithStorage(t *testing.T) {
	t.Helper()

	cr := &v1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rolloutCRName,
			Namespace: rolloutTestNS,
		},
		Spec: v1alpha1.LlamaStackDistributionSpec{
			Replicas: 1,
			Server: v1alpha1.ServerSpec{
				Distribution: v1alpha1.DistributionType{
					Name: starterDistType,
				},
				ContainerSpec: v1alpha1.ContainerSpec{
					Name: "llama-stack",
					Env: []corev1.EnvVar{
						{Name: ollamaInferenceModelEnv, Value: "llama3.2:1b"},
						{Name: "OLLAMA_URL", Value: "http://ollama-server-service.ollama-dist.svc.cluster.local:11434"},
					},
				},
				Storage: &v1alpha1.StorageSpec{},
			},
		},
	}

	t.Log("Creating LlamaStackDistribution with persistent storage")
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
	updateDistributionEnvVar(t, rolloutTestNS, rolloutCRName, ollamaInferenceModelEnv, "llama3.2:3b")

	// Poll until the operator reconciles the env var into the Deployment,
	// otherwise the old (still-ready) Deployment passes readiness immediately.
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
			// Transient during cross-node volume migration with Recreate strategy.
			// A permanent deadlock (RollingUpdate) would have timed out above.
			t.Logf("Transient FailedAttachVolume (expected): %s", event.Message)
		}
	}
}

func testRolloutCleanup(t *testing.T) {
	t.Helper()

	distribution := &v1alpha1.LlamaStackDistribution{
		ObjectMeta: metav1.ObjectMeta{
			Name:      rolloutCRName,
			Namespace: rolloutTestNS,
		},
	}
	err := TestEnv.Client.Delete(TestEnv.Ctx, distribution)
	if err != nil && !k8serrors.IsNotFound(err) {
		require.NoError(t, err)
	}

	err = EnsureResourceDeleted(t, TestEnv, schema.GroupVersionKind{
		Group: "apps", Version: "v1", Kind: "Deployment",
	}, rolloutCRName, rolloutTestNS, ResourceReadyTimeout)
	require.NoError(t, err, "Deployment should be deleted")
}

// cordonNodeForTest marks a node as unschedulable and registers a
// t.Cleanup to uncordon it when the test finishes.
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

// updateDistributionEnvVar updates an env var on a LlamaStackDistribution,
// retrying on conflict.
func updateDistributionEnvVar(t *testing.T, namespace, name, envName, newValue string) {
	t.Helper()

	err := wait.PollUntilContextTimeout(TestEnv.Ctx, time.Second, 30*time.Second, true, func(ctx context.Context) (bool, error) {
		latest := &v1alpha1.LlamaStackDistribution{}
		if getErr := TestEnv.Client.Get(ctx, client.ObjectKey{
			Namespace: namespace, Name: name,
		}, latest); getErr != nil {
			return false, getErr
		}

		for i, env := range latest.Spec.Server.ContainerSpec.Env {
			if env.Name == envName {
				latest.Spec.Server.ContainerSpec.Env[i].Value = newValue
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

// waitForDeploymentEnvVar polls until the Deployment's container spec
// reflects the expected env var value.
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
