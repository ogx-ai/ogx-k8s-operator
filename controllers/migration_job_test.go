package controllers

import (
	"context"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/stretchr/testify/require"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/tools/events"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func migrationScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, batchv1.AddToScheme(scheme))
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))
	return scheme
}

func migrationReconciler(t *testing.T, objs ...client.Object) (*OGXServerReconciler, *events.FakeRecorder) {
	t.Helper()
	scheme := migrationScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).
		WithStatusSubresource(&batchv1.Job{}).Build()
	rec := events.NewFakeRecorder(20)
	return &OGXServerReconciler{
		Client:   c,
		Scheme:   scheme,
		Recorder: rec,
		ClusterInfo: &cluster.ClusterInfo{
			DistributionImages: map[string]string{"starter": "quay.io/ogx/starter:test"},
		},
		ImageMappingOverrides: map[string]string{},
	}, rec
}

func sourceSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pg-secret",
			Namespace: "ogx",
			Labels:    map[string]string{WatchLabelKey: WatchLabelValue},
		},
		Data: map[string][]byte{"conn": []byte("postgres://ogx")},
	}
}

func targetSecret() *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "praxis-pg",
			Namespace: "ogx",
			Labels:    map[string]string{WatchLabelKey: WatchLabelValue},
		},
		Data: map[string][]byte{"url": []byte("postgres://praxis")},
	}
}

func runtimeConfigMap() *corev1.ConfigMap {
	return &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-config",
			Namespace: "ogx",
			Labels:    map[string]string{WatchLabelKey: WatchLabelValue},
		},
		Data: map[string]string{"config.yaml": "version: 2\n"},
	}
}

func migrationObjects() []client.Object {
	return []client.Object{sourceSecret(), targetSecret(), runtimeConfigMap()}
}

func migrationInstance(enabled bool, withMigrationJob bool) *ogxiov1beta1.OGXServer {
	inst := &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "demo",
			Namespace:  "ogx",
			Generation: 3,
			UID:        types.UID("a1b2c3d4-e5f6-7890-abcd-ef1234567890"),
		},
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Image: "quay.io/ogx/demo:test"},
			Storage: &ogxiov1beta1.StateStorageSpec{
				SQL: &ogxiov1beta1.SQLStorageSpec{
					Type: "postgres",
					ConnectionString: &ogxiov1beta1.SecretKeyRef{
						Name: "pg-secret",
						Key:  "conn",
					},
				},
			},
			PraxisMode: &ogxiov1beta1.PraxisModeSpec{
				Enabled: ptr.To(enabled),
			},
		},
	}
	if withMigrationJob {
		inst.Spec.PraxisMode.MigrationJob = &ogxiov1beta1.MigrationJobSpec{
			Enabled: ptr.To(true),
			TargetConnectionString: &ogxiov1beta1.SecretKeyRef{
				Name: "praxis-pg",
				Key:  "url",
			},
		}
	}
	return inst
}

func runtimeCfg() *runtimeConfigRef {
	return &runtimeConfigRef{ConfigMapName: "demo-config", ConfigMapKey: "config.yaml"}
}

func markJobComplete(job *batchv1.Job) {
	job.Status.Succeeded = 1
	job.Status.Conditions = []batchv1.JobCondition{{
		Type:   batchv1.JobComplete,
		Status: corev1.ConditionTrue,
	}}
}

func markJobFailed(job *batchv1.Job) {
	job.Status.Failed = migrationJobBackoffLimit + 1
	job.Status.Conditions = []batchv1.JobCondition{{
		Type:   batchv1.JobFailed,
		Status: corev1.ConditionTrue,
	}}
}

