package compare

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/google/go-cmp/cmp"
	"github.com/google/go-cmp/cmp/cmpopts"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// HasUnexpectedServiceChanges checks for any unexpected mutations between two Service objects.
// It returns true and a diff string if a change is found in any field that is not
// explicitly managed by the operator or the cluster.
func HasUnexpectedServiceChanges(desired, current *corev1.Service) (bool, string) {
	// Ignore fields that we are intentionally managing and expect to be different.
	managedSpecFields := cmpopts.IgnoreFields(corev1.ServiceSpec{}, "Ports", "Selector")

	// Ignore metadata fields that are managed by the Kubernetes API server.
	// Comparing these would cause unnecessary diffs on every update.
	clusterManagedFields := cmpopts.IgnoreFields(metav1.ObjectMeta{}, "ResourceVersion", "UID", "CreationTimestamp", "Generation", "ManagedFields")

	// Ignore the status field, as it is managed by the Kubernetes service controller,
	// not by our operator.
	ignoreStatus := cmpopts.IgnoreFields(corev1.Service{}, "Status")

	diff := cmp.Diff(current, desired, managedSpecFields, clusterManagedFields, ignoreStatus)
	if diff != "" {
		return true, diff
	}

	return false, ""
}

// CheckAndLogServiceChanges encapsulates the full logic to fetch the current service,
// compare it against the desired state, and log any unexpected changes.
func CheckAndLogServiceChanges(ctx context.Context, c client.Client, desired *unstructured.Unstructured) error {
	logger := logr.FromContextOrDiscard(ctx)
	key := client.ObjectKeyFromObject(desired)
	foundService := &corev1.Service{}

	err := c.Get(ctx, key, foundService)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil
		}
		return fmt.Errorf("failed to fetch existing service for comparison: %w", err)
	}

	desiredService := &corev1.Service{}
	if err := runtime.DefaultUnstructuredConverter.FromUnstructured(desired.UnstructuredContent(), desiredService); err != nil {
		return fmt.Errorf("failed to convert desired unstructured object to Service: %w", err)
	}

	changed, unexpectedChanges := HasUnexpectedServiceChanges(desiredService, foundService)
	if changed {
		logger.V(1).Info("Ignoring unexpected changes in Service manifest", "changes", unexpectedChanges)
	}

	return nil
}
