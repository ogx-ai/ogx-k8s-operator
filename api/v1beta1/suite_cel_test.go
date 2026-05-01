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

package v1beta1

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

var (
	cfg         *rest.Config
	k8sClient   client.Client
	testEnv     *envtest.Environment
	nameCounter int64
)

func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		logf.Log.Error(err, "failed to start test environment")
		os.Exit(1)
	}

	err = AddToScheme(scheme.Scheme)
	if err != nil {
		logf.Log.Error(err, "failed to add v1beta1 scheme")
		os.Exit(1)
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		logf.Log.Error(err, "failed to create client")
		os.Exit(1)
	}

	code := m.Run()

	err = testEnv.Stop()
	if err != nil {
		logf.Log.Error(err, "failed to stop test environment")
		os.Exit(1)
	}

	os.Exit(code)
}

func uniqueName() string {
	return fmt.Sprintf("cel-%d", atomic.AddInt64(&nameCounter, 1))
}

func createCELTestNamespace(t *testing.T, prefix string) string {
	t.Helper()
	name := fmt.Sprintf("%s-%d", prefix, atomic.AddInt64(&nameCounter, 1))
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{Name: name},
	}
	if err := k8sClient.Create(context.Background(), ns); err != nil {
		t.Fatalf("failed to create test namespace %q: %v", name, err)
	}
	t.Cleanup(func() {
		_ = k8sClient.Delete(context.Background(), ns)
	})
	return name
}

func validOGXServer(name, namespace string) *OGXServer {
	return &OGXServer{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Spec: OGXServerSpec{
			Distribution: DistributionSpec{
				Image: "test:latest",
			},
		},
	}
}

func ptr[T any](v T) *T {
	return &v
}

func requireCELError(t *testing.T, err error, expectedMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected CEL validation error, got nil")
	}
	if !apierrors.IsInvalid(err) {
		t.Fatalf("expected StatusReasonInvalid, got: %v", err)
	}
	statusErr, ok := err.(*apierrors.StatusError) //nolint:errorlint // envtest errors are never wrapped
	if !ok {
		t.Fatalf("expected *StatusError, got %T", err)
	}
	causes := statusErr.Status().Details.Causes
	for _, c := range causes {
		if strings.Contains(c.Message, expectedMsg) {
			return
		}
	}
	t.Errorf("no cause contained %q; causes: %v", expectedMsg, causes)
}

func requireAPIError(t *testing.T, err error, expectedMsg string) {
	t.Helper()
	if err == nil {
		t.Fatal("expected API validation error, got nil")
	}
	if !strings.Contains(err.Error(), expectedMsg) {
		t.Errorf("error %q does not contain %q", err.Error(), expectedMsg)
	}
}

func validUnstructuredOGXServer(t *testing.T, name, namespace string) map[string]any {
	t.Helper()
	obj := validOGXServer(name, namespace)
	data, err := json.Marshal(obj)
	if err != nil {
		t.Fatalf("failed to marshal OGXServer: %v", err)
	}
	var raw map[string]any
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatalf("failed to unmarshal to map: %v", err)
	}
	raw["apiVersion"] = "ogx.io/v1beta1"
	raw["kind"] = "OGXServer"
	return raw
}

func createUnstructured(t *testing.T, raw map[string]any) error {
	t.Helper()
	u := &unstructured.Unstructured{Object: raw}
	return k8sClient.Create(context.Background(), u)
}

func setNestedField(obj map[string]any, value any, fields ...string) {
	m := obj
	for _, f := range fields[:len(fields)-1] {
		next, ok := m[f].(map[string]any)
		if !ok {
			next = make(map[string]any)
			m[f] = next
		}
		m = next
	}
	m[fields[len(fields)-1]] = value
}
