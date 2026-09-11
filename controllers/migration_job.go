/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controllers

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/config"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

const (
	migrationJobNameSuffix       = "-praxis-migration"
	migrationAttemptAnnotation   = "ogx.io/migration-attempt"
	migrationManagedLabelKey     = managedByLabelKey
	migrationManagedLabelValue   = managedByLabelVal
	migrationComponentLabelKey   = "app.kubernetes.io/component"
	migrationComponentLabelValue = "praxis-migration"
	migrationContainerName       = "ogx-praxis-migration"
	praxisDatabaseURLEnv         = "PRAXIS_DATABASE_URL"
	migrationActiveDeadlineSecs  = int64(3600)
	migrationJobBackoffLimit     = int32(2)
	// dash as /bin/sh rejects pipefail.
	migrationJobShell = "set -eu; " +
		"ogx migrate praxis --dry-run \"$OGX_CONFIG\" && " +
		"ogx migrate praxis \"$OGX_CONFIG\""
)

func (r *OGXServerReconciler) reconcileMigration(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
) error {
	if !r.isMigrationRequested(instance) {
		return r.handleMigrationNotRequested(ctx, instance)
	}

	r.ensureSoftRollbackWarning(instance)

	if preflightErr := r.runMigrationPreflight(ctx, instance, runtimeConfig); preflightErr != nil {
		r.applyPreflightFailure(instance, preflightErr)
		r.emitMigrationEvent(instance, corev1.EventTypeWarning, ReasonMigrationPreflightFailed, preflightErr.Error())
		return nil
	}
	SetMigrationPreflightReadyCondition(&instance.Status, true, ReasonMigrationPreflightPassed,
		"Migration preflight checks passed")

	attemptKey, err := r.migrationAttemptKey(ctx, instance, runtimeConfig)
	if err != nil {
		return err
	}
	job, err := r.ensureMigrationJob(ctx, instance, runtimeConfig, attemptKey)
	if err != nil {
		return err
	}
	if job == nil {
		return nil
	}
	if !migrationJobIsCurrent(job, attemptKey) {
		r.markMigrationWaiting(instance, job, attemptKey,
			"Waiting for the previous migration Job to finish terminating before starting a new attempt")
		return nil
	}
	r.observeMigrationJob(instance, job, attemptKey)
	return nil
}

func (r *OGXServerReconciler) isMigrationRequested(instance *ogxiov1beta1.OGXServer) bool {
	if !r.resolvePraxisMode(instance) {
		return false
	}
	mj := instance.Spec.PraxisMode.MigrationJob
	if mj == nil {
		return false
	}
	return mj.Enabled == nil || *mj.Enabled
}

func (r *OGXServerReconciler) handleMigrationNotRequested(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	if err := r.deleteMigrationJob(ctx, instance); err != nil {
		return err
	}
	if !hasMigrationHistory(instance) {
		return nil
	}
	r.warnSoftRollbackOnPraxisDisable(instance)
	r.clearMigrationState(instance, "Migration is not opted in (requires praxisMode.enabled and praxisMode.migrationJob)")
	return nil
}

func hasMigrationHistory(instance *ogxiov1beta1.OGXServer) bool {
	if instance.Status.Migration != nil {
		mig := instance.Status.Migration
		if mig.SoftRollbackWarning != "" || mig.JobName != "" || mig.AttemptKey != "" ||
			(mig.Phase != "" && mig.Phase != ogxiov1beta1.MigrationPhasePending) {
			return true
		}
	}
	for _, c := range instance.Status.Conditions {
		switch c.Type {
		case ConditionTypeMigrationPreflightReady, ConditionTypeMigrationJobSucceeded,
			ConditionTypeMigrationValidated, ConditionTypePraxisCutoverReady,
			ConditionTypeSoftRollbackAvailable:
			return true
		}
	}
	return false
}

