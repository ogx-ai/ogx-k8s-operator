package deploy

import (
	"context"
	"fmt"

	"github.com/go-logr/logr"
	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	networkingv1 "k8s.io/api/networking/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

// ApplyNetworkPolicy creates or updates a NetworkPolicy.
func ApplyNetworkPolicy(ctx context.Context, c client.Client, scheme *runtime.Scheme,
	instance *ogxiov1beta1.OGXServer, networkPolicy *networkingv1.NetworkPolicy, log logr.Logger) error {
	// Set the controller reference
	if err := ctrl.SetControllerReference(instance, networkPolicy, scheme); err != nil {
		return fmt.Errorf("failed to set controller reference: %w", err)
	}

	// Check if the NetworkPolicy already exists
	existing := &networkingv1.NetworkPolicy{}
	err := c.Get(ctx, client.ObjectKeyFromObject(networkPolicy), existing)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			// Create the NetworkPolicy if it doesn't exist
			if err = c.Create(ctx, networkPolicy); err != nil {
				return fmt.Errorf("failed to create NetworkPolicy: %w", err)
			}
			log.Info("Created NetworkPolicy", "name", networkPolicy.Name)
			return nil
		}
		return fmt.Errorf("failed to get NetworkPolicy: %w", err)
	}

	// Update the NetworkPolicy if it exists
	networkPolicy.ResourceVersion = existing.ResourceVersion
	if err := c.Update(ctx, networkPolicy); err != nil {
		return fmt.Errorf("failed to update NetworkPolicy: %w", err)
	}
	log.Info("Updated NetworkPolicy", "name", networkPolicy.Name)
	return nil
}

// HandleDisabledNetworkPolicy handles the deletion of a NetworkPolicy when the feature is disabled.
// It checks if the NetworkPolicy exists and deletes it if found.
func HandleDisabledNetworkPolicy(ctx context.Context, c client.Client, networkPolicy *networkingv1.NetworkPolicy, log logr.Logger) error {
	log.Info("NetworkPolicy creation is disabled, checking if deletion is needed")
	existingPolicy := &networkingv1.NetworkPolicy{}
	err := c.Get(ctx, client.ObjectKeyFromObject(networkPolicy), existingPolicy)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			log.Info("NetworkPolicy does not exist, nothing to delete", "name", networkPolicy.Name)
			return nil // NetworkPolicy doesn't exist, nothing to do
		}
		return fmt.Errorf("failed to check NetworkPolicy existence: %w", err)
	}

	// NetworkPolicy exists, proceed with deletion
	if err := c.Delete(ctx, existingPolicy); err != nil {
		return fmt.Errorf("failed to delete NetworkPolicy: %w", err)
	}
	log.Info("Deleted NetworkPolicy", "name", networkPolicy.Name)
	return nil
}