func TestReconcileMigration_SuccessPathSetsCutoverReady(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	require.Equal(t, []string{"/bin/sh", "-c"}, job.Spec.Template.Spec.Containers[0].Command)
	require.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "set -eu")
	require.NotContains(t, job.Spec.Template.Spec.Containers[0].Args[0], "pipefail")
	require.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "ogx migrate praxis --dry-run")
	require.Contains(t, job.Spec.Template.Spec.Containers[0].Args[0], "ogx migrate praxis \"$OGX_CONFIG\"")
	require.Equal(t, "demo-sa", job.Spec.Template.Spec.ServiceAccountName)
	require.NotNil(t, job.Spec.Template.Spec.SecurityContext)
	require.NotNil(t, job.Spec.Template.Spec.SecurityContext.FSGroup)
	require.Equal(t, FSGroup, *job.Spec.Template.Spec.SecurityContext.FSGroup)

	var hasPraxisURL bool
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == praxisDatabaseURLEnv {
			hasPraxisURL = true
			require.NotNil(t, env.ValueFrom)
			require.NotNil(t, env.ValueFrom.SecretKeyRef)
			require.Equal(t, "praxis-pg", env.ValueFrom.SecretKeyRef.Name)
			require.Equal(t, "url", env.ValueFrom.SecretKeyRef.Key)
			require.Empty(t, env.Value, "must not inline DB credentials")
		}
	}
	require.True(t, hasPraxisURL)
	require.NotEmpty(t, job.Spec.Template.Spec.Volumes)
	require.Equal(t, "demo-config", job.Spec.Template.Spec.Volumes[0].ConfigMap.Name)

	markJobComplete(job)
	require.NoError(t, r.Status().Update(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	require.True(t, IsConditionTrue(&inst.Status, ConditionTypeMigrationPreflightReady))
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypeMigrationJobSucceeded))
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypeMigrationValidated))
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))
	require.Equal(t, ogxiov1beta1.MigrationPhaseValidated, inst.Status.Migration.Phase)
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypeSoftRollbackAvailable))
	require.Contains(t, inst.Status.Migration.SoftRollbackWarning, "data loss")
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))
}

func TestReconcileMigration_UsesImageConfigWhenRuntimeConfigOmitted(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, sourceSecret(), targetSecret())
	require.NoError(t, r.reconcileMigration(context.Background(), inst, nil))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	require.Empty(t, job.Spec.Template.Spec.Volumes)
	require.Empty(t, job.Spec.Template.Spec.Containers[0].VolumeMounts)
	var ogxConfig string
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "OGX_CONFIG" {
			ogxConfig = env.Value
		}
	}
	require.Equal(t, "/etc/ogx/config.yaml", ogxConfig)
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypeMigrationPreflightReady))
}

func TestReconcileMigration_RequiresOGXDBSecret(t *testing.T) {
	inst := migrationInstance(true, true)
	inst.Spec.Storage = nil
	r, _ := migrationReconciler(t, targetSecret())
	require.NoError(t, r.reconcileMigration(context.Background(), inst, nil))
	require.Equal(t, ogxiov1beta1.MigrationPhasePreflightFailed, inst.Status.Migration.Phase)
	require.Contains(t, inst.Status.Migration.Message, "spec.storage.sql")
	jobList := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), jobList))
	require.Empty(t, jobList.Items)
}

func TestReconcileMigration_PreflightFailure(t *testing.T) {
	inst := migrationInstance(true, true)
	r, rec := migrationReconciler(t)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	require.True(t, IsConditionFalse(&inst.Status, ConditionTypeMigrationPreflightReady))
	require.Equal(t, ogxiov1beta1.MigrationPhasePreflightFailed, inst.Status.Migration.Phase)
	require.True(t, IsConditionFalse(&inst.Status, ConditionTypePraxisCutoverReady))
	require.False(t, IsConditionTrue(&inst.Status, ConditionTypeMigrationValidated))

	jobList := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), jobList))
	require.Empty(t, jobList.Items)

	select {
	case evt := <-rec.Events:
		require.Contains(t, evt, ReasonMigrationPreflightFailed)
	default:
		t.Fatal("expected preflight failure event")
	}
}

func TestReconcileMigration_PreflightRequiresWatchLabel(t *testing.T) {
	src := sourceSecret()
	src.Labels = nil
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, src, targetSecret(), runtimeConfigMap())
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.Equal(t, ogxiov1beta1.MigrationPhasePreflightFailed, inst.Status.Migration.Phase)
	require.Contains(t, inst.Status.Migration.Message, WatchLabelKey)
}

func TestReconcileMigration_RequiresTargetConnectionString(t *testing.T) {
	inst := migrationInstance(true, true)
	inst.Spec.PraxisMode.MigrationJob.TargetConnectionString = nil
	r, _ := migrationReconciler(t, sourceSecret(), runtimeConfigMap())
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.Equal(t, ogxiov1beta1.MigrationPhasePreflightFailed, inst.Status.Migration.Phase)
	require.Contains(t, inst.Status.Migration.Message, "targetConnectionString")
	jobList := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), jobList))
	require.Empty(t, jobList.Items)
}