func (r *OGXServerReconciler) clearMigrationState(instance *ogxiov1beta1.OGXServer, message string) {
	priorWarning := ""
	if instance.Status.Migration != nil {
		priorWarning = instance.Status.Migration.SoftRollbackWarning
	}
	instance.Status.Migration = &ogxiov1beta1.MigrationStatus{
		Phase:               ogxiov1beta1.MigrationPhasePending,
		Message:             message,
		SoftRollbackWarning: priorWarning,
	}
	SetMigrationPreflightReadyCondition(&instance.Status, false, ReasonMigrationNotRequested, message)
	SetMigrationJobSucceededCondition(&instance.Status, false, ReasonMigrationNotRequested, message)
	SetMigrationValidatedCondition(&instance.Status, false, ReasonMigrationNotRequested, message)
	SetPraxisCutoverReadyCondition(&instance.Status, false, ReasonPraxisCutoverNotReady, message)
	if priorWarning == "" {
		SetSoftRollbackAvailableCondition(&instance.Status, false, ReasonMigrationNotRequested, message)
	}
}

func (r *OGXServerReconciler) ensureSoftRollbackWarning(instance *ogxiov1beta1.OGXServer) {
	if instance.Status.Migration == nil {
		instance.Status.Migration = &ogxiov1beta1.MigrationStatus{}
	}
	instance.Status.Migration.SoftRollbackWarning = SoftRollbackWarningMessage
	SetSoftRollbackAvailableCondition(&instance.Status, true, ReasonSoftRollbackOnly, SoftRollbackWarningMessage)
}

func (r *OGXServerReconciler) applyPreflightFailure(instance *ogxiov1beta1.OGXServer, preflightErr error) {
	msg := preflightErr.Error()
	instance.Status.Migration = &ogxiov1beta1.MigrationStatus{
		Phase:               ogxiov1beta1.MigrationPhasePreflightFailed,
		ObservedGeneration:  instance.Generation,
		Message:             msg,
		SoftRollbackWarning: SoftRollbackWarningMessage,
	}
	SetMigrationPreflightReadyCondition(&instance.Status, false, ReasonMigrationPreflightFailed, msg)
	SetMigrationJobSucceededCondition(&instance.Status, false, ReasonMigrationPreflightFailed, msg)
	SetMigrationValidatedCondition(&instance.Status, false, ReasonMigrationValidationPending, msg)
	SetPraxisCutoverReadyCondition(&instance.Status, false, ReasonPraxisCutoverNotReady,
		"Cutover is blocked until the migration Job completes successfully")
	SetSoftRollbackAvailableCondition(&instance.Status, true, ReasonSoftRollbackOnly, SoftRollbackWarningMessage)
}

func (r *OGXServerReconciler) runMigrationPreflight(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
) error {
	sourceRef, err := migrationSourceSecretRef(instance)
	if err != nil {
		return err
	}
	if err = r.ensureSecretKeyExists(ctx, instance.Namespace, sourceRef); err != nil {
		return fmt.Errorf("failed to pass migration preflight for OGX DB secret: %w", err)
	}
	targetRef, err := requireMigrationTargetSecretRef(instance)
	if err != nil {
		return err
	}
	if err = r.ensureSecretKeyExists(ctx, instance.Namespace, *targetRef); err != nil {
		return fmt.Errorf("failed to pass migration preflight for Praxis DB secret: %w", err)
	}
	if err = r.ensureDistinctMigrationDatabases(ctx, instance.Namespace, sourceRef, *targetRef); err != nil {
		return err
	}
	if err = r.ensureMigrationRuntimeConfig(ctx, instance.Namespace, runtimeConfig); err != nil {
		return err
	}
	if err = r.ensureMigrationCABundle(ctx, instance); err != nil {
		return err
	}
	if _, err = r.resolveImage(instance.Spec.Distribution); err != nil {
		return fmt.Errorf("failed to pass migration preflight: %w", err)
	}
	logger := log.FromContext(ctx)
	logger.Info("Migration preflight passed",
		"sourceSecret", sourceRef.Name,
		"targetSecret", targetRef.Name,
		"configMap", migrationConfigSource(runtimeConfig))
	return nil
}

func migrationSourceSecretRef(instance *ogxiov1beta1.OGXServer) (ogxiov1beta1.SecretKeyRef, error) {
	if instance.Spec.Storage == nil || instance.Spec.Storage.SQL == nil {
		return ogxiov1beta1.SecretKeyRef{}, errors.New("failed to pass migration preflight: spec.storage.sql must be configured")
	}
	if instance.Spec.Storage.SQL.Type != "postgres" {
		return ogxiov1beta1.SecretKeyRef{}, errors.New("failed to pass migration preflight: migration requires spec.storage.sql.type=postgres")
	}
	if instance.Spec.Storage.SQL.ConnectionString == nil {
		return ogxiov1beta1.SecretKeyRef{}, errors.New("failed to pass migration preflight: spec.storage.sql.connectionString is required")
	}
	return *instance.Spec.Storage.SQL.ConnectionString, nil
}

