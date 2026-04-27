package deploy

import (
	"fmt"
	"os"

	llamav1alpha1 "github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
)

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
