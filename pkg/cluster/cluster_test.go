package cluster

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/stretchr/testify/require"
)

// TestDistributionsJSONIsValid ensures that the distributions.json file always
// contains well-formed JSON and that all keys and values are non-empty.
func TestDistributionsJSONIsValid(t *testing.T) {
	data, err := os.ReadFile("../../distributions.json")
	if err != nil {
		t.Fatalf("failed to read distributions.json: %v", err)
	}

	var dist map[string]string
	if err := json.Unmarshal(data, &dist); err != nil {
		t.Fatalf("failed to validate distributions.json: %v", err)
	}

	for k, v := range dist {
		if k == "" {
			t.Fatalf("failed to validate distributions.json: contains an empty key")
		}
		if v == "" {
			t.Fatalf("failed to validate distributions.json: contains an empty value for key %q", k)
		}
	}
}

func TestDistributionEnvVarKey(t *testing.T) {
	tests := []struct {
		name     string
		expected string
	}{
		{name: "starter", expected: "RELATED_IMAGE_STARTER"},
		{name: "remote-vllm", expected: "RELATED_IMAGE_REMOTE_VLLM"},
		{name: "meta-reference-gpu", expected: "RELATED_IMAGE_META_REFERENCE_GPU"},
		{name: "postgres-demo", expected: "RELATED_IMAGE_POSTGRES_DEMO"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			require.Equal(t, tt.expected, DistributionEnvVarKey(tt.name))
		})
	}
}

func TestNewClusterInfoRelatedImageOverrides(t *testing.T) {
	distributions := `{
		"starter": "docker.io/ogxai/distribution-starter:latest",
		"remote-vllm": "docker.io/ogxai/distribution-remote-vllm:latest",
		"meta-reference-gpu": "docker.io/ogxai/distribution-meta-reference-gpu:latest"
	}`

	t.Run("no env vars set uses distributions.json defaults", func(t *testing.T) {
		var images map[string]string
		require.NoError(t, json.Unmarshal([]byte(distributions), &images))

		applyRelatedImageOverrides(images)

		require.Equal(t, "docker.io/ogxai/distribution-starter:latest", images["starter"])
		require.Equal(t, "docker.io/ogxai/distribution-remote-vllm:latest", images["remote-vllm"])
		require.Equal(t, "docker.io/ogxai/distribution-meta-reference-gpu:latest", images["meta-reference-gpu"])
	})

	t.Run("env var overrides single distribution", func(t *testing.T) {
		t.Setenv("RELATED_IMAGE_STARTER", "mirror.example.com/ogx-starter@sha256:abc123")

		var images map[string]string
		require.NoError(t, json.Unmarshal([]byte(distributions), &images))

		applyRelatedImageOverrides(images)

		require.Equal(t, "mirror.example.com/ogx-starter@sha256:abc123", images["starter"])
		require.Equal(t, "docker.io/ogxai/distribution-remote-vllm:latest", images["remote-vllm"])
	})

	t.Run("env var overrides all distributions", func(t *testing.T) {
		t.Setenv("RELATED_IMAGE_STARTER", "mirror.example.com/starter@sha256:aaa")
		t.Setenv("RELATED_IMAGE_REMOTE_VLLM", "mirror.example.com/vllm@sha256:bbb")
		t.Setenv("RELATED_IMAGE_META_REFERENCE_GPU", "mirror.example.com/gpu@sha256:ccc")

		var images map[string]string
		require.NoError(t, json.Unmarshal([]byte(distributions), &images))

		applyRelatedImageOverrides(images)

		require.Equal(t, "mirror.example.com/starter@sha256:aaa", images["starter"])
		require.Equal(t, "mirror.example.com/vllm@sha256:bbb", images["remote-vllm"])
		require.Equal(t, "mirror.example.com/gpu@sha256:ccc", images["meta-reference-gpu"])
	})

	t.Run("empty env var does not override", func(t *testing.T) {
		t.Setenv("RELATED_IMAGE_STARTER", "")

		var images map[string]string
		require.NoError(t, json.Unmarshal([]byte(distributions), &images))

		applyRelatedImageOverrides(images)

		require.Equal(t, "docker.io/ogxai/distribution-starter:latest", images["starter"])
	})
}
