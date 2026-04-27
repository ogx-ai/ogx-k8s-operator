package cluster

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/go-logr/logr"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/deploy"
	rbacv1 "k8s.io/api/rbac/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

type ClusterInfo struct {
	OperatorNamespace  string
	DistributionImages map[string]string
}

// NewClusterInfo creates a new ClusterInfo object using embedded distributions data.
func NewClusterInfo(ctx context.Context, client client.Client, embeddedDistributions []byte) (*ClusterInfo, error) {
	operatorNamespace, err := deploy.GetOperatorNamespace()
	if err != nil {
		return nil, fmt.Errorf("failed to find operator namespace: %w", err)
	}

	var distributionImages map[string]string
	if err := json.Unmarshal(embeddedDistributions, &distributionImages); err != nil {
		return nil, fmt.Errorf("failed to parse embedded distributions JSON: %w", err)
	}

	return &ClusterInfo{
		OperatorNamespace:  operatorNamespace,
		DistributionImages: distributionImages,
	}, nil
}

// PerformUpgradeCleanup performs one-time cleanup operations for seamless upgrades.
func PerformUpgradeCleanup(ctx context.Context, client client.Client) error {
	logger := log.FromContext(ctx).WithName("upgrade-cleanup")
	logger.Info("Starting upgrade cleanup operations")

	// Cleanup legacy ClusterRoleBindings from cluster-scoped to namespace-scoped migration
	cleanupLegacyClusterRoleBindings(ctx, client, logger)
	logger.Info("Upgrade cleanup operations completed successfully")
	return nil
}

// cleanupLegacyClusterRoleBindings removes ClusterRoleBindings from previous operator versions.
func cleanupLegacyClusterRoleBindings(ctx context.Context, client client.Client, logger logr.Logger) {
	// List all ClusterRoleBindings
	clusterRoleBindingList := &rbacv1.ClusterRoleBindingList{}
	if err := client.List(ctx, clusterRoleBindingList); err != nil {
		logger.V(1).Info("Unable to list ClusterRoleBindings for cleanup, skipping legacy cleanup", "error", err)
		return
	}

	var crbsToDelete []*rbacv1.ClusterRoleBinding
	for i := range clusterRoleBindingList.Items {
		crb := &clusterRoleBindingList.Items[i]

		if shouldDeleteLegacyClusterRoleBinding(crb) {
			crbsToDelete = append(crbsToDelete, crb)
		}
	}

	// Delete the identified ClusterRoleBindings
	for _, crb := range crbsToDelete {
		logger.Info("Cleaning up legacy ClusterRoleBinding from previous operator version",
			"clusterRoleBinding", crb.Name)

		if err := client.Delete(ctx, crb); err != nil {
			if k8serrors.IsNotFound(err) {
				// Already deleted, continue
				continue
			}
			// Log the error but don't fail startup - the cleanup is best-effort
			logger.Error(err, "Failed to delete legacy ClusterRoleBinding, continuing with startup",
				"clusterRoleBinding", crb.Name)
		}
	}

	if len(crbsToDelete) > 0 {
		logger.Info("Successfully cleaned up legacy ClusterRoleBindings", "count", len(crbsToDelete))
	}
}

// shouldDeleteLegacyClusterRoleBinding determines if a ClusterRoleBinding should be deleted.
func shouldDeleteLegacyClusterRoleBinding(crb *rbacv1.ClusterRoleBinding) bool {
	// Only delete ClusterRoleBindings that were created by our operator
	if managedBy, exists := crb.Labels["app.kubernetes.io/managed-by"]; !exists || managedBy != "llama-stack-operator" {
		return false
	}

	// Check if any subjects are ServiceAccounts in namespaces (namespace-scoped)
	for _, subject := range crb.Subjects {
		if subject.Kind == "ServiceAccount" && subject.Namespace != "" {
			return true
		}
	}

	return false
}
