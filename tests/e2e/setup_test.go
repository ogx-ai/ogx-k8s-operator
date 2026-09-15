//nolint:testpackage
package e2e

import (
	"os"
	"testing"
)

var (
	TestEnv *TestEnvironment
)

func TestMain(m *testing.M) {
	// Register schemes before building the clients so individual suites can be run standalone
	// with -run, not only through TestE2E.
	registerSchemes()

	// Set up test environment
	var err error
	TestEnv, err = SetupTestEnv()
	if err != nil {
		os.Exit(1)
	}

	// Run tests
	code := m.Run()

	// Clean up test environment
	CleanupTestEnv(TestEnv)

	os.Exit(code)
}
