package deploy

import (
	"fmt"
	"os"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
)

func GetOperatorNamespace() (string, error) {
	operatorNS, exist := os.LookupEnv("OPERATOR_NAMESPACE")
	if exist && operatorNS != "" {
		return operatorNS, nil
	}
	data, err := os.ReadFile("/var/run/secrets/kubernetes.io/serviceaccount/namespace")
	return string(data), err
}

func GetServicePort(instance *ogxiov1beta1.OGXServer) int32 {
	if instance.Spec.Network != nil && instance.Spec.Network.Port != 0 {
		return instance.Spec.Network.Port
	}
	return ogxiov1beta1.DefaultServerPort
}

func GetServiceName(instance *ogxiov1beta1.OGXServer) string {
	return fmt.Sprintf("%s-service", instance.Name)
}

const defaultMetricsPort = int32(9464)

func getEffectiveMetricsPort(instance *ogxiov1beta1.OGXServer) int32 {
	monitoring := instance.Spec.Monitoring
	if monitoring != nil && monitoring.Enabled != nil && !*monitoring.Enabled {
		return 0
	}
	if monitoring != nil && monitoring.MetricsPort != nil {
		return *monitoring.MetricsPort
	}
	return defaultMetricsPort
}

// GetEffectiveReplicas returns the desired replica count, defaulting to 1.
func GetEffectiveReplicas(instance *ogxiov1beta1.OGXServer) int32 {
	if instance.Spec.Workload != nil && instance.Spec.Workload.Replicas != nil {
		return *instance.Spec.Workload.Replicas
	}
	return 1
}
