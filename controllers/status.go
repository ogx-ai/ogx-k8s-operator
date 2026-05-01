package controllers

import (
	llamav1alpha1 "github.com/ogx-ai/ogx-k8s-operator/api/v1alpha1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Condition types.
const (
	// ConditionTypeDeploymentReady indicates whether the deployment is ready.
	ConditionTypeDeploymentReady = "DeploymentReady"
	// ConditionTypeHealthCheck indicates whether the health check passed.
	ConditionTypeHealthCheck = "HealthCheck"
	// ConditionTypeStorageReady indicates whether the storage is ready.
	ConditionTypeStorageReady = "StorageReady"
	// ConditionTypeServiceReady indicates whether the service is ready.
	ConditionTypeServiceReady = "ServiceReady"
)

// Condition reasons.
const (
	// ReasonDeploymentReady indicates the deployment is ready.
	ReasonDeploymentReady = "DeploymentReady"
	// ReasonDeploymentFailed indicates the deployment failed.
	ReasonDeploymentFailed = "DeploymentFailed"
	// ReasonDeploymentPending indicates the deployment is pending.
	ReasonDeploymentPending = "DeploymentPending"
	// ReasonHealthCheckPassed indicates the health check passed.
	ReasonHealthCheckPassed = "HealthCheckPassed"
	// ReasonHealthCheckFailed indicates the health check failed.
	ReasonHealthCheckFailed = "HealthCheckFailed"
	// ReasonStorageReady indicates the storage is ready.
	ReasonStorageReady = "StorageReady"
	// ReasonStorageFailed indicates the storage failed.
	ReasonStorageFailed = "StorageFailed"
	// ReasonServiceReady indicates the service is ready.
	ReasonServiceReady = "ServiceReady"
	// ReasonServiceFailed indicates the service failed.
	ReasonServiceFailed = "ServiceFailed"
)

// Condition messages.
const (
	// MessageDeploymentReady indicates the deployment is ready.
	MessageDeploymentReady = "Deployment is ready"
	// MessageDeploymentFailed indicates the deployment failed.
	MessageDeploymentFailed = "Deployment failed"
	// MessageDeploymentPending indicates the deployment is pending.
	MessageDeploymentPending = "Deployment is pending"
	// MessageHealthCheckPassed indicates the health check passed.
	MessageHealthCheckPassed = "Health check passed"
	// MessageHealthCheckFailed indicates the health check failed.
	MessageHealthCheckFailed = "Health check failed"
	// MessageStorageReady indicates the storage is ready.
	MessageStorageReady = "Storage is ready"
	// MessageStorageFailed indicates the storage failed.
	MessageStorageFailed = "Storage failed"
	// MessageServiceReady indicates the service is ready.
	MessageServiceReady = "Service is ready"
	// MessageServiceFailed indicates the service failed.
	MessageServiceFailed = "Service failed"
)

// SetDeploymentReadyCondition sets the deployment ready condition.
func SetDeploymentReadyCondition(status *llamav1alpha1.LlamaStackDistributionStatus, ready bool, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeDeploymentReady,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonDeploymentReady,
		Message:            MessageDeploymentReady,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}

	if !ready {
		condition.Status = metav1.ConditionFalse
		condition.Reason = ReasonDeploymentFailed
		condition.Message = message
	}

	SetCondition(status, condition)
}

// SetHealthCheckCondition sets the health check condition.
func SetHealthCheckCondition(status *llamav1alpha1.LlamaStackDistributionStatus, healthy bool, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeHealthCheck,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonHealthCheckPassed,
		Message:            MessageHealthCheckPassed,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}

	if !healthy {
		condition.Status = metav1.ConditionFalse
		condition.Reason = ReasonHealthCheckFailed
		condition.Message = message
	}

	SetCondition(status, condition)
}

// SetStorageReadyCondition sets the storage ready condition.
func SetStorageReadyCondition(status *llamav1alpha1.LlamaStackDistributionStatus, ready bool, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeStorageReady,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonStorageReady,
		Message:            MessageStorageReady,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}

	if !ready {
		condition.Status = metav1.ConditionFalse
		condition.Reason = ReasonStorageFailed
		condition.Message = message
	}

	SetCondition(status, condition)
}

// SetServiceReadyCondition sets the service ready condition.
func SetServiceReadyCondition(status *llamav1alpha1.LlamaStackDistributionStatus, ready bool, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeServiceReady,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonServiceReady,
		Message:            MessageServiceReady,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}

	if !ready {
		condition.Status = metav1.ConditionFalse
		condition.Reason = ReasonServiceFailed
		condition.Message = message
	}

	SetCondition(status, condition)
}

// SetCondition sets a condition in the status.
func SetCondition(status *llamav1alpha1.LlamaStackDistributionStatus, condition metav1.Condition) {
	// Initialize conditions if needed
	if status.Conditions == nil {
		status.Conditions = make([]metav1.Condition, 0)
	}

	// Find existing condition
	for i := range status.Conditions {
		if status.Conditions[i].Type == condition.Type {
			// Update existing condition
			status.Conditions[i] = condition
			return
		}
	}

	// Add new condition
	status.Conditions = append(status.Conditions, condition)
}

// GetCondition returns a condition by type.
func GetCondition(status *llamav1alpha1.LlamaStackDistributionStatus, conditionType string) *metav1.Condition {
	if status == nil || status.Conditions == nil {
		return nil
	}
	for i := range status.Conditions {
		if status.Conditions[i].Type == conditionType {
			return &status.Conditions[i]
		}
	}
	return nil
}

// IsConditionTrue returns true if the condition is true.
func IsConditionTrue(status *llamav1alpha1.LlamaStackDistributionStatus, conditionType string) bool {
	condition := GetCondition(status, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// IsConditionFalse returns true if the condition is false.
func IsConditionFalse(status *llamav1alpha1.LlamaStackDistributionStatus, conditionType string) bool {
	condition := GetCondition(status, conditionType)
	return condition != nil && condition.Status == metav1.ConditionFalse
}
