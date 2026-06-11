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
	"crypto/tls"
	"flag"
	"fmt"
	"os"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/ogx-ai/ogx-k8s-operator/controllers"
	"github.com/ogx-ai/ogx-k8s-operator/pkg/cluster"
	configv1 "github.com/openshift/api/config/v1"
	tlspkg "github.com/openshift/controller-runtime-common/pkg/tls"
	"go.uber.org/zap/zapcore"
	appsv1 "k8s.io/api/apps/v1"
	autoscalingv2 "k8s.io/api/autoscaling/v2"
	corev1 "k8s.io/api/core/v1"
	networkingv1 "k8s.io/api/networking/v1"
	policyv1 "k8s.io/api/policy/v1"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/cache"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/config"
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
	//+kubebuilder:scaffold:scheme
}

func setupWebhook(mgr ctrl.Manager, clusterInfo *cluster.ClusterInfo) error {
	distNames := make([]string, 0, len(clusterInfo.DistributionImages))
	for name := range clusterInfo.DistributionImages {
		distNames = append(distNames, name)
	}
	return ogxiov1beta1.SetupWebhookWithManager(mgr, distNames)
}

func setupReconciler(ctx context.Context, cli client.Client, mgr ctrl.Manager, clusterInfo *cluster.ClusterInfo, directClient client.Reader) error {
	reconciler, err := controllers.NewOGXServerReconciler(ctx, cli, scheme, clusterInfo, directClient)
	if err != nil {
		return fmt.Errorf("failed to create reconciler: %w", err)
	}
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
			&appsv1.Deployment{}:                     managedByFilter,
			&policyv1.PodDisruptionBudget{}:          managedByFilter,
			&autoscalingv2.HorizontalPodAutoscaler{}: managedByFilter,
			&corev1.Service{}:                        managedByFilter,
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

type tlsSetupResult struct {
	tlsOpts               []func(*tls.Config)
	profile               configv1.TLSProfileSpec
	hasOpenShiftConfigAPI bool
}

func setupTLS() (tlsSetupResult, error) {
	var result tlsSetupResult
	bootstrapCtx, bootstrapCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer bootstrapCancel()
	bootstrapClient, err := client.New(ctrl.GetConfigOrDie(), client.Options{Scheme: scheme})
	if err != nil {
		return result, fmt.Errorf("unable to create bootstrap client for TLS profile: %w", err)
	}
	result.profile, err = tlspkg.FetchAPIServerTLSProfile(bootstrapCtx, bootstrapClient)
	if err != nil {
		if apimeta.IsNoMatchError(err) {
			setupLog.Info("TLS profile not available, using hardened defaults (non-OpenShift cluster)")
			result.tlsOpts = append(result.tlsOpts, func(c *tls.Config) {
				c.MinVersion = tls.VersionTLS12
			})
		} else {
			return result, fmt.Errorf("unable to read APIServer TLS profile: %w", err)
		}
	} else {
		result.hasOpenShiftConfigAPI = true
		tlsConfigFn, unsupportedCiphers := tlspkg.NewTLSConfigFromProfile(result.profile)
		if len(unsupportedCiphers) > 0 {
			setupLog.Info("some ciphers from TLS profile are not supported by Go", "unsupported", unsupportedCiphers)
		}
		result.tlsOpts = append(result.tlsOpts, tlsConfigFn)
	}
	result.tlsOpts = append(result.tlsOpts, func(c *tls.Config) {
		c.NextProtos = []string{"h2", "http/1.1"}
	})
	return result, nil
}

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

	tlsResult, err := setupTLS()
	if err != nil {
		setupLog.Error(err, "failed to set up TLS")
		os.Exit(1)
	}

	// root context
	sigCtx := ctrl.SetupSignalHandler()
	ctx, cancel := context.WithCancel(sigCtx)
	defer cancel()
	ctx = logf.IntoContext(ctx, setupLog)

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                     scheme,
		Metrics:                    metricsserver.Options{BindAddress: metricsAddr, TLSOpts: tlsResult.tlsOpts},
		Cache:                      newCacheOptions(),
		HealthProbeBindAddress:     probeAddr,
		LeaderElection:             enableLeaderElection,
		LeaderElectionID:           "54e06e98.ogx.io",
		LeaderElectionResourceLock: "leases",
		LeaderElectionNamespace:    "",
		WebhookServer: webhook.NewServer(webhook.Options{
			TLSOpts: tlsResult.tlsOpts,
		}),
		// LeaderElectionReleaseOnCancel defines if the leader should step down voluntarily
		// when the Manager ends. This requires the binary to immediately end when the
		// Manager is stopped, otherwise, this setting is unsafe. Setting this significantly
		// speeds up voluntary leader transitions as the new leader don't have to wait
		// LeaseDuration time first.
		//
		// In the default scaffold provided, the program ends immediately after
		// the manager stops, so would be fine to enable this option. However,
		// if you are doing or is intended to do any operation such as perform cleanups
		// after the manager stops then its usage might be unsafe.
		// LeaderElectionReleaseOnCancel: true,
	})
	if err != nil {
		setupLog.Error(err, "failed to start manager")
		os.Exit(1)
	}

	cfg, err := config.GetConfig()
	if err != nil {
		setupLog.Error(err, "failed to get config for setup")
		os.Exit(1)
	}

	setupClient, err := client.New(cfg, client.Options{
		Scheme: scheme,
	})
	if err != nil {
		setupLog.Error(err, "failed to set up clients")
		os.Exit(1)
	}

	clusterInfo, err := cluster.NewClusterInfo(ctx, setupClient, embeddedDistributions)
	if err != nil {
		setupLog.Error(err, "failed to initialize cluster config")
		os.Exit(1)
	}

	// Perform one-time upgrade cleanup operations
	if err := cluster.PerformUpgradeCleanup(ctx, setupClient); err != nil {
		setupLog.Error(err, "failed to perform upgrade cleanup")
		os.Exit(1)
	}

	if err := setupWebhook(mgr, clusterInfo); err != nil {
		setupLog.Error(err, "failed to set up webhook")
		os.Exit(1)
	}

	if err := setupReconciler(ctx, setupClient, mgr, clusterInfo, setupClient); err != nil {
		setupLog.Error(err, "failed to set up reconciler")
		os.Exit(1)
	}

	if err := setupHealthChecks(mgr); err != nil {
		setupLog.Error(err, "failed to set up health checks")
		os.Exit(1)
	}

	// Register SecurityProfileWatcher on OpenShift: cancel context on TLS profile change so pod restarts
	if tlsResult.hasOpenShiftConfigAPI {
		watcher := &tlspkg.SecurityProfileWatcher{
			Client:                mgr.GetClient(),
			InitialTLSProfileSpec: tlsResult.profile,
			OnProfileChange: func(_ context.Context, _, _ configv1.TLSProfileSpec) {
				setupLog.Info("TLS profile changed, initiating graceful shutdown to reload")
				cancel()
			},
		}
		if err := watcher.SetupWithManager(mgr); err != nil {
			setupLog.Error(err, "unable to register TLS security profile watcher")
			os.Exit(1)
		}
	}

	setupLog.Info("starting manager")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "failed to run manager")
		os.Exit(1)
	}
}
