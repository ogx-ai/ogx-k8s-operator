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

package tls

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/go-logr/logr"
	configv1 "github.com/openshift/api/config/v1"
	tlspkg "github.com/openshift/controller-runtime-common/pkg/tls"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	apimeta "k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/runtime"
	restclient "k8s.io/client-go/rest"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/manager"
)

// bootstrapTimeout bounds the startup profile read.
const bootstrapTimeout = 10 * time.Second

func isTransientReadError(err error) bool {
	var netErr net.Error
	return errors.Is(err, context.DeadlineExceeded) ||
		apierrors.IsServiceUnavailable(err) ||
		apierrors.IsTimeout(err) ||
		apierrors.IsServerTimeout(err) ||
		apierrors.IsTooManyRequests(err) ||
		(errors.As(err, &netErr) && netErr.Timeout())
}

// Result holds the TLS configuration resolved from the cluster profile.
type Result struct {
	TLSOpts               []func(*tls.Config)
	Profile               configv1.TLSProfileSpec
	HasOpenShiftConfigAPI bool
}

// Resolve reads the cluster TLS profile through a bootstrap client bounded by
// bootstrapTimeout and returns TLS options for controller-runtime servers.
func Resolve(cfg *restclient.Config, logger logr.Logger) (*Result, error) {
	// Bound the bootstrap client's requests, including REST mapper discovery,
	// which the caller's context does not cover.
	bootstrapCfg := restclient.CopyConfig(cfg)
	bootstrapCfg.Timeout = bootstrapTimeout
	scheme := runtime.NewScheme()
	if err := configv1.Install(scheme); err != nil {
		return nil, fmt.Errorf("failed to register OpenShift config API scheme: %w", err)
	}
	bootstrapClient, err := client.New(bootstrapCfg, client.Options{Scheme: scheme})
	if err != nil {
		return nil, fmt.Errorf("failed to create bootstrap client for TLS profile: %w", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), bootstrapTimeout)
	defer cancel()

	result := &Result{}
	profile, err := tlspkg.FetchAPIServerTLSProfile(ctx, bootstrapClient)
	if err != nil {
		switch {
		case apimeta.IsNoMatchError(err):
			logger.Info("config.openshift.io API not available (non-OpenShift cluster), using Intermediate TLS profile as fallback")
		case apierrors.IsNotFound(err):
			logger.Info("APIServer resource not found, using Intermediate TLS profile as fallback")
		case isTransientReadError(err):
			// The config API exists, so keep the watcher registered to reload the real profile.
			result.HasOpenShiftConfigAPI = true
			logger.Info("Transient API error reading TLS profile, using Intermediate TLS profile as fallback", "error", err)
		default:
			return nil, fmt.Errorf("failed to read APIServer TLS profile: %w", err)
		}
		fallbackProfile := *configv1.TLSProfiles[configv1.TLSProfileIntermediateType]
		fallbackFn, _ := tlspkg.NewTLSConfigFromProfile(fallbackProfile)
		result.TLSOpts = append(result.TLSOpts, fallbackFn)
		result.Profile = fallbackProfile
	} else {
		result.HasOpenShiftConfigAPI = true
		result.Profile = profile
		profileOpts, err := buildProfileTLSOpts(profile, logger)
		if err != nil {
			return nil, fmt.Errorf("failed to configure TLS: %w", err)
		}
		result.TLSOpts = append(result.TLSOpts, profileOpts...)
	}
	result.TLSOpts = append(result.TLSOpts, func(c *tls.Config) {
		c.NextProtos = []string{"h2", "http/1.1"}
	})
	return result, nil
}

// minTLSVersion resolves a profile's minimum TLS version, mirroring
// library-go's TLSVersionOrDie without the panic so invalid values fail
// startup with an error instead.
func minTLSVersion(version configv1.TLSProtocolVersion) (uint16, error) {
	switch version {
	case "":
		return tls.VersionTLS12, nil // library-go defaults an unset minimum to TLS 1.2
	case configv1.VersionTLS10:
		return tls.VersionTLS10, nil
	case configv1.VersionTLS11:
		return tls.VersionTLS11, nil
	case configv1.VersionTLS12:
		return tls.VersionTLS12, nil
	case configv1.VersionTLS13:
		return tls.VersionTLS13, nil
	default:
		return 0, fmt.Errorf("failed to resolve minimum TLS version %q: unsupported", version)
	}
}