func requireMigrationTargetSecretRef(instance *ogxiov1beta1.OGXServer) (*ogxiov1beta1.SecretKeyRef, error) {
	targetRef := migrationTargetSecretRef(instance)
	if targetRef == nil {
		return nil, errors.New("failed to pass migration preflight: spec.praxisMode.migrationJob.targetConnectionString is required")
	}
	return targetRef, nil
}

func (r *OGXServerReconciler) ensureDistinctMigrationDatabases(
	ctx context.Context,
	namespace string,
	sourceRef, targetRef ogxiov1beta1.SecretKeyRef,
) error {
	if sourceRef.Name == targetRef.Name && sourceRef.Key == targetRef.Key {
		return errors.New("failed to pass migration preflight: targetConnectionString must not reference the same Secret key as spec.storage.sql.connectionString")
	}
	srcFP, err := r.secretKeyFingerprint(ctx, namespace, sourceRef)
	if err != nil {
		return fmt.Errorf("failed to pass migration preflight for OGX DB secret: %w", err)
	}
	dstFP, err := r.secretKeyFingerprint(ctx, namespace, targetRef)
	if err != nil {
		return fmt.Errorf("failed to pass migration preflight for Praxis DB secret: %w", err)
	}
	if srcFP == dstFP {
		return errors.New("failed to pass migration preflight: OGX and Praxis connection strings must not be the same database")
	}
	return nil
}

func (r *OGXServerReconciler) ensureMigrationRuntimeConfig(
	ctx context.Context,
	namespace string,
	runtimeConfig *runtimeConfigRef,
) error {
	if runtimeConfig == nil {
		return nil
	}
	if err := r.ensureConfigMapKeyExists(ctx, namespace, runtimeConfig); err != nil {
		return fmt.Errorf("failed to pass migration preflight for runtime config: %w", err)
	}
	return nil
}

func (r *OGXServerReconciler) ensureMigrationCABundle(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	if !hasAnyCABundle(ctx, r, instance) {
		return nil
	}
	caName := getManagedCABundleConfigMapName(instance)
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: caName, Namespace: instance.Namespace}, cm); err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to pass migration preflight: CA bundle ConfigMap %q is required when TLS trust is configured",
				caName)
		}
		return fmt.Errorf("failed to pass migration preflight for CA bundle ConfigMap: %w", err)
	}
	return nil
}

func migrationConfigSource(runtimeConfig *runtimeConfigRef) string {
	if runtimeConfig == nil {
		return ogxConfigPath
	}
	return runtimeConfig.ConfigMapName
}

func migrationTargetSecretRef(instance *ogxiov1beta1.OGXServer) *ogxiov1beta1.SecretKeyRef {
	if instance.Spec.PraxisMode == nil || instance.Spec.PraxisMode.MigrationJob == nil {
		return nil
	}
	return instance.Spec.PraxisMode.MigrationJob.TargetConnectionString
}

func (r *OGXServerReconciler) ensureSecretKeyExists(
	ctx context.Context,
	namespace string,
	ref ogxiov1beta1.SecretKeyRef,
) error {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to find Secret %q (must exist and carry label %s=%s)",
				ref.Name, WatchLabelKey, WatchLabelValue)
		}
		return fmt.Errorf("failed to get Secret %q: %w", ref.Name, err)
	}
	if secret.Labels[WatchLabelKey] != WatchLabelValue {
		return fmt.Errorf("failed to find Secret %q (must exist and carry label %s=%s)",
			ref.Name, WatchLabelKey, WatchLabelValue)
	}
	if _, ok := secret.Data[ref.Key]; !ok {
		return fmt.Errorf("failed to find key %q in Secret %q", ref.Key, ref.Name)
	}
	return nil
}

