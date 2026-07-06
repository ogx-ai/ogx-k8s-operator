//nolint:testpackage
package e2e

import (
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
)

func TestE2E(t *testing.T) {
	registerSchemes()
	// Run validation tests
	t.Run("validation", TestValidationSuite)

	// Run combined creation and deletion tests for multiple distributions
	distributions := []string{"starter"}
	for _, dist := range distributions {
		t.Run("creation-deletion-"+dist, func(t *testing.T) {
			t.Logf("Testing distribution: %s", dist)
			runCreationDeletionSuiteForDistribution(t, dist)
		})
	}

	// Run TLS tests
	t.Run("tls", func(t *testing.T) {
		TestTLSSuite(t)
	})
}

// runCreationDeletionSuiteForDistribution runs creation tests followed by deletion tests for a specific distribution.
func runCreationDeletionSuiteForDistribution(t *testing.T, distType string) {
	t.Helper()
	if TestOpts.SkipCreation {
		t.Skip("Skipping creation-deletion test suite")
	}

	var creationFailed bool
	var createdServer *ogxiov1beta1.OGXServer

	t.Run("creation", func(t *testing.T) {
		createdServer = runCreationTestsForDistribution(t, distType)
		creationFailed = t.Failed()
	})

	if !creationFailed && !TestOpts.SkipDeletion && createdServer != nil {
		t.Run("deletion", func(t *testing.T) {
			runDeletionTests(t, createdServer)
		})
	} else {
		if TestOpts.SkipDeletion {
			t.Log("Skipping deletion tests (SkipDeletion=true)")
		} else {
			t.Log("Skipping deletion tests due to creation test failures")
		}
	}
}
