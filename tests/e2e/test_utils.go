//nolint:testpackage
package e2e

import (
	"context"
	"fmt"
	"log"
	"os"
	"path/filepath"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/require"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	discoveryv1 "k8s.io/api/discovery/v1"
	apiextv1 "k8s.io/apiextensions-apiserver/pkg/apis/apiextensions/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	"k8s.io/apimachinery/pkg/util/wait"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
	"sigs.k8s.io/yaml"
)

const (
	starterDistType      = "starter"
	ollamaNS             = "ollama-dist"
	pollInterval         = 10 * time.Second
	ResourceReadyTimeout = 5 * time.Minute
	generalRetryInterval = 5 * time.Second
)

var (
	Scheme = runtime.NewScheme()
)

// TestEnvironment holds the test environment configuration.
type TestEnvironment struct {
	Client client.Client
	Ctx    context.Context //nolint:containedctx // Context is used for test environment
}

// SetupTestEnv sets up the test environment.
func SetupTestEnv() (*TestEnvironment, error) {
	cfg, err := config.GetConfig()
	if err != nil {
		return nil, err
	}

	cl, err := client.New(cfg, client.Options{Scheme: Scheme})
	if err != nil {
		return nil, err
	}

	return &TestEnvironment{
		Client: cl,
		Ctx:    context.TODO(),
	}, nil
}

// validateCRD checks if a CustomResourceDefinition is established.
func validateCRD(c client.Client, ctx context.Context, crdName string) error {
	crd := &apiextv1.CustomResourceDefinition{}
	obj := client.ObjectKey{
		Name: crdName,
	}

	err := wait.PollUntilContextTimeout(ctx, generalRetryInterval, ResourceReadyTimeout, true, func(ctx context.Context) (bool, error) {
		err := c.Get(ctx, obj, crd)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			log.Printf("Failed to get CRD %s", crdName)
			return false, err
		}

		for _, condition := range crd.Status.Conditions {
			if condition.Type == apiextv1.Established {
				if condition.Status == apiextv1.ConditionTrue {
					return true, nil
				}
			}
		}
		log.Printf("Error to get CRD %s condition's matching", crdName)
		return false, nil
	})

	return err
}

// GetDeployment gets a deployment by name and namespace.
func GetDeployment(cl client.Client, ctx context.Context, name, namespace string) (*appsv1.Deployment, error) {
	deployment := &appsv1.Deployment{}
	err := cl.Get(ctx, types.NamespacedName{Namespace: namespace, Name: name}, deployment)
	return deployment, err
}

// EnsureResourceReady polls until the resource is ready.
func EnsureResourceReady(
	t *testing.T,
	testenv *TestEnvironment,
	gvk schema.GroupVersionKind,
	name, namespace string,
	timeout time.Duration,
	isReady func(*unstructured.Unstructured) bool,
) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(testenv.Ctx, timeout)
	defer cancel()
	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk)
		err := testenv.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
		if err != nil {
			if errors.IsNotFound(err) {
				return false, nil
			}
			return false, err
		}
		return isReady(obj), nil
	})
}

// EnsureResourceDeleted polls until the resource is deleted.
func EnsureResourceDeleted(t *testing.T, testenv *TestEnvironment, gvk schema.GroupVersionKind, name, namespace string, timeout time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(testenv.Ctx, timeout)
	defer cancel()
	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		obj := &unstructured.Unstructured{}
		obj.SetGroupVersionKind(gvk)
		err := testenv.Client.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, obj)
		if errors.IsNotFound(err) {
			return true, nil
		}
		return false, nil
	})
}

// WaitForPodsReady polls until all pods for a deployment are running and ready.
func WaitForPodsReady(t *testing.T, testenv *TestEnvironment, namespace, deploymentName string, timeout time.Duration) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(testenv.Ctx, timeout)
	defer cancel()

	return wait.PollUntilContextTimeout(ctx, pollInterval, timeout, true, func(ctx context.Context) (bool, error) {
		podList, err := GetPodsForDeployment(testenv, ctx, namespace, deploymentName)
		if err != nil {
			t.Logf("Error listing pods: %v", err)
			return false, err
		}

		if len(podList.Items) == 0 {
			t.Logf("No pods found for deployment %s yet", deploymentName)
			return false, nil
		}

		for _, pod := range podList.Items {
			ready, err := checkPodStatus(t, &pod)
			if err != nil {
				return false, err
			}
			if !ready {
				return false, nil
			}
		}

		t.Logf("All pods for deployment %s are ready", deploymentName)
		return true, nil
	})
}