func (r *OGXServerReconciler) ensureConfigMapKeyExists(
	ctx context.Context,
	namespace string,
	runtimeConfig *runtimeConfigRef,
) error {
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: runtimeConfig.ConfigMapName, Namespace: namespace}, cm); err != nil {
		if k8serrors.IsNotFound(err) {
			return fmt.Errorf("failed to find ConfigMap %q (must exist and carry label %s=%s)",
				runtimeConfig.ConfigMapName, WatchLabelKey, WatchLabelValue)
		}
		return fmt.Errorf("failed to get ConfigMap %q: %w", runtimeConfig.ConfigMapName, err)
	}
	if _, ok := cm.Data[runtimeConfig.ConfigMapKey]; !ok {
		if _, ok := cm.BinaryData[runtimeConfig.ConfigMapKey]; !ok {
			return fmt.Errorf("failed to find key %q in ConfigMap %q",
				runtimeConfig.ConfigMapKey, runtimeConfig.ConfigMapName)
		}
	}
	return nil
}

func (r *OGXServerReconciler) migrationAttemptKey(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
) (string, error) {
	var b strings.Builder
	b.WriteString("src=")
	if instance.Spec.Storage != nil && instance.Spec.Storage.SQL != nil && instance.Spec.Storage.SQL.ConnectionString != nil {
		ref := instance.Spec.Storage.SQL.ConnectionString
		fp, err := r.secretKeyFingerprint(ctx, instance.Namespace, *ref)
		if err != nil {
			return "", err
		}
		b.WriteString(ref.Name)
		b.WriteByte('/')
		b.WriteString(ref.Key)
		b.WriteByte('@')
		b.WriteString(fp)
	}
	b.WriteString(";dst=")
	if target := migrationTargetSecretRef(instance); target != nil {
		dstFP, err := r.secretKeyFingerprint(ctx, instance.Namespace, *target)
		if err != nil {
			return "", err
		}
		b.WriteString(target.Name)
		b.WriteByte('/')
		b.WriteString(target.Key)
		b.WriteByte('@')
		b.WriteString(dstFP)
	}
	b.WriteString(";cfg=")
	if runtimeConfig != nil {
		cfgFP, cfgErr := r.configMapKeyFingerprint(ctx, instance.Namespace, runtimeConfig)
		if cfgErr != nil {
			return "", cfgErr
		}
		b.WriteString(runtimeConfig.ConfigMapName)
		b.WriteByte('/')
		b.WriteString(runtimeConfig.ConfigMapKey)
		b.WriteByte('@')
		b.WriteString(cfgFP)
	} else {
		b.WriteString(ogxConfigPath)
	}
	image, err := r.resolveImage(instance.Spec.Distribution)
	if err != nil {
		return "", fmt.Errorf("failed to resolve migration image for attempt key: %w", err)
	}
	b.WriteString(";image=")
	b.WriteString(image)
	sum := sha256.Sum256([]byte(b.String()))
	return hex.EncodeToString(sum[:8]), nil
}

func (r *OGXServerReconciler) secretKeyFingerprint(
	ctx context.Context,
	namespace string,
	ref ogxiov1beta1.SecretKeyRef,
) (string, error) {
	secret := &corev1.Secret{}
	if err := r.Get(ctx, types.NamespacedName{Name: ref.Name, Namespace: namespace}, secret); err != nil {
		return "", fmt.Errorf("failed to fingerprint Secret %q: %w", ref.Name, err)
	}
	return fingerprintBytes(secret.Data[ref.Key]), nil
}

func (r *OGXServerReconciler) configMapKeyFingerprint(
	ctx context.Context,
	namespace string,
	runtimeConfig *runtimeConfigRef,
) (string, error) {
	cm := &corev1.ConfigMap{}
	if err := r.Get(ctx, types.NamespacedName{Name: runtimeConfig.ConfigMapName, Namespace: namespace}, cm); err != nil {
		return "", fmt.Errorf("failed to fingerprint ConfigMap %q: %w", runtimeConfig.ConfigMapName, err)
	}
	if raw, ok := cm.BinaryData[runtimeConfig.ConfigMapKey]; ok {
		return fingerprintBytes(raw), nil
	}
	return fingerprintBytes([]byte(cm.Data[runtimeConfig.ConfigMapKey])), nil
}

func fingerprintBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:8])
}

