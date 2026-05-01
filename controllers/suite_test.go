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

package controllers_test

//revive:disable:dot-imports
import (
	"os"
	"path/filepath"
	"testing"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// Test infrastructure for controller tests using testify.

var cfg *rest.Config
var k8sClient client.Client
var testEnv *envtest.Environment

// TestMain sets up the shared test environment for controller tests.
func TestMain(m *testing.M) {
	logf.SetLogger(zap.New(zap.UseDevMode(true)))

	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: os.Getenv("KUBEBUILDER_ASSETS"),
	}

	var err error
	cfg, err = testEnv.Start()
	if err != nil {
		logf.Log.Error(err, "failed to start test environment")
		os.Exit(1)
	}

	err = ogxiov1beta1.AddToScheme(scheme.Scheme)
	if err != nil {
		logf.Log.Error(err, "failed to add ogxiov1beta1 scheme")
		os.Exit(1)
	}

	err = corev1.AddToScheme(scheme.Scheme)
	if err != nil {
		logf.Log.Error(err, "failed to add corev1 scheme")
		os.Exit(1)
	}

	err = appsv1.AddToScheme(scheme.Scheme)
	if err != nil {
		logf.Log.Error(err, "failed to add appsv1 scheme")
		os.Exit(1)
	}

	err = networkingv1.AddToScheme(scheme.Scheme)
	if err != nil {
		logf.Log.Error(err, "failed to add networkingv1 scheme")
		os.Exit(1)
	}

	err = rbacv1.AddToScheme(scheme.Scheme)
	if err != nil {
		logf.Log.Error(err, "failed to add rbacv1 scheme")
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
