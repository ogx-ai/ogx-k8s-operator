package deploy

import (
	"errors"
	"fmt"
	"os"
	"strconv"

	llamav1alpha1 "github.com/llamastack/llama-stack-k8s-operator/api/v1alpha1"
)

const defaultAPIServerPort int32 = 443

func GetOperatorNamespace() (string, error) {
	operatorNS, exist := os.LookupEnv("OPERATOR_NAMESPACE")
	if exist && operatorNS != "" {
		return operatorNS, nil
	}
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	return string(data), err
}

func GetServicePort(instance *llamav1alpha1.LlamaStackDistribution) int32 {
	// Use the container's port (defaulted to 8321 if unset)
	port := instance.Spec.Server.ContainerSpec.Port
	if port == 0 {
		port = llamav1alpha1.DefaultServerPort
	}
	return port
}

func GetServiceName(instance *llamav1alpha1.LlamaStackDistribution) string {
	return fmt.Sprintf("%s-service", instance.Name)
}

// GetAPIServerEndpoint returns the Kubernetes API server host and port.
// These are injected into every pod by the kubelet automatically.
func GetAPIServerEndpoint() (string, int32, error) {
	host := os.Getenv("KUBERNETES_SERVICE_HOST")
	if host == "" {
		return "", 0, errors.New("failed to get API server endpoint: KUBERNETES_SERVICE_HOST not set")
	}

	portStr := os.Getenv("KUBERNETES_SERVICE_PORT")
	if portStr == "" {
		return host, defaultAPIServerPort, nil
	}

	port, err := strconv.ParseInt(portStr, 10, 32)
	if err != nil {
		return "", 0, fmt.Errorf("failed to parse KUBERNETES_SERVICE_PORT %q: %w", portStr, err)
	}
	return host, int32(port), nil
}
