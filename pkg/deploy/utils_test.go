package deploy

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestGetAPIServerEndpoint_MissingHost(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "")
	t.Setenv("KUBERNETES_SERVICE_PORT", "443")

	host, port, err := GetAPIServerEndpoint()
	require.Error(t, err)
	require.Empty(t, host)
	require.Zero(t, port)
	require.Contains(t, err.Error(), "failed to get API server endpoint: KUBERNETES_SERVICE_HOST not set")
}

func TestGetAPIServerEndpoint_DefaultPort(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "")

	host, port, err := GetAPIServerEndpoint()
	require.NoError(t, err)
	require.Equal(t, "10.96.0.1", host)
	require.Equal(t, int32(443), port)
}

func TestGetAPIServerEndpoint_InvalidPort(t *testing.T) {
	t.Setenv("KUBERNETES_SERVICE_HOST", "10.96.0.1")
	t.Setenv("KUBERNETES_SERVICE_PORT", "invalid")

	host, port, err := GetAPIServerEndpoint()
	require.Error(t, err)
	require.Empty(t, host)
	require.Zero(t, port)
	require.Contains(t, err.Error(), "failed to parse KUBERNETES_SERVICE_PORT")
}