// checkPodStatus checks if a single pod is running and ready.
func checkPodStatus(t *testing.T, pod *corev1.Pod) (bool, error) {
	t.Helper()
	t.Logf("Pod %s status: Phase=%s, Ready=%v", pod.Name, pod.Status.Phase, isPodReady(pod))

	if pod.Status.Phase != corev1.PodRunning && pod.Status.Phase != corev1.PodSucceeded {
		t.Logf("Pod %s not running yet (phase: %s)", pod.Name, pod.Status.Phase)
		return false, nil
	}

	if !isPodReady(pod) {
		t.Logf("Pod %s not ready yet", pod.Name)
		return false, nil
	}

	return checkContainerStatuses(t, pod)
}

// checkContainerStatuses checks all container statuses in a pod for errors.
func checkContainerStatuses(t *testing.T, pod *corev1.Pod) (bool, error) {
	t.Helper()
	for _, containerStatus := range pod.Status.ContainerStatuses {
		if containerStatus.State.Waiting != nil {
			t.Logf("Container %s in pod %s is waiting: %s - %s",
				containerStatus.Name, pod.Name,
				containerStatus.State.Waiting.Reason,
				containerStatus.State.Waiting.Message)

			if containerStatus.State.Waiting.Reason == "ImagePullBackOff" ||
				containerStatus.State.Waiting.Reason == "ErrImagePull" ||
				containerStatus.State.Waiting.Reason == "CrashLoopBackOff" {
				return false, fmt.Errorf("failed to start container %s: %s - %s",
					containerStatus.Name,
					containerStatus.State.Waiting.Reason,
					containerStatus.State.Waiting.Message)
			}
		}

		if containerStatus.State.Terminated != nil && containerStatus.State.Terminated.ExitCode != 0 {
			return false, fmt.Errorf("failed to run container %s: terminated with exit code %d: %s",
				containerStatus.Name,
				containerStatus.State.Terminated.ExitCode,
				containerStatus.State.Terminated.Reason)
		}
	}
	return true, nil
}

// isPodReady checks if a pod has the Ready condition set to True.
func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady {
			return condition.Status == corev1.ConditionTrue
		}
	}
	return false
}

// GetPodsForDeployment retrieves the list of pods for a given deployment.
func GetPodsForDeployment(testenv *TestEnvironment, ctx context.Context, namespace, deploymentName string) (*corev1.PodList, error) {
	podList := &corev1.PodList{}
	err := testenv.Client.List(ctx, podList, client.InNamespace(namespace), client.MatchingLabels{
		"app.kubernetes.io/instance": deploymentName,
	})
	if err != nil {
		return nil, err
	}
	return podList, nil
}

// CleanupTestEnv cleans up the test environment.
func CleanupTestEnv(env *TestEnvironment) {
	// Implementation will be added later
}

// registerSchemes registers all necessary schemes for testing.
func registerSchemes() {
	schemes := []func(*runtime.Scheme) error{
		clientgoscheme.AddToScheme,
		apiextv1.AddToScheme,
		ogxiov1beta1.AddToScheme,
	}

	for _, schemeFn := range schemes {
		utilruntime.Must(schemeFn(Scheme))
	}
}

// EnsureOverrideConfigMap creates the ConfigMap referenced by the CR's overrideConfig
// if one is configured. The ConfigMap must exist before the CR is created.
func EnsureOverrideConfigMap(t *testing.T, c client.Client, ctx context.Context, server *ogxiov1beta1.OGXServer) {
	t.Helper()

	if server.Spec.OverrideConfig == nil || server.Spec.OverrideConfig.Name == "" {
		return
	}

	projectRoot, err := filepath.Abs("../..")
	require.NoError(t, err)

	configMapPath := filepath.Join(projectRoot, "config", "samples", "starter-config-configmap.yaml")
	yamlFile, err := os.ReadFile(configMapPath)
	require.NoError(t, err)

	cm := &corev1.ConfigMap{}
	err = yaml.Unmarshal(yamlFile, cm)
	require.NoError(t, err)

	cm.Namespace = server.Namespace
	err = c.Create(ctx, cm)
	if err != nil && !errors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}
}