func TestReconcileMigration_RejectsSameSecretRef(t *testing.T) {
	inst := migrationInstance(true, true)
	inst.Spec.PraxisMode.MigrationJob.TargetConnectionString = &ogxiov1beta1.SecretKeyRef{
		Name: "pg-secret",
		Key:  "conn",
	}
	r, _ := migrationReconciler(t, sourceSecret(), runtimeConfigMap())
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.Equal(t, ogxiov1beta1.MigrationPhasePreflightFailed, inst.Status.Migration.Phase)
	require.Contains(t, inst.Status.Migration.Message, "same Secret key")
	jobList := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), jobList))
	require.Empty(t, jobList.Items)
}

func TestReconcileMigration_RejectsIdenticalConnectionString(t *testing.T) {
	dup := targetSecret()
	dup.Data["url"] = []byte("postgres://ogx")
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, sourceSecret(), dup, runtimeConfigMap())
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.Equal(t, ogxiov1beta1.MigrationPhasePreflightFailed, inst.Status.Migration.Phase)
	require.Contains(t, inst.Status.Migration.Message, "same database")
	jobList := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), jobList))
	require.Empty(t, jobList.Items)
}

func TestReconcileMigration_MountsCABundleWhenConfigured(t *testing.T) {
	inst := migrationInstance(true, true)
	inst.Spec.TLS = &ogxiov1beta1.TLSClientConfig{
		Trust: &ogxiov1beta1.TrustConfig{
			CACertificates: []ogxiov1beta1.ConfigMapKeyRef{{Name: "custom-ca", Key: "ca.crt"}},
		},
	}
	caBundle := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{Name: inst.Name + ManagedCABundleConfigMapSuffix, Namespace: "ogx"},
		Data:       map[string]string{ManagedCABundleKey: "-----BEGIN CERTIFICATE-----\nMIIB\n-----END CERTIFICATE-----\n"},
	}
	r, _ := migrationReconciler(t, append(migrationObjects(), caBundle)...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	var sslCert string
	for _, env := range job.Spec.Template.Spec.Containers[0].Env {
		if env.Name == "SSL_CERT_FILE" {
			sslCert = env.Value
		}
	}
	require.Equal(t, ManagedCABundleFilePath, sslCert)
	var hasCAVolume, hasCAMount bool
	for _, vol := range job.Spec.Template.Spec.Volumes {
		if vol.Name == CABundleVolumeName {
			hasCAVolume = true
			require.Equal(t, caBundle.Name, vol.ConfigMap.Name)
		}
	}
	for _, m := range job.Spec.Template.Spec.Containers[0].VolumeMounts {
		if m.Name == CABundleVolumeName {
			hasCAMount = true
			require.Equal(t, ManagedCABundleMountPath, m.MountPath)
		}
	}
	require.True(t, hasCAVolume)
	require.True(t, hasCAMount)
}