func migrationJobName(instanceName string) string {
	return instanceName + migrationJobNameSuffix
}

func (r *OGXServerReconciler) getMigrationJob(
	ctx context.Context,
	namespace, name string,
) (*batchv1.Job, error) {
	job := &batchv1.Job{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: namespace}, job)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get migration Job %s/%s: %w", namespace, name, err)
	}
	return job, nil
}

func migrationJobIsCurrent(job *batchv1.Job, attemptKey string) bool {
	if job == nil {
		return false
	}
	if !job.DeletionTimestamp.IsZero() {
		return false
	}
	return job.Annotations[migrationAttemptAnnotation] == attemptKey
}

func migrationAttemptAlreadySucceeded(instance *ogxiov1beta1.OGXServer, attemptKey string) bool {
	if instance.Status.Migration == nil {
		return false
	}
	return instance.Status.Migration.Phase == ogxiov1beta1.MigrationPhaseValidated &&
		instance.Status.Migration.AttemptKey == attemptKey
}

func (r *OGXServerReconciler) ensureMigrationJob(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
	attemptKey string,
) (*batchv1.Job, error) {
	jobName := migrationJobName(instance.Name)
	existing, err := r.getMigrationJob(ctx, instance.Namespace, jobName)
	if err != nil {
		return nil, err
	}
	if existing != nil {
		return r.reconcileExistingMigrationJob(ctx, instance, existing, attemptKey)
	}
	if migrationAttemptAlreadySucceeded(instance, attemptKey) {
		return nil, nil
	}
	return r.createMigrationJob(ctx, instance, runtimeConfig, jobName, attemptKey)
}

func (r *OGXServerReconciler) reconcileExistingMigrationJob(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	existing *batchv1.Job,
	attemptKey string,
) (*batchv1.Job, error) {
	if err := assertOwnedMigrationJob(instance, existing); err != nil {
		return nil, err
	}
	if !existing.DeletionTimestamp.IsZero() {
		return existing, nil
	}
	if existing.Annotations[migrationAttemptAnnotation] != attemptKey {
		if err := r.replaceStaleMigrationJob(ctx, existing, attemptKey); err != nil {
			return nil, err
		}
	}
	return existing, nil
}

func assertOwnedMigrationJob(instance *ogxiov1beta1.OGXServer, job *batchv1.Job) error {
	if metav1.IsControlledBy(job, instance) {
		return nil
	}
	return fmt.Errorf("failed to reconcile migration Job %s/%s: Job is not owned by OGXServer %s/%s",
		job.Namespace, job.Name, instance.Namespace, instance.Name)
}

func (r *OGXServerReconciler) deleteMigrationJob(ctx context.Context, instance *ogxiov1beta1.OGXServer) error {
	job, err := r.getMigrationJob(ctx, instance.Namespace, migrationJobName(instance.Name))
	if err != nil {
		return err
	}
	if job == nil || !job.DeletionTimestamp.IsZero() {
		return nil
	}
	if err := assertOwnedMigrationJob(instance, job); err != nil {
		return err
	}
	logger := log.FromContext(ctx)
	logger.Info("Deleting migration Job", "job", job.Name)
	return deleteMigrationJobObject(ctx, r.Client, job)
}

func (r *OGXServerReconciler) replaceStaleMigrationJob(
	ctx context.Context,
	existing *batchv1.Job,
	newAttemptKey string,
) error {
	logger := log.FromContext(ctx)
	logger.Info("Replacing migration Job",
		"job", existing.Name,
		"existingAttempt", existing.Annotations[migrationAttemptAnnotation],
		"desiredAttempt", newAttemptKey,
		"failed", jobHasCondition(existing, batchv1.JobFailed))
	return deleteMigrationJobObject(ctx, r.Client, existing)
}

func deleteMigrationJobObject(ctx context.Context, c client.Client, job *batchv1.Job) error {
	propagation := metav1.DeletePropagationForeground
	if err := c.Delete(ctx, job, &client.DeleteOptions{PropagationPolicy: &propagation}); err != nil &&
		!k8serrors.IsNotFound(err) {
		return fmt.Errorf("failed to delete migration Job %s/%s: %w", job.Namespace, job.Name, err)
	}
	return nil
}

