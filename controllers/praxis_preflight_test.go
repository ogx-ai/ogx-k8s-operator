package controllers

import (
	"context"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func praxisPreflightScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	require.NoError(t, corev1.AddToScheme(scheme))
	require.NoError(t, ogxiov1beta1.AddToScheme(scheme))
	return scheme
}

// praxisPod builds a Running pod with the given labels. When ready it has a True Ready condition;
// otherwise a False Ready condition.
func praxisPod(namespace string, labels map[string]string, ready bool) *corev1.Pod {
	readyStatus := corev1.ConditionFalse
	if ready {
		readyStatus = corev1.ConditionTrue
	}
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{Name: "praxis-pod", Namespace: namespace, Labels: labels},
		Status: corev1.PodStatus{
			Phase:      corev1.PodRunning,
			Conditions: []corev1.PodCondition{{Type: corev1.PodReady, Status: readyStatus}},
		},
	}
}

func newPraxisReconciler(t *testing.T, objs ...client.Object) *OGXServerReconciler {
	t.Helper()
	scheme := praxisPreflightScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithObjects(objs...).Build()
	return &OGXServerReconciler{Client: c, Scheme: scheme}
}

// praxisTestNamespace and praxisTestLabels identify the Praxis instance targeted by the selector
// in the reachability tests.
const praxisTestNamespace = "praxis-ns"

func praxisTestLabels() map[string]string { return map[string]string{"app": "praxis"} }

// instanceWithPraxisSelector returns an OGXServer whose praxisSelector targets praxisTestNamespace
// and praxisTestLabels.
func instanceWithPraxisSelector() *ogxiov1beta1.OGXServer {
	return &ogxiov1beta1.OGXServer{
		ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ogx"},
		Spec: ogxiov1beta1.OGXServerSpec{
			PraxisMode: &ogxiov1beta1.PraxisModeSpec{
				PraxisSelector: &ogxiov1beta1.PraxisSelector{
					Namespace:   praxisTestNamespace,
					PodSelector: metav1.LabelSelector{MatchLabels: praxisTestLabels()},
				},
			},
		},
	}
}

func TestUpdatePraxisReachableCondition(t *testing.T) {
	defaultLabels := map[string]string{ogxiov1beta1.DefaultLabelKey: ogxiov1beta1.DefaultPraxisPodLabelValue}

	tests := []struct {
		name       string
		pods       []client.Object
		instance   *ogxiov1beta1.OGXServer
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name:       "selector matches a ready pod",
			pods:       []client.Object{praxisPod(praxisTestNamespace, praxisTestLabels(), true)},
			instance:   instanceWithPraxisSelector(),
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonPraxisReachable,
		},
		{
			name:       "selector matches only a not-ready pod",
			pods:       []client.Object{praxisPod(praxisTestNamespace, praxisTestLabels(), false)},
			instance:   instanceWithPraxisSelector(),
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonPraxisUnreachable,
		},
		{
			name:       "no pods match the selector",
			pods:       nil,
			instance:   instanceWithPraxisSelector(),
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonPraxisUnreachable,
		},
		{
			name:       "ready pod in a different namespace does not count",
			pods:       []client.Object{praxisPod("other-ns", praxisTestLabels(), true)},
			instance:   instanceWithPraxisSelector(),
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonPraxisUnreachable,
		},
		{
			name:       "default selector used when praxisSelector unset",
			pods:       []client.Object{praxisPod(ogxiov1beta1.DefaultPraxisNamespace, defaultLabels, true)},
			instance:   &ogxiov1beta1.OGXServer{ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ogx"}},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonPraxisReachable,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newPraxisReconciler(t, tt.pods...)
			r.updatePraxisReachableCondition(context.Background(), tt.instance)

			cond := GetCondition(&tt.instance.Status, ConditionTypePraxisReachable)
			require.NotNil(t, cond, "expected PraxisReachable condition to be set")
			require.Equal(t, tt.wantStatus, cond.Status)
			require.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}

func TestUpdateTLSConfiguredCondition(t *testing.T) {
	tlsSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "tls-secret", Namespace: "ogx"},
		Type:       corev1.SecretTypeTLS,
	}

	tests := []struct {
		name       string
		objs       []client.Object
		instance   *ogxiov1beta1.OGXServer
		wantStatus metav1.ConditionStatus
		wantReason string
	}{
		{
			name: "tls not configured",
			instance: &ogxiov1beta1.OGXServer{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ogx"},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonTLSNotConfigured,
		},
		{
			name: "tls secretName set but secret missing",
			instance: &ogxiov1beta1.OGXServer{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ogx"},
				Spec: ogxiov1beta1.OGXServerSpec{
					Network: &ogxiov1beta1.NetworkSpec{TLS: &ogxiov1beta1.TLSSpec{SecretName: "tls-secret"}},
				},
			},
			wantStatus: metav1.ConditionFalse,
			wantReason: ReasonTLSSecretMissing,
		},
		{
			name: "tls secretName set and secret exists",
			objs: []client.Object{tlsSecret},
			instance: &ogxiov1beta1.OGXServer{
				ObjectMeta: metav1.ObjectMeta{Name: "test", Namespace: "ogx"},
				Spec: ogxiov1beta1.OGXServerSpec{
					Network: &ogxiov1beta1.NetworkSpec{TLS: &ogxiov1beta1.TLSSpec{SecretName: "tls-secret"}},
				},
			},
			wantStatus: metav1.ConditionTrue,
			wantReason: ReasonTLSConfigured,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newPraxisReconciler(t, tt.objs...)
			r.updateTLSConfiguredCondition(context.Background(), tt.instance)

			cond := GetCondition(&tt.instance.Status, ConditionTypeTLSConfigured)
			require.NotNil(t, cond, "expected TLSConfigured condition to be set")
			require.Equal(t, tt.wantStatus, cond.Status)
			require.Equal(t, tt.wantReason, cond.Reason)
		})
	}
}