// GetSampleCRForDistribution returns an OGXServer configured for the specified distribution type.
func GetSampleCRForDistribution(t *testing.T, distType string) *ogxiov1beta1.OGXServer {
	t.Helper()
	projectRoot, err := filepath.Abs("../..")
	require.NoError(t, err)

	samplePath := filepath.Join(projectRoot, "config", "samples", "_v1beta1_ogxserver.yaml")

	yamlFile, err := os.ReadFile(samplePath)
	require.NoError(t, err)

	server := &ogxiov1beta1.OGXServer{}
	err = yaml.Unmarshal(yamlFile, server)
	require.NoError(t, err)

	switch distType {
	case starterDistType:
		server.Spec.Distribution.Name = starterDistType
		server.ObjectMeta.Name = "ogxserver-" + starterDistType + "-sample"
	default:
		t.Fatalf("Unknown distribution type: %s", distType)
	}

	if server.Spec.Workload != nil {
		server.Spec.Workload.Autoscaling = nil
		server.Spec.Workload.Storage = nil
		server.Spec.Workload.PodDisruptionBudget = nil
		server.Spec.Workload.TopologySpreadConstraints = nil
	}

	return server
}

// checkOGXServerStatus helps identify if the custom resource reached the expected state during test execution.
func checkOGXServerStatus(t *testing.T, testenv *TestEnvironment, namespace, name string) {
	t.Helper()

	ogxServer := &ogxiov1beta1.OGXServer{}
	err := testenv.Client.Get(testenv.Ctx, client.ObjectKey{Namespace: namespace, Name: name}, ogxServer)
	if err != nil {
		t.Logf("Error getting OGXServer: %v", err)
		return
	}

	t.Logf("OGXServer status:")
	t.Logf("  Phase: %s", ogxServer.Status.Phase)
	t.Logf("  Generation: %d", ogxServer.Generation)
	t.Logf("  ResourceVersion: %s", ogxServer.ResourceVersion)
	t.Logf("  Conditions: %+v", ogxServer.Status.Conditions)
}

// checkNamespaceEvents reveals what Kubernetes operations occurred and why they may have failed.
func checkNamespaceEvents(t *testing.T, testenv *TestEnvironment, namespace string) {
	t.Helper()

	eventList := &corev1.EventList{}
	err := testenv.Client.List(testenv.Ctx, eventList, client.InNamespace(namespace))
	if err != nil {
		t.Logf("Error getting events: %v", err)
		return
	}

	if len(eventList.Items) == 0 {
		t.Log("No events found in namespace")
		return
	}

	maxEvents := 25
	if len(eventList.Items) > maxEvents {
		t.Logf("Showing first %d events (of %d total):", maxEvents, len(eventList.Items))
		eventList.Items = eventList.Items[:maxEvents]
	} else {
		t.Logf("Found %d events in namespace %s:", len(eventList.Items), namespace)
	}

	for _, event := range eventList.Items {
		t.Logf("  %s: %s (%s) - %s",
			event.LastTimestamp.Format("15:04:05"),
			event.Reason,
			event.Type,
			event.Message)
	}
}

// requireNoErrorWithDebugging provides comprehensive debugging context when tests fail.
func requireNoErrorWithDebugging(t *testing.T, testenv *TestEnvironment, err error, msg string, namespace, crName string) {
	t.Helper()
	if err != nil {
		t.Logf("ERROR OCCURRED: %s - %v", msg, err)

		checkOGXServerStatus(t, testenv, namespace, crName)
		checkNamespaceEvents(t, testenv, namespace)
		logPodDetails(t, testenv, namespace)
		logServiceEndpoints(t, testenv, namespace, crName+"-service")
		logServiceSpec(t, testenv, namespace, crName+"-service")
		logDeploymentSpec(t, testenv, namespace, crName)

		require.NoError(t, err, msg)
	}
}