func (r *OGXServerReconciler) createMigrationJob(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
	jobName, attemptKey string,
) (*batchv1.Job, error) {
	job, err := r.buildMigrationJob(ctx, instance, runtimeConfig, jobName, attemptKey)
	if err != nil {
		return nil, err
	}
	if err := r.Create(ctx, job); err != nil {
		if k8serrors.IsAlreadyExists(err) {
			existing, getErr := r.getMigrationJob(ctx, instance.Namespace, jobName)
			if getErr != nil {
				return nil, getErr
			}
			if existing == nil {
				return nil, fmt.Errorf("failed to create migration Job %s/%s: already exists but could not be read",
					instance.Namespace, jobName)
			}
			if ownErr := assertOwnedMigrationJob(instance, existing); ownErr != nil {
				return nil, ownErr
			}
			return existing, nil
		}
		return nil, fmt.Errorf("failed to create migration Job %s/%s: %w", instance.Namespace, jobName, err)
	}
	r.emitMigrationEvent(instance, corev1.EventTypeNormal, ReasonMigrationJobPending,
		fmt.Sprintf("Created migration Job %q for attempt %s", jobName, attemptKey))
	return job, nil
}

func (r *OGXServerReconciler) buildMigrationJob(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
	jobName, attemptKey string,
) (*batchv1.Job, error) {
	image, err := r.resolveImage(instance.Spec.Distribution)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve migration Job image: %w", err)
	}
	container, volumes := r.migrationJobPodInputs(ctx, instance, runtimeConfig, image)
	job := newMigrationJob(instance, jobName, attemptKey, container, volumes)
	applyMigrationPodDefaults(instance, &job.Spec.Template.Spec)
	if err := controllerutil.SetControllerReference(instance, job, r.Scheme); err != nil {
		return nil, fmt.Errorf("failed to set owner reference on migration Job: %w", err)
	}
	return job, nil
}

func (r *OGXServerReconciler) migrationJobPodInputs(
	ctx context.Context,
	instance *ogxiov1beta1.OGXServer,
	runtimeConfig *runtimeConfigRef,
	image string,
) (corev1.Container, []corev1.Volume) {
	container := corev1.Container{
		Name:            migrationContainerName,
		Image:           image,
		ImagePullPolicy: corev1.PullIfNotPresent,
		Command:         []string{"/bin/sh", "-c"},
		Args:            []string{migrationJobShell},
		Env:             migrationJobEnv(instance),
	}
	volumes := []corev1.Volume{}
	if runtimeConfig != nil {
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      "user-config",
			MountPath: "/etc/ogx/",
			ReadOnly:  true,
		})
		volumes = append(volumes, corev1.Volume{
			Name: "user-config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{Name: runtimeConfig.ConfigMapName},
					Items: []corev1.KeyToPath{{
						Key:  runtimeConfig.ConfigMapKey,
						Path: "config.yaml",
					}},
				},
			},
		})
	}
	if hasAnyCABundle(ctx, r, instance) {
		container.Env = append(container.Env, corev1.EnvVar{
			Name:  "SSL_CERT_FILE",
			Value: ManagedCABundleFilePath,
		})
		container.VolumeMounts = append(container.VolumeMounts, corev1.VolumeMount{
			Name:      CABundleVolumeName,
			MountPath: ManagedCABundleMountPath,
			ReadOnly:  true,
		})
		volumes = append(volumes, createCABundleVolume(getManagedCABundleConfigMapName(instance)))
	}
	return container, volumes
}

func migrationJobEnv(instance *ogxiov1beta1.OGXServer) []corev1.EnvVar {
	env := append([]corev1.EnvVar{}, config.CollectSecretRefs(&instance.Spec)...)
	env = append(env,
		corev1.EnvVar{Name: "OGX_CONFIG", Value: ogxConfigPath},
		corev1.EnvVar{Name: "RUN_CONFIG_PATH", Value: ogxConfigPath},
	)
	if targetRef := migrationTargetSecretRef(instance); targetRef != nil {
		env = append(env, corev1.EnvVar{
			Name: praxisDatabaseURLEnv,
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: targetRef.Name},
					Key:                  targetRef.Key,
				},
			},
		})
	}
	return env
}

