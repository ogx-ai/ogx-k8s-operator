//nolint:testpackage
package e2e

import (
	"fmt"
)

// TestOptions defines the configuration for running tests.
type TestOptions struct {
	SkipValidation bool
	SkipCreation   bool
	SkipDeletion   bool
	OperatorNS     string
}

// TestOpts is the global test options instance.
var TestOpts = NewTestOptions()

// NewTestOptions creates a new TestOptions instance with default values.
func NewTestOptions() *TestOptions {
	return &TestOptions{
		SkipValidation: false,
		SkipCreation:   false,
		SkipDeletion:   false,
		OperatorNS:     "ogx-k8s-operator-system",
	}
}

// String returns a string representation of the test options.
func (o *TestOptions) String() string {
	return fmt.Sprintf("SkipValidation: %v, SkipCreation: %v, SkipDeletion: %v, OperatorNS: %s",
		o.SkipValidation, o.SkipCreation, o.SkipDeletion, o.OperatorNS)
}
