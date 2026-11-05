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
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/deploy"
	pkgtls "github.com/ogx-ai/ogx-k8s-operator/pkg/tls"
	configv1 "github.com/openshift/api/config/v1"
	monitoringv1 "github.com/prometheus-operator/prometheus-operator/pkg/apis/monitoring/v1"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"
	"sigs.k8s.io/controller-runtime/pkg/webhook"

	_ "embed"
	// Import all Kubernetes client auth plugins (e.g. Azure, GCP, OIDC, etc.)
	// to ensure that exec-entrypoint and run can make use of them.
	_ "k8s.io/client-go/plugin/pkg/client/auth"
)

//go:embed distributions.json
var embeddedDistributions []byte

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() { //nolint:gochecknoinits
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))

	utilruntime.Must(ogxiov1beta1.AddToScheme(scheme))
	utilruntime.Must(configv1.Install(scheme))
	utilruntime.Must(monitoringv1.AddToScheme(scheme))
	//+kubebuilder:scaffold:scheme
}

func setupWebhook(mgr ctrl.Manager, clusterInfo *cluster.ClusterInfo) error {
	distNames := make([]string, 0, len(clusterInfo.DistributionImages))
	for name := range clusterInfo.DistributionImages {
		distNames = append(distNames, name)
	}
	return ogxiov1beta1.SetupWebhookWithManager(mgr, distNames)
}

func setupReconciler(ctx context.Context, setupClient client.Client, mgr ctrl.Manager, clusterInfo *cluster.ClusterInfo) error {
	operatorNamespace, err := deploy.GetOperatorNamespace()
	if err != nil {
		return fmt.Errorf("failed to get operator namespace: %w", err)
	}

	configMap, err := controllers.InitializeOperatorConfigMap(ctx, setupClient, operatorNamespace)
	if err != nil {
		return fmt.Errorf("failed to initialize operator config: %w", err)
	}

	imageMappingOverrides := controllers.ParseImageMappingOverrides(ctx, configMap.Data)

	reconciler := controllers.NewOGXServerReconciler(mgr.GetClient(), scheme, clusterInfo, imageMappingOverrides, operatorNamespace)
	if err = reconciler.SetupWithManager(ctx, mgr); err != nil {
		return fmt.Errorf("failed to create controller: %w", err)
	}
	return nil
}

func newCacheOptions() cache.Options {
	managedBySelector := labels.SelectorFromSet(labels.Set{
		"app.kubernetes.io/managed-by": "ogx-operator",
	})
	managedByFilter := cache.ByObject{Label: managedBySelector}

	return cache.Options{
		DefaultTransform: cache.TransformStripManagedFields(),
		ByObject: map[client.Object]cache.ByObject{
			&corev1.ConfigMap{}: {
				Label: labels.SelectorFromSet(labels.Set{
					controllers.WatchLabelKey: controllers.WatchLabelValue,
				}),
			},
			&corev1.Secret{}: {
				Label: labels.SelectorFromSet(labels.Set{
					controllers.WatchLabelKey: controllers.WatchLabelValue,
				}),
			},
			&appsv1.Deployment{}:                     managedByFilter,
			&policyv1.PodDisruptionBudget{}:          managedByFilter,
			&autoscalingv2.HorizontalPodAutoscaler{}: managedByFilter,
			&corev1.Service{}:                        managedByFilter,
			&batchv1.Job{}:                           managedByFilter,
			&networkingv1.NetworkPolicy{}:            managedByFilter,
			&networkingv1.Ingress{}:                  managedByFilter,
			&corev1.PersistentVolumeClaim{}:          managedByFilter,
		},
	}
}

func setupHealthChecks(mgr ctrl.Manager) error {
	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to set up health check: %w", err)
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return fmt.Errorf("failed to set up ready check: %w", err)
	}
	return nil
}

var errTLSProfileChanged = errors.New("TLS profile changed, restarting")

func main() {
	var metricsAddr string
	var enableLeaderElection bool
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":8080", "The address the metric endpoint binds to.")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "The address the probe endpoint binds to.")
	flag.BoolVar(&enableLeaderElection, "leader-elect", false,
		"Enable leader election for controller manager. "+
			"Enabling this will ensure there is only one active controller manager.")
	opts := zap.Options{
		Development:     false,
		StacktraceLevel: zapcore.PanicLevel, // Set higher than ErrorLevel to avoid stack traces in logs
	}
	opts.BindFlags(flag.CommandLine)
	flag.Parse()

	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	if err := run(metricsAddr, probeAddr, enableLeaderElection); err != nil {
		setupLog.Error(err, "failed to run manager")
		os.Exit(1)
	}
}

func setupComponents(ctx context.Context, cfg *restclient.Config, mgr ctrl.Manager) error {
	setupClient, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		return fmt.Errorf("failed to set up clients: %w", err)
	}

	clusterInfo, err := cluster.NewClusterInfo(ctx, setupClient, embeddedDistributions)
	if err != nil {
		return fmt.Errorf("failed to initialize cluster config: %w", err)
	}

	if err := cluster.PerformUpgradeCleanup(ctx, setupClient); err != nil {
		return fmt.Errorf("failed to perform upgrade cleanup: %w", err)
	}

	if err := setupWebhook(mgr, clusterInfo); err != nil {
		return fmt.Errorf("failed to set up webhook: %w", err)
	}

	if err := setupReconciler(ctx, setupClient, mgr, clusterInfo); err != nil {
		return fmt.Errorf("failed to set up reconciler: %w", err)
	}

	return setupHealthChecks(mgr)
}

func run(metricsAddr, probeAddr string, enableLeaderElection bool) error {
	cfg, err := ctrl.GetConfig()
	if err != nil {
		return fmt.Errorf("failed to get kubeconfig: %w", err)
	}

	tlsResult, err := pkgtls.Resolve(cfg, setupLog)
	if err != nil {
		return fmt.Errorf("failed to set up TLS: %w", err)
	}

	sigCtx := ctrl.SetupSignalHandler()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()
	ctx = logf.IntoContext(ctx, setupLog)

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                     scheme,
		Metrics:                    metricsserver.Options{BindAddress: metricsAddr, TLSOpts: tlsResult.TLSOpts},
		Cache:                      newCacheOptions(),
		HealthProbeBindAddress:     probeAddr,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "54e06e98.ogx.io",
		LeaderElectionResourceLock: "leases",
		LeaderElectionNamespace:    "",
		WebhookServer: webhook.NewServer(webhook.Options{
			TLSOpts: tlsResult.TLSOpts,
		}),
	})
	if err != nil {
		return fmt.Errorf("failed to start manager: %w", err)
	}

	if err := setupComponents(ctx, cfg, mgr); err != nil {
		return err
	}

	if err := pkgtls.SetupWatcher(mgr, tlsResult, cancel, setupLog); err != nil {
		return err
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		return err
	}
	if ctx.Err() != nil && tlsResult.HasOpenShiftConfigAPI {
		return errTLSProfileChanged
	}
	return nil
}