func newMigrationJob(
	instance *ogxiov1beta1.OGXServer,
	jobName, attemptKey string,
	container corev1.Container,
	volumes []corev1.Volume,
) *batchv1.Job {
	labels := map[string]string{
		migrationManagedLabelKey:     migrationManagedLabelValue,
		migrationComponentLabelKey:   migrationComponentLabelValue,
		ogxiov1beta1.DefaultLabelKey: ogxiov1beta1.DefaultLabelValue,
		"app.kubernetes.io/instance": instance.Name,
	}
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:        jobName,
			Namespace:   instance.Namespace,
			Labels:      labels,
			Annotations: map[string]string{migrationAttemptAnnotation: attemptKey},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit:          ptr.To(migrationJobBackoffLimit),
			Completions:           ptr.To(int32(1)),
			Parallelism:           ptr.To(int32(1)),
			ActiveDeadlineSeconds: ptr.To(migrationActiveDeadlineSecs),
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{Labels: labels},
				Spec: corev1.PodSpec{
					RestartPolicy: corev1.RestartPolicyOnFailure,
					Containers:    []corev1.Container{container},
					Volumes:       volumes,
				},
			},
		},
	}
}

func applyMigrationPodDefaults(instance *ogxiov1beta1.OGXServer, podSpec *corev1.PodSpec) {
	fsGroup := FSGroup
	podSpec.SecurityContext = &corev1.PodSecurityContext{FSGroup: &fsGroup}
	configurePodOverrides(instance, podSpec)
	if instance.Spec.Workload != nil && instance.Spec.Workload.Resources != nil && len(podSpec.Containers) > 0 {
		podSpec.Containers[0].Resources = *instance.Spec.Workload.Resources.DeepCopy()
	}
}

func (r *OGXServerReconciler) markMigrationWaiting(
	instance *ogxiov1beta1.OGXServer,
	job *batchv1.Job,
	attemptKey, message string,
) {
	jobName := migrationJobName(instance.Name)
	if job != nil {
		jobName = job.Name
	}
	instance.Status.Migration = &ogxiov1beta1.MigrationStatus{
		Phase:               ogxiov1beta1.MigrationPhaseRunning,
		ObservedGeneration:  instance.Generation,
		AttemptKey:          attemptKey,
		JobName:             jobName,
		Message:             message,
		SoftRollbackWarning: SoftRollbackWarningMessage,
	}
	SetMigrationJobSucceededCondition(&instance.Status, false, ReasonMigrationJobPending, message)
	SetMigrationValidatedCondition(&instance.Status, false, ReasonMigrationValidationPending, message)
	SetPraxisCutoverReadyCondition(&instance.Status, false, ReasonPraxisCutoverNotReady,
		"Cutover is blocked until the migration Job completes successfully")
	SetSoftRollbackAvailableCondition(&instance.Status, true, ReasonSoftRollbackOnly, SoftRollbackWarningMessage)
}

func (r *OGXServerReconciler) observeMigrationJob(
	instance *ogxiov1beta1.OGXServer,
	job *batchv1.Job,
	attemptKey string,
) {
	status := &ogxiov1beta1.MigrationStatus{
		ObservedGeneration:  instance.Generation,
		AttemptKey:          attemptKey,
		JobName:             job.Name,
		SoftRollbackWarning: SoftRollbackWarningMessage,
	}
	SetSoftRollbackAvailableCondition(&instance.Status, true, ReasonSoftRollbackOnly, SoftRollbackWarningMessage)

	switch {
	case jobHasCondition(job, batchv1.JobComplete):
		r.applyMigrationJobComplete(instance, status, job)
	case jobHasCondition(job, batchv1.JobFailed):
		r.applyMigrationJobFailed(instance, status, job)
	case job.Status.Active > 0:
		status.Phase = ogxiov1beta1.MigrationPhaseRunning
		status.Message = fmt.Sprintf("Migration Job %q is active", job.Name)
		SetMigrationJobSucceededCondition(&instance.Status, false, ReasonMigrationJobRunning, status.Message)
		SetMigrationValidatedCondition(&instance.Status, false, ReasonMigrationValidationPending, status.Message)
		SetPraxisCutoverReadyCondition(&instance.Status, false, ReasonPraxisCutoverNotReady,
			"Cutover is blocked until the migration Job completes successfully")
	default:
		status.Phase = ogxiov1beta1.MigrationPhaseRunning
		status.Message = fmt.Sprintf("Migration Job %q created; waiting for completion", job.Name)
		SetMigrationJobSucceededCondition(&instance.Status, false, ReasonMigrationJobPending, status.Message)
		SetMigrationValidatedCondition(&instance.Status, false, ReasonMigrationValidationPending, status.Message)
		SetPraxisCutoverReadyCondition(&instance.Status, false, ReasonPraxisCutoverNotReady,
			"Cutover is blocked until the migration Job completes successfully")
	}
	instance.Status.Migration = status
}