func TestReconcileMigration_JobFailure(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	markJobFailed(job)
	require.NoError(t, r.Status().Update(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	require.True(t, IsConditionFalse(&inst.Status, ConditionTypeMigrationJobSucceeded))
	require.Equal(t, ReasonMigrationJobFailed, GetCondition(&inst.Status, ConditionTypeMigrationJobSucceeded).Reason)
	require.True(t, IsConditionFalse(&inst.Status, ConditionTypeMigrationValidated))
	require.True(t, IsConditionFalse(&inst.Status, ConditionTypePraxisCutoverReady))
	require.Equal(t, ogxiov1beta1.MigrationPhaseFailed, inst.Status.Migration.Phase)
	require.Contains(t, inst.Status.Migration.Message, "oc logs")

	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	require.True(t, jobHasCondition(job, batchv1.JobFailed), "failed Job must be kept for CLI logs")
}

func TestReconcileMigration_PodFailureIsNotTerminal(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	job.Status.Failed = 1
	job.Status.Active = 1
	require.NoError(t, r.Status().Update(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	require.Equal(t, ogxiov1beta1.MigrationPhaseRunning, inst.Status.Migration.Phase)
	require.False(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))
	require.NotEqual(t, ReasonMigrationJobFailed, GetCondition(&inst.Status, ConditionTypeMigrationJobSucceeded).Reason)
}

func TestReconcileMigration_ValidationFailureKeepsCutoverBlocked(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	markJobFailed(job)
	require.NoError(t, r.Status().Update(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	require.Equal(t, ReasonMigrationValidationFailed, GetCondition(&inst.Status, ConditionTypeMigrationValidated).Reason)
	require.False(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))
}

func TestReconcileMigration_FailedJobIsKeptUntilDeleted(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	markJobFailed(job)
	require.NoError(t, r.Status().Update(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.Equal(t, ogxiov1beta1.MigrationPhaseFailed, inst.Status.Migration.Phase)

	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	require.True(t, jobHasCondition(job, batchv1.JobFailed))

	require.NoError(t, r.Delete(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	require.False(t, jobHasCondition(job, batchv1.JobFailed))
	require.Equal(t, int32(0), job.Status.Failed)
}

func TestReconcileMigration_SucceededJobNotRecreatedIfDeleted(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	markJobComplete(job)
	require.NoError(t, r.Status().Update(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.Equal(t, ogxiov1beta1.MigrationPhaseValidated, inst.Status.Migration.Phase)
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))

	require.NoError(t, r.Delete(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	list := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), list))
	require.Empty(t, list.Items)
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))
}

func TestReconcileMigration_DeletingJobIsNotObservedAsSuccess(t *testing.T) {
	inst := migrationInstance(true, true)
	probe, _ := migrationReconciler(t, migrationObjects()...)
	attemptKey, err := probe.migrationAttemptKey(context.Background(), inst, runtimeCfg())
	require.NoError(t, err)

	now := metav1.Now()
	deleting := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "demo-praxis-migration",
			Namespace:         "ogx",
			Annotations:       map[string]string{migrationAttemptAnnotation: attemptKey},
			DeletionTimestamp: &now,
			Finalizers:        []string{"ogx.io/test-keep"},
			OwnerReferences: []metav1.OwnerReference{{
				APIVersion: ogxiov1beta1.GroupVersion.String(),
				Kind:       ogxiov1beta1.OGXServerKind,
				Name:       inst.Name,
				UID:        inst.UID,
				Controller: ptr.To(true),
			}},
		},
	}
	markJobComplete(deleting)
	r, _ := migrationReconciler(t, append(migrationObjects(), deleting)...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.False(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))
	require.Equal(t, ogxiov1beta1.MigrationPhaseRunning, inst.Status.Migration.Phase)
}

func TestReconcileMigration_UnownedJobIsRejected(t *testing.T) {
	inst := migrationInstance(true, true)
	unowned := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "demo-praxis-migration",
			Namespace: "ogx",
			Annotations: map[string]string{
				migrationAttemptAnnotation: "foreign",
			},
		},
	}
	markJobComplete(unowned)
	r, _ := migrationReconciler(t, append(migrationObjects(), unowned)...)
	err := r.reconcileMigration(context.Background(), inst, runtimeCfg())
	require.Error(t, err)
	require.Contains(t, err.Error(), "not owned")
	require.False(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))
}

func TestReconcileMigration_NoCutoverBeforeValidation(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	job.Status.Active = 1
	require.NoError(t, r.Status().Update(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	require.True(t, IsConditionFalse(&inst.Status, ConditionTypeMigrationValidated))
	require.True(t, IsConditionFalse(&inst.Status, ConditionTypePraxisCutoverReady))
	require.Equal(t, ReasonPraxisCutoverNotReady, GetCondition(&inst.Status, ConditionTypePraxisCutoverReady).Reason)
	require.Equal(t, ogxiov1beta1.MigrationPhaseRunning, inst.Status.Migration.Phase)
}

func TestReconcileMigration_SoftRollbackWarning(t *testing.T) {
	inst := migrationInstance(true, true)
	r, rec := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	markJobComplete(job)
	require.NoError(t, r.Status().Update(context.Background(), job))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypePraxisCutoverReady))
	require.Contains(t, GetCondition(&inst.Status, ConditionTypeSoftRollbackAvailable).Message, "data loss")
	firstTransition := GetCondition(&inst.Status, ConditionTypeSoftRollbackAvailable).LastTransitionTime

	inst.Spec.PraxisMode.Enabled = ptr.To(false)
	inst.Spec.PraxisMode.MigrationJob = nil
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.True(t, IsConditionFalse(&inst.Status, ConditionTypePraxisCutoverReady))
	require.True(t, IsConditionTrue(&inst.Status, ConditionTypeSoftRollbackAvailable))
	require.Contains(t, inst.Status.Migration.SoftRollbackWarning, "ABAC")
	list := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), list))
	require.Empty(t, list.Items, "disable must delete the migration Job")

	for len(rec.Events) > 0 {
		evt := <-rec.Events
		require.NotContains(t, evt, ReasonSoftRollbackOnly, "soft-rollback warning is an admission warning, not an Event")
	}

	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.Equal(t, firstTransition, GetCondition(&inst.Status, ConditionTypeSoftRollbackAvailable).LastTransitionTime)
	for len(rec.Events) > 0 {
		evt := <-rec.Events
		require.NotContains(t, evt, ReasonSoftRollbackOnly)
	}
}