// usableCipherSuites filters cipher suite IDs down to those Go actually
// implements for TLS <= 1.2 handshakes: TLS 1.3-only IDs (0x1300-0x13ff) and
// defined-but-unimplemented IDs are dropped.
func usableCipherSuites(mapped []uint16) []uint16 {
	implemented := make(map[uint16]struct{})
	for _, suite := range tls.CipherSuites() {
		implemented[suite.ID] = struct{}{}
	}
	for _, suite := range tls.InsecureCipherSuites() {
		implemented[suite.ID] = struct{}{}
	}

	var usable []uint16
	for _, id := range mapped {
		if id >= 0x1300 && id <= 0x13ff {
			continue
		}
		if _, ok := implemented[id]; !ok {
			continue
		}
		usable = append(usable, id)
	}
	return usable
}

// buildProfileTLSOpts validates the profile and returns TLS opts applying it.
func buildProfileTLSOpts(profile configv1.TLSProfileSpec, logger logr.Logger) ([]func(*tls.Config), error) {
	minVersion, err := minTLSVersion(profile.MinTLSVersion)
	if err != nil {
		return nil, err
	}
	// Serving a TLS 1.0/1.1 minimum on Go 1.22+ requires no GODEBUG:
	// crypto/tls drops versions below 1.2 only when MinVersion is unset
	// (Config.supportedVersions), and NewTLSConfigFromProfile always sets
	// MinVersion explicitly, even for an empty profile minimum (library-go
	// maps it to TLS 1.2).
	tlsConfigFn, unsupportedCiphers := tlspkg.NewTLSConfigFromProfile(profile)
	if len(unsupportedCiphers) > 0 {
		logger.Info("some ciphers from TLS profile are not supported by Go", "unsupported", unsupportedCiphers)
	}
	opts := []func(*tls.Config){tlsConfigFn}

	if minVersion >= tls.VersionTLS13 {
		// TLS 1.3 suites are managed by Go, never set CipherSuites.
		return opts, nil
	}

	// This scratch config only inspects suites after the profile option is applied.
	// It is never used to serve traffic.
	probe := &tls.Config{MinVersion: tls.VersionTLS12} // tls-lint:ignore
	tlsConfigFn(probe)
	usable := usableCipherSuites(probe.CipherSuites)
	if len(profile.Ciphers) > 0 && len(usable) == 0 {
		return nil, fmt.Errorf("failed to find usable cipher suites for TLS <= 1.2: all %d ciphers in TLS profile are TLS 1.3-only or unimplemented by Go", len(profile.Ciphers))
	}
	// Keep usable suites when the profile lists ciphers so unusable IDs never reach the
	// effective config. An empty list means no restriction; crc and the control plane use
	// Go's default suites, so we match rather than fail.
	return append(opts, func(c *tls.Config) {
		c.CipherSuites = usable
	}), nil
}

// SetupWatcher registers the security profile watcher so profile changes
// trigger a graceful shutdown and reload. It is a no-op when the cluster has
// no config.openshift.io API.
func SetupWatcher(mgr manager.Manager, result *Result, cancel context.CancelFunc, logger logr.Logger) error {
	if result == nil || !result.HasOpenShiftConfigAPI {
		return nil
	}

	watcher := &tlspkg.SecurityProfileWatcher{
		Client:                mgr.GetClient(),
		InitialTLSProfileSpec: result.Profile,
		OnProfileChange: func(_ context.Context, _, _ configv1.TLSProfileSpec) {
			logger.Info("TLS profile changed, initiating graceful shutdown to reload")
			cancel()
		},
	}
	if err := watcher.SetupWithManager(mgr); err != nil {
		return fmt.Errorf("failed to register TLS security profile watcher: %w", err)
	}
	return nil
}