// logPodDetails helps diagnose pod startup issues and container restart problems during test failures.
func logPodDetails(t *testing.T, testenv *TestEnvironment, namespace string) {
	t.Helper()

	podList := &corev1.PodList{}
	err := testenv.Client.List(testenv.Ctx, podList, client.InNamespace(namespace))
	if err != nil {
		t.Logf("Failed to list pods: %v", err)
		return
	}

	t.Logf("Found %d pods in namespace %s:", len(podList.Items), namespace)
	for _, pod := range podList.Items {
		t.Logf("Pod: %s, Phase: %s", pod.Name, pod.Status.Phase)

		for _, cs := range pod.Status.ContainerStatuses {
			t.Logf("  Container %s: Ready=%v, RestartCount=%d",
				cs.Name, cs.Ready, cs.RestartCount)

			if cs.State.Waiting != nil {
				t.Logf("    Waiting: %s - %s",
					cs.State.Waiting.Reason, cs.State.Waiting.Message)
			}
			if cs.State.Terminated != nil {
				t.Logf("    Terminated: %s - %s",
					cs.State.Terminated.Reason, cs.State.Terminated.Message)
			}
		}

		t.Logf("  (Pod logs require direct kubectl access)")
	}
}

// logServiceEndpoints logs service endpoint details to see if pods are ready.
func logServiceEndpoints(t *testing.T, testenv *TestEnvironment, namespace, serviceName string) {
	t.Helper()

	endpointSliceList := &discoveryv1.EndpointSliceList{}
	err := testenv.Client.List(testenv.Ctx, endpointSliceList,
		client.InNamespace(namespace),
		client.MatchingLabels{"kubernetes.io/service-name": serviceName})

	if err != nil {
		t.Logf("Failed to get endpoint slices for service %s: %v", serviceName, err)
		return
	}

	if len(endpointSliceList.Items) == 0 {
		t.Logf("Service %s has no endpoint slices", serviceName)
		return
	}

	t.Logf("Service %s endpoints:", serviceName)
	for i, slice := range endpointSliceList.Items {
		t.Logf("  EndpointSlice %d (%s):", i, slice.Name)
		logEndpointSliceDetails(t, &slice)
	}
}

// logEndpointSliceDetails logs the details of a single endpoint slice.
func logEndpointSliceDetails(t *testing.T, slice *discoveryv1.EndpointSlice) {
	t.Helper()

	readyCount, notReadyCount := logEndpointAddresses(t, slice.Endpoints)
	t.Logf("    Summary: %d ready, %d not ready", readyCount, notReadyCount)

	t.Logf("    Ports:")
	logPortDetails(t, slice.Ports)
}

// logEndpointAddresses logs endpoint addresses and returns counts of ready and not-ready endpoints.
func logEndpointAddresses(t *testing.T, endpoints []discoveryv1.Endpoint) (readyCount, notReadyCount int) {
	t.Helper()

	for _, endpoint := range endpoints {
		isReady := endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready
		if isReady {
			readyCount++
			for _, addr := range endpoint.Addresses {
				t.Logf("    Ready: %s", addr)
			}
		} else {
			notReadyCount++
			for _, addr := range endpoint.Addresses {
				t.Logf("    Not ready: %s", addr)
			}
		}
	}
	return readyCount, notReadyCount
}

// logPortDetails logs port information from an endpoint slice.
func logPortDetails(t *testing.T, ports []discoveryv1.EndpointPort) {
	t.Helper()

	for _, port := range ports {
		portNum := int32(0)
		if port.Port != nil {
			portNum = *port.Port
		}
		portName := ""
		if port.Name != nil {
			portName = *port.Name
		}
		t.Logf("      - %s: %d", portName, portNum)
	}
}

// logDeploymentSpec helps identify configuration mismatches that prevent pods from starting correctly.
func logDeploymentSpec(t *testing.T, testenv *TestEnvironment, namespace, name string) {
	t.Helper()

	deployment := &appsv1.Deployment{}
	err := testenv.Client.Get(testenv.Ctx, types.NamespacedName{
		Name:      name,
		Namespace: namespace,
	}, deployment)

	if err != nil {
		t.Logf("Failed to get deployment: %v", err)
		return
	}

	t.Logf("Deployment %s spec:", name)
	t.Logf("  Replicas: %d", *deployment.Spec.Replicas)
	t.Logf("  Selector: %+v", deployment.Spec.Selector.MatchLabels)
	t.Logf("  Template labels: %+v", deployment.Spec.Template.Labels)

	for _, container := range deployment.Spec.Template.Spec.Containers {
		t.Logf("  Container: %s", container.Name)
		t.Logf("    Image: %s", container.Image)
		t.Logf("    Ports:")
		for _, port := range container.Ports {
			t.Logf("      - %d", port.ContainerPort)
		}
		t.Logf("    Env vars:")
		for _, env := range container.Env {
			t.Logf("      %s=%s", env.Name, env.Value)
		}
		if container.ReadinessProbe != nil {
			t.Logf("    Readiness probe: %+v", container.ReadinessProbe)
		}
	}
}