func (r *OGXServerReconciler) applyMigrationJobComplete(
	instance *ogxiov1beta1.OGXServer,
	status *ogxiov1beta1.MigrationStatus,
	job *batchv1.Job,
) {
	alreadyReady := IsConditionTrue(&instance.Status, ConditionTypePraxisCutoverReady) &&
		GetCondition(&instance.Status, ConditionTypePraxisCutoverReady).Reason == ReasonPraxisCutoverReady
	status.Phase = ogxiov1beta1.MigrationPhaseValidated
	status.Message = fmt.Sprintf("Migration Job %q completed", job.Name)
	SetMigrationJobSucceededCondition(&instance.Status, true, ReasonMigrationJobSucceeded, status.Message)
	SetMigrationValidatedCondition(&instance.Status, true, ReasonMigrationValidated, status.Message)
	SetPraxisCutoverReadyCondition(&instance.Status, true, ReasonPraxisCutoverReady,
		"Migration Job completed; cutover may proceed")
	if !alreadyReady {
		r.emitMigrationEvent(instance, corev1.EventTypeNormal, ReasonPraxisCutoverReady, status.Message)
	}
}

func (r *OGXServerReconciler) applyMigrationJobFailed(
	instance *ogxiov1beta1.OGXServer,
	status *ogxiov1beta1.MigrationStatus,
	job *batchv1.Job,
) {
	status.Phase = ogxiov1beta1.MigrationPhaseFailed
	status.Message = fmt.Sprintf("Migration Job %q failed; inspect logs with `oc logs job/%s`. Delete the Job to retry",
		job.Name, job.Name)
	alreadyFailed := GetCondition(&instance.Status, ConditionTypeMigrationJobSucceeded)
	SetMigrationJobSucceededCondition(&instance.Status, false, ReasonMigrationJobFailed, status.Message)
	SetMigrationValidatedCondition(&instance.Status, false, ReasonMigrationValidationFailed, status.Message)
	SetPraxisCutoverReadyCondition(&instance.Status, false, ReasonPraxisCutoverNotReady,
		"Cutover is blocked because the migration Job did not succeed")
	if alreadyFailed == nil || alreadyFailed.Reason != ReasonMigrationJobFailed {
		r.emitMigrationEvent(instance, corev1.EventTypeWarning, ReasonMigrationJobFailed, status.Message)
	}
}

func jobHasCondition(job *batchv1.Job, condType batchv1.JobConditionType) bool {
	for i := range job.Status.Conditions {
		c := job.Status.Conditions[i]
		if c.Type == condType && c.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}

func (r *OGXServerReconciler) emitMigrationEvent(instance *ogxiov1beta1.OGXServer, eventType, reason, message string) {
	if r.Recorder == nil {
		return
	}
	r.Recorder.Eventf(instance, nil, eventType, reason, reason, "%s", message)
}

func (r *OGXServerReconciler) warnSoftRollbackOnPraxisDisable(instance *ogxiov1beta1.OGXServer) {
	if r.isMigrationRequested(instance) {
		return
	}
	mig := instance.Status.Migration
	if mig == nil {
		return
	}
	mig.SoftRollbackWarning = SoftRollbackWarningMessage
	SetSoftRollbackAvailableCondition(&instance.Status, true, ReasonSoftRollbackOnly, SoftRollbackWarningMessage)
}

func migrationNeedsRequeue(instance *ogxiov1beta1.OGXServer) bool {
	if instance.Status.Migration == nil {
		return false
	}
	switch instance.Status.Migration.Phase {
	case ogxiov1beta1.MigrationPhaseRunning:
		return true
	default:
		return false
	}
}
