//nolint:testpackage
package e2e

import (
	"context"
	"crypto/tls"
	"io"
	"net/http"
	"os"
	"testing"
	"time"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	"github.com/stretchr/testify/require"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// TestIdentityAntiSpoofingSuite verifies end-to-end security properties:
// - Scenario 1: Valid SA token + spoofed headers -> Gateway overwrites identity headers (200 OK).
// - Scenario 2: Missing token + spoofed headers   -> Gateway rejects request (401 Unauthorized).
// - Scenario 3: Direct request bypassing Gateway -> Direct access rejected by NetworkPolicy / trusted_proxy_cidrs (403 Forbidden).
func TestIdentityAntiSpoofingSuite(t *testing.T) {
	if TestOpts.SkipCreation {
		t.Skip("Skipping Identity Anti-Spoofing suite (SkipCreation=true)")
	}

	nsName := "ogx-anti-spoofing-test"
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: nsName}}
	err := TestEnv.Client.Create(TestEnv.Ctx, ns)
	if err != nil && !apierrors.IsAlreadyExists(err) {
		require.NoError(t, err)
	}

	praxisOn := true
	server := GetSampleCRForDistribution(t, starterDistType)
	server.Name = "ogx-anti-spoofing"
	server.Namespace = nsName
	server.Spec.PraxisMode = &ogxiov1beta1.PraxisModeSpec{
		Enabled: &praxisOn,
		PraxisSelector: &ogxiov1beta1.PraxisSelector{
			Namespace: nsName,
			PodSelector: metav1.LabelSelector{
				MatchLabels: map[string]string{"app": "payload-processing"},
			},
		},
	}

	EnsureOverrideConfigMap(t, TestEnv.Client, TestEnv.Ctx, server)
	require.NoError(t, TestEnv.Client.Create(TestEnv.Ctx, server))

	t.Cleanup(func() {
		_ = TestEnv.Client.Delete(context.Background(), server)
		_ = TestEnv.Client.Delete(context.Background(), ns)
	})

	gatewayURL := os.Getenv("GATEWAY_URL")
	saToken := os.Getenv("K8S_SA_TOKEN")
	expectedUser := os.Getenv("EXPECTED_USER")
	expectedTenant := os.Getenv("EXPECTED_TENANT")

	if gatewayURL == "" {
		gatewayURL = "http://localhost:8080"
	}

	client := &http.Client{
		Timeout: 10 * time.Second,
		Transport: &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		},
	}

	// --- Scenario 1: Valid Token + Spoofed Headers ---
	t.Run("Scenario 1: Valid SA token + spoofed headers -> Gateway overwrites identity headers (200 OK)", func(t *testing.T) {
		if saToken == "" {
			t.Skip("K8S_SA_TOKEN not provided; skipping live token validation assertion")
		}
		req, err := http.NewRequestWithContext(TestEnv.Ctx, http.MethodGet, gatewayURL+"/v1/health", nil)
		require.NoError(t, err)

		req.Header.Set("Authorization", "Bearer "+saToken)
		req.Header.Set("X-User-Id", "evil-admin")
		req.Header.Set("X-Tenant-Id", "other-tenant")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		body, _ := io.ReadAll(resp.Body)
		require.Equal(t, http.StatusOK, resp.StatusCode, "Valid token should be accepted by Gateway; body: %s", string(body))

		if expectedUser != "" {
			require.NotEqual(t, "evil-admin", resp.Header.Get("X-User-Id"), "SECURITY VIOLATION: Spoofed x-user-id was passed through!")
			require.Equal(t, expectedUser, resp.Header.Get("X-User-Id"), "x-user-id must match authenticated SA identity")
		}
		if expectedTenant != "" {
			require.NotEqual(t, "other-tenant", resp.Header.Get("X-Tenant-Id"), "SECURITY VIOLATION: Spoofed x-tenant-id was passed through!")
			require.Equal(t, expectedTenant, resp.Header.Get("X-Tenant-Id"), "x-tenant-id must match derived tenant")
		}
	})

	// --- Scenario 2: Missing Token + Spoofed Headers ---
	t.Run("Scenario 2: Missing token + spoofed headers -> Gateway rejects request (401 Unauthorized)", func(t *testing.T) {
		req, err := http.NewRequestWithContext(TestEnv.Ctx, http.MethodGet, gatewayURL+"/v1/health", nil)
		require.NoError(t, err)

		req.Header.Set("X-User-Id", "evil-admin")
		req.Header.Set("X-Tenant-Id", "other-tenant")

		resp, err := client.Do(req)
		require.NoError(t, err)
		defer resp.Body.Close()

		require.Equal(t, http.StatusUnauthorized, resp.StatusCode, "Gateway MUST reject unauthenticated request with 401")
	})

	// --- Scenario 3: Direct Access Bypassing Gateway ---
	t.Run("Scenario 3: Direct request bypassing Gateway -> Direct access rejected (403 Forbidden)", func(t *testing.T) {
		directPodURL := os.Getenv("DIRECT_OGX_URL")
		if directPodURL == "" {
			directPodURL = "http://localhost:8321"
		}

		req, err := http.NewRequestWithContext(TestEnv.Ctx, http.MethodGet, directPodURL+"/v1/health", nil)
		require.NoError(t, err)

		req.Header.Set("X-User-Id", "evil-admin")
		req.Header.Set("X-Tenant-Id", "other-tenant")

		resp, err := client.Do(req)
		if err != nil {
			// Connection refused / blocked by NetworkPolicy is also a valid direct-access rejection
			t.Logf("Direct access blocked at network layer: %v", err)
			return
		}
		defer resp.Body.Close()

		require.Equal(t, http.StatusForbidden, resp.StatusCode, "Direct access bypassing Gateway MUST be rejected with 403 Forbidden")
	})
}