// logOperatorPodStatus logs operator pod details to diagnose webhook failures
// caused by pod restarts, OOMKills, or CrashLoopBackOff.
func logOperatorPodStatus(t *testing.T, testenv *TestEnvironment, operatorNS string) {
	t.Helper()

	podList := &corev1.PodList{}
	err := testenv.Client.List(testenv.Ctx, podList,
		client.InNamespace(operatorNS),
		client.MatchingLabels{"control-plane": "controller-manager"})
	if err != nil {
		t.Logf("Failed to list operator pods: %v", err)
		return
	}

	for _, pod := range podList.Items {
		t.Logf("Operator pod %s: Phase=%s", pod.Name, pod.Status.Phase)
		for _, cs := range pod.Status.ContainerStatuses {
			t.Logf("  Container %s: Ready=%v, RestartCount=%d", cs.Name, cs.Ready, cs.RestartCount)
			if cs.LastTerminationState.Terminated != nil {
				t.Logf("  Last termination: Reason=%s, ExitCode=%d, At=%s",
					cs.LastTerminationState.Terminated.Reason,
					cs.LastTerminationState.Terminated.ExitCode,
					cs.LastTerminationState.Terminated.FinishedAt.String())
			}
		}
	}
}

// WaitForWebhookReady polls until the webhook service endpoint has at least one ready address.
// This prevents "connection refused" errors when creating OGXServer resources before
// the operator's webhook server is accepting connections.
func WaitForWebhookReady(t *testing.T, testenv *TestEnvironment, operatorNS string, timeout time.Duration) error {
	t.Helper()
	t.Log("Waiting for webhook endpoint to become ready...")

	webhookServiceName := "ogx-k8s-operator-webhook-service"

	return wait.PollUntilContextTimeout(testenv.Ctx, generalRetryInterval, timeout, true, func(ctx context.Context) (bool, error) {
		endpointSliceList := &discoveryv1.EndpointSliceList{}
		err := testenv.Client.List(ctx, endpointSliceList,
			client.InNamespace(operatorNS),
			client.MatchingLabels{"kubernetes.io/service-name": webhookServiceName})
		if err != nil {
			t.Logf("Failed to list endpoint slices for webhook service: %v", err)
			return false, nil
		}

		if len(endpointSliceList.Items) == 0 {
			t.Log("No endpoint slices found for webhook service yet")
			return false, nil
		}

		for _, slice := range endpointSliceList.Items {
			for _, endpoint := range slice.Endpoints {
				if endpoint.Conditions.Ready != nil && *endpoint.Conditions.Ready {
					t.Log("Webhook endpoint is ready")
					return true, nil
				}
			}
		}

		t.Log("Webhook endpoint exists but no ready addresses yet")
		return false, nil
	})
}

// logServiceSpec logs the actual service configuration to debug selector issues.
func logServiceSpec(t *testing.T, testenv *TestEnvironment, namespace, serviceName string) {
	t.Helper()

	service := &corev1.Service{}
	err := testenv.Client.Get(testenv.Ctx, types.NamespacedName{
		Name:      serviceName,
		Namespace: namespace,
	}, service)

	if err != nil {
		t.Logf("Failed to get service %s: %v", serviceName, err)
		return
	}

	t.Logf("Service %s spec:", serviceName)
	t.Logf("  Type: %s", service.Spec.Type)
	t.Logf("  Selector: %+v", service.Spec.Selector)
	t.Logf("  Ports:")
	for _, port := range service.Spec.Ports {
		t.Logf("    - %s: %d -> %s", port.Name, port.Port, port.TargetPort.String())
	}
}
