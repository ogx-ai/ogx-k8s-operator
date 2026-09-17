package controllers

import (
	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
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
	// ConditionTypeConfigGenerated indicates whether config generation succeeded.
	ConditionTypeConfigGenerated = "ConfigGenerated"
	// ConditionTypeStorageAdopted indicates whether legacy storage was adopted.
	ConditionTypeStorageAdopted = "StorageAdopted"
	// ConditionTypeNetworkingAdopted indicates whether legacy networking was adopted.
	ConditionTypeNetworkingAdopted = "NetworkingAdopted"
	// ConditionTypeAdoptionConfigInvalid indicates whether adoption annotation values are invalid.
	ConditionTypeAdoptionConfigInvalid = "AdoptionConfigInvalid"
	// ConditionTypePraxisReachable indicates whether the Praxis selector resolves to at least one
	// Ready Praxis pod (Praxis-fronted mode only).
	ConditionTypePraxisReachable = "PraxisReachable"
	// ConditionTypeTLSConfigured indicates whether spec.network.tls.secretName is set and the
	// referenced Secret exists (Praxis-fronted mode only).
	ConditionTypeTLSConfigured = "TLSConfigured"
	// ConditionTypeMigrationPreflightReady indicates migration preflight checks passed.
	ConditionTypeMigrationPreflightReady = "MigrationPreflightReady"
	// ConditionTypeMigrationJobSucceeded indicates the migration Job completed successfully.
	ConditionTypeMigrationJobSucceeded = "MigrationJobSucceeded"
	// ConditionTypeMigrationValidated indicates the migration Job completed.
	ConditionTypeMigrationValidated = "MigrationValidated"
	// ConditionTypePraxisCutoverReady indicates cutover may proceed after a successful migration Job.
	ConditionTypePraxisCutoverReady = "PraxisCutoverReady"
	// ConditionTypeSoftRollbackAvailable indicates only soft rollback is supported (with data-loss risk).
	ConditionTypeSoftRollbackAvailable = "SoftRollbackAvailable"
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
	// ReasonStorageAdopted indicates legacy storage was adopted.
	ReasonStorageAdopted = "StorageAdopted"
	// ReasonNetworkingAdopted indicates legacy networking was adopted.
	ReasonNetworkingAdopted = "NetworkingAdopted"
	// ReasonAdoptionConfigInvalid indicates adoption annotation values are invalid.
	ReasonAdoptionConfigInvalid = "AdoptionConfigInvalid"
	// ReasonPraxisReachable indicates the Praxis selector resolves to at least one Ready pod.
	ReasonPraxisReachable = "PraxisPodsReady"
	// ReasonPraxisUnreachable indicates no Ready Praxis pod matches the selector.
	ReasonPraxisUnreachable = "NoReadyPraxisPods"
	// ReasonPraxisSelectorInvalid indicates the Praxis pod selector could not be parsed.
	ReasonPraxisSelectorInvalid = "PraxisSelectorInvalid"
	// ReasonTLSConfigured indicates the referenced TLS Secret exists.
	ReasonTLSConfigured = "TLSSecretFound"
	// ReasonTLSSecretMissing indicates the referenced TLS Secret is set but not found.
	ReasonTLSSecretMissing = "TLSSecretMissing"
	// ReasonTLSNotConfigured indicates spec.network.tls.secretName is not set.
	ReasonTLSNotConfigured = "TLSSecretNotSet"
	// ReasonMigrationPreflightPassed indicates migration preflight succeeded.
	ReasonMigrationPreflightPassed = "MigrationPreflightPassed"
	// ReasonMigrationPreflightFailed indicates migration preflight failed.
	ReasonMigrationPreflightFailed = "MigrationPreflightFailed"
	// ReasonMigrationNotRequested indicates migration was not opted in.
	ReasonMigrationNotRequested = "MigrationNotRequested"
	// ReasonMigrationJobRunning indicates the migration Job is active.
	ReasonMigrationJobRunning = "MigrationJobRunning"
	// ReasonMigrationJobSucceeded indicates the migration Job succeeded.
	ReasonMigrationJobSucceeded = "MigrationJobSucceeded"
	// ReasonMigrationJobFailed indicates the migration Job failed.
	ReasonMigrationJobFailed = "MigrationJobFailed"
	// ReasonMigrationJobPending indicates the migration Job has not been observed yet.
	ReasonMigrationJobPending = "MigrationJobPending"
	// ReasonMigrationValidated indicates the migration Job completed.
	ReasonMigrationValidated = "MigrationValidated"
	// ReasonMigrationValidationPending indicates validation has not succeeded yet.
	ReasonMigrationValidationPending = "MigrationValidationPending"
	// ReasonMigrationValidationFailed indicates validation failed.
	ReasonMigrationValidationFailed = "MigrationValidationFailed"
	// ReasonPraxisCutoverReady indicates cutover is safe.
	ReasonPraxisCutoverReady = "PraxisCutoverReady"
	// ReasonPraxisCutoverNotReady indicates cutover is not safe yet.
	ReasonPraxisCutoverNotReady = "PraxisCutoverNotReady"
	// ReasonSoftRollbackOnly indicates only soft rollback is available.
	ReasonSoftRollbackOnly = "SoftRollbackOnly"
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
func SetDeploymentReadyCondition(status *ogxiov1beta1.OGXServerStatus, ready bool, message string) {
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
func SetHealthCheckCondition(status *ogxiov1beta1.OGXServerStatus, healthy bool, message string) {
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
func SetStorageReadyCondition(status *ogxiov1beta1.OGXServerStatus, ready bool, message string) {
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
func SetServiceReadyCondition(status *ogxiov1beta1.OGXServerStatus, ready bool, message string) {
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

// SetPraxisReachableCondition sets the Praxis reachability preflight condition.
func SetPraxisReachableCondition(status *ogxiov1beta1.OGXServerStatus, reachable bool, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypePraxisReachable,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !reachable {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SetTLSConfiguredCondition sets the TLS configuration preflight condition.
func SetTLSConfiguredCondition(status *ogxiov1beta1.OGXServerStatus, configured bool, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeTLSConfigured,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !configured {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SoftRollbackWarningMessage is the canonical soft-rollback data-loss warning.
const SoftRollbackWarningMessage = "Soft rollback only restores the pre-enablement OGX Responses/Conversations " +
	"serving posture. Responses/Conversations writes made through Praxis after cutover do not flow back to OGX, " +
	"ABAC flattening is not restored, and data loss is possible."

// SetMigrationPreflightReadyCondition sets the migration preflight condition.
func SetMigrationPreflightReadyCondition(status *ogxiov1beta1.OGXServerStatus, ready bool, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeMigrationPreflightReady,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !ready {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SetMigrationJobSucceededCondition sets the migration Job success condition.
func SetMigrationJobSucceededCondition(status *ogxiov1beta1.OGXServerStatus, succeeded bool, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeMigrationJobSucceeded,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !succeeded {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SetMigrationValidatedCondition sets the migration validation condition.
func SetMigrationValidatedCondition(status *ogxiov1beta1.OGXServerStatus, validated bool, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeMigrationValidated,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !validated {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SetPraxisCutoverReadyCondition sets the Praxis cutover readiness gate.
func SetPraxisCutoverReadyCondition(status *ogxiov1beta1.OGXServerStatus, ready bool, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypePraxisCutoverReady,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !ready {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SetSoftRollbackAvailableCondition sets the soft-rollback availability condition.
func SetSoftRollbackAvailableCondition(status *ogxiov1beta1.OGXServerStatus, available bool, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeSoftRollbackAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !available {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(status, condition)
}

// SetCondition sets a condition. Unchanged Status/Reason/Message are a no-op;
// LastTransitionTime is preserved when Status is unchanged.
func SetCondition(status *ogxiov1beta1.OGXServerStatus, condition metav1.Condition) {
	if status.Conditions == nil {
		status.Conditions = make([]metav1.Condition, 0)
	}

	for i := range status.Conditions {
		if status.Conditions[i].Type != condition.Type {
			continue
		}
		existing := status.Conditions[i]
		if existing.Status == condition.Status && existing.Reason == condition.Reason && existing.Message == condition.Message {
			return
		}
		if existing.Status == condition.Status {
			condition.LastTransitionTime = existing.LastTransitionTime
		}
		status.Conditions[i] = condition
		return
	}

	status.Conditions = append(status.Conditions, condition)
}

// GetCondition returns a condition by type.
func GetCondition(status *ogxiov1beta1.OGXServerStatus, conditionType string) *metav1.Condition {
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
func IsConditionTrue(status *ogxiov1beta1.OGXServerStatus, conditionType string) bool {
	condition := GetCondition(status, conditionType)
	return condition != nil && condition.Status == metav1.ConditionTrue
}

// IsConditionFalse returns true if the condition is false.
func IsConditionFalse(status *ogxiov1beta1.OGXServerStatus, conditionType string) bool {
	condition := GetCondition(status, conditionType)
	return condition != nil && condition.Status == metav1.ConditionFalse
}

// setConfigGeneratedCondition sets the ConfigGenerated condition.
func (r *OGXServerReconciler) setConfigGeneratedCondition(instance *ogxiov1beta1.OGXServer, success bool, reason, message string) {
	condition := metav1.Condition{
		Type:               ConditionTypeConfigGenerated,
		Status:             metav1.ConditionTrue,
		Reason:             reason,
		Message:            message,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	if !success {
		condition.Status = metav1.ConditionFalse
	}
	SetCondition(&instance.Status, condition)
}
