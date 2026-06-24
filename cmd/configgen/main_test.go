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

package main

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/config"
	"github.com/stretchr/testify/require"
	"gopkg.in/yaml.v3"
)

// TestRun discovers test cases from testdata/ subdirectories. Each subdirectory
// must contain a cr.yaml. Optional files:
//   - base.yaml:              explicit base config (omit to use standard config resolution)
//   - want-err:              if present, expect error containing this text
//   - want-config.yaml:      if present, assert generated config matches structurally
//
// Validation uses testdata/distributions.json by default. A testcase-local
// distributions.json can override it when a fixture needs a custom registry.
//
// Set KUBEBUILDER_ASSETS to enable CRD/CEL and webhook validation for each case.
func TestRun(t *testing.T) {
	entries, err := os.ReadDir("testdata")
	if err != nil {
		t.Fatalf("failed to read testdata: %v", err)
	}

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		t.Run(name, func(t *testing.T) {
			tc := loadTestCase(t, filepath.Join("testdata", name))
			generated, runErr := run(tc.opts)
			tc.assert(t, generated, runErr)
		})
	}
}

type testCase struct {
	opts       options
	wantErr    string
	wantConfig string
}

func loadTestCase(t *testing.T, dir string) testCase {
	t.Helper()

	crPath := filepath.Join(dir, "cr.yaml")
	if !fileExists(crPath) {
		t.Fatalf("missing cr.yaml in %s", dir)
	}

	var basePath string
	if p := filepath.Join(dir, "base.yaml"); fileExists(p) {
		basePath = p
	}
	distributionsPath := filepath.Join("testdata", "distributions.json")
	if p := filepath.Join(dir, "distributions.json"); fileExists(p) {
		distributionsPath = p
	}

	return testCase{
		opts: options{
			crPath:            crPath,
			basePath:          basePath,
			crdPath:           "../../config/crd/bases",
			distributionsPath: distributionsPath,
			validate:          os.Getenv("KUBEBUILDER_ASSETS") != "",
		},
		wantErr:    strings.TrimSpace(readOptionalFile(t, filepath.Join(dir, "want-err"))),
		wantConfig: readOptionalFile(t, filepath.Join(dir, "want-config.yaml")),
	}
}

func TestLoadKnownDistributionNames(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "distributions.json")
	require.NoError(t, os.WriteFile(path, []byte(`{"starter":"img-a","postgres-demo":"img-b"}`), 0o600))

	server := &ogxiov1beta1.OGXServer{
		Spec: ogxiov1beta1.OGXServerSpec{
			Distribution: ogxiov1beta1.DistributionSpec{Name: "starter"},
		},
	}

	names, err := loadKnownDistributionNames(server, path)
	require.NoError(t, err)
	require.Equal(t, []string{"postgres-demo", "starter"}, names)
}

func (tc testCase) assert(t *testing.T, generated *config.GeneratedConfig, runErr error) {
	t.Helper()

	if tc.wantErr != "" {
		if runErr == nil {
			t.Fatal("expected error, got nil")
		}
		if !strings.Contains(runErr.Error(), tc.wantErr) {
			t.Fatalf("error %q does not contain %q", runErr, tc.wantErr)
		}
		return
	}
	if runErr != nil {
		t.Fatalf("unexpected error: %v", runErr)
	}
	if tc.wantConfig != "" {
		assertYAMLEqual(t, tc.wantConfig, generated.ConfigYAML)
	}
}

func assertYAMLEqual(t *testing.T, want, got string) {
	t.Helper()
	var wantObj, gotObj any
	if err := yaml.Unmarshal([]byte(want), &wantObj); err != nil {
		t.Fatalf("failed to parse want-config.yaml: %v", err)
	}
	if err := yaml.Unmarshal([]byte(got), &gotObj); err != nil {
		t.Fatalf("failed to parse generated config: %v", err)
	}
	if !reflect.DeepEqual(wantObj, gotObj) {
		t.Errorf("config mismatch:\n--- want\n%s\n--- got\n%s", want, got)
	}
}

func fileExists(path string) bool {
	_, err := os.Stat(path)
	return err == nil
}

func readOptionalFile(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return string(data)
}