func TestReconcileMigration_SkippedForNonPraxis(t *testing.T) {
	inst := migrationInstance(false, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	list := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), list))
	require.Empty(t, list.Items)
	require.Nil(t, inst.Status.Migration)
	require.Nil(t, GetCondition(&inst.Status, ConditionTypeMigrationPreflightReady))
	require.Nil(t, GetCondition(&inst.Status, ConditionTypePraxisCutoverReady))
}

func TestReconcileMigration_SkippedWhenMigrationJobOmitted(t *testing.T) {
	inst := migrationInstance(true, false)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	list := &batchv1.JobList{}
	require.NoError(t, r.List(context.Background(), list))
	require.Empty(t, list.Items)
	require.Nil(t, inst.Status.Migration)
}

func TestReconcileMigration_UsesWorkloadServiceAccountAndResources(t *testing.T) {
	inst := migrationInstance(true, true)
	inst.Spec.Workload = &ogxiov1beta1.WorkloadSpec{
		Resources: &corev1.ResourceRequirements{
			Requests: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("128Mi"),
			},
		},
		Overrides: &ogxiov1beta1.WorkloadOverrides{ServiceAccountName: "custom-sa"},
	}
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	require.Equal(t, "custom-sa", job.Spec.Template.Spec.ServiceAccountName)
	require.Equal(t, resource.MustParse("100m"), job.Spec.Template.Spec.Containers[0].Resources.Requests[corev1.ResourceCPU])
}

func TestReconcileMigration_SecretContentChangeStartsNewAttempt(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t, migrationObjects()...)
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))

	job := &batchv1.Job{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	firstAttempt := job.Annotations[migrationAttemptAnnotation]

	sec := &corev1.Secret{}
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "praxis-pg", Namespace: "ogx"}, sec))
	sec.Data["url"] = []byte("postgres://praxis-rotated")
	require.NoError(t, r.Update(context.Background(), sec))

	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.NoError(t, r.reconcileMigration(context.Background(), inst, runtimeCfg()))
	require.NoError(t, r.Get(context.Background(), client.ObjectKey{Name: "demo-praxis-migration", Namespace: "ogx"}, job))
	require.NotEqual(t, firstAttempt, job.Annotations[migrationAttemptAnnotation])
}

func TestInstanceReferencesSecretIncludesMigrationTarget(t *testing.T) {
	inst := migrationInstance(true, true)
	r, _ := migrationReconciler(t)
	require.True(t, r.instanceReferencesSecret(inst, "praxis-pg", "ogx"))
	require.True(t, r.instanceReferencesSecret(inst, "pg-secret", "ogx"))
	require.False(t, r.instanceReferencesSecret(inst, "other", "ogx"))
	require.False(t, r.instanceReferencesSecret(inst, "praxis-pg", "other-ns"))
}

func TestSetConditionPreservesLastTransitionTimeWhenUnchanged(t *testing.T) {
	status := &ogxiov1beta1.OGXServerStatus{}
	first := metav1.Condition{
		Type:               ConditionTypeSoftRollbackAvailable,
		Status:             metav1.ConditionTrue,
		Reason:             ReasonSoftRollbackOnly,
		Message:            SoftRollbackWarningMessage,
		LastTransitionTime: metav1.NewTime(metav1.Now().UTC()),
	}
	SetCondition(status, first)
	original := GetCondition(status, ConditionTypeSoftRollbackAvailable).LastTransitionTime

	second := first
	second.LastTransitionTime = metav1.NewTime(original.Add(60e9))
	SetCondition(status, second)
	require.True(t, original.Equal(&GetCondition(status, ConditionTypeSoftRollbackAvailable).LastTransitionTime))
}
