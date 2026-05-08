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

package config

import (
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// testRegistry is a minimal in-process OCI registry that serves manifests and
// config blobs. It avoids depending on go-containerregistry's registry package.
type testRegistry struct {
	// key: "repo:tag" -> manifest JSON bytes
	manifests map[string][]byte
	// key: digest -> config blob bytes
	blobs map[string][]byte
}

func newTestRegistry() *testRegistry {
	return &testRegistry{
		manifests: make(map[string][]byte),
		blobs:     make(map[string][]byte),
	}
}

type ociImageConfig struct {
	Config struct {
		Labels map[string]string `json:"Labels"`
	} `json:"config"`
}

type ociManifest struct {
	SchemaVersion int `json:"schemaVersion"`
	MediaType     string `json:"mediaType"`
	Config        struct {
		MediaType string `json:"mediaType"`
		Digest    string `json:"digest"`
	} `json:"config"`
}

func (r *testRegistry) pushImage(repo, tag string, labels map[string]string) {
	cfg := ociImageConfig{}
	cfg.Config.Labels = labels
	configBytes, _ := json.Marshal(cfg) //nolint:errchkjson // test-only typed struct
	digest := fmt.Sprintf("sha256:%x", sha256.Sum256(configBytes))

	r.blobs[digest] = configBytes

	m := ociManifest{
		SchemaVersion: 2,
		MediaType:     "application/vnd.oci.image.manifest.v1+json",
	}
	m.Config.MediaType = "application/vnd.oci.image.config.v1+json"
	m.Config.Digest = digest
	manifestBytes, _ := json.Marshal(m) //nolint:errchkjson // test-only typed struct
	r.manifests[repo+":"+tag] = manifestBytes
}

func (r *testRegistry) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	path := strings.TrimPrefix(req.URL.Path, "/v2/")

	// Handle manifest requests: <repo>/manifests/<tag>
	if parts := strings.SplitN(path, "/manifests/", 2); len(parts) == 2 {
		repo, tag := parts[0], parts[1]
		if data, ok := r.manifests[repo+":"+tag]; ok {
			w.Header().Set("Content-Type", "application/vnd.oci.image.manifest.v1+json")
			w.Write(data)
			return
		}
		http.NotFound(w, req)
		return
	}

	// Handle blob requests: <repo>/blobs/<digest>
	if parts := strings.SplitN(path, "/blobs/", 2); len(parts) == 2 {
		digest := parts[1]
		if data, ok := r.blobs[digest]; ok {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
			return
		}
		http.NotFound(w, req)
		return
	}

	http.NotFound(w, req)
}

func setupTestRegistryWithImages(t *testing.T) (*testRegistry, string) {
	t.Helper()
	reg := newTestRegistry()
	s := httptest.NewServer(reg)
	t.Cleanup(s.Close)
	host := strings.TrimPrefix(s.URL, "http://")
	return reg, host
}

func TestFetchOCILabels_WithConfigLabel(t *testing.T) {
	reg, host := setupTestRegistryWithImages(t)

	configYAML := "version: '2'\nimage_name: test-distro\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(configYAML))

	reg.pushImage("distro/test", "v1", map[string]string{
		OCIConfigLabel: encoded,
		"unrelated":    "value",
	})

	labels, err := fetchOCILabels(fmt.Sprintf("%s/distro/test:v1", host))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if labels[OCIConfigLabel] != encoded {
		t.Errorf("expected label %q = %q, got %q", OCIConfigLabel, encoded, labels[OCIConfigLabel])
	}
}

func TestFetchOCILabels_NoLabels(t *testing.T) {
	reg, host := setupTestRegistryWithImages(t)

	reg.pushImage("distro/empty", "v1", nil)

	labels, err := fetchOCILabels(fmt.Sprintf("%s/distro/empty:v1", host))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(labels) != 0 {
		t.Errorf("expected empty labels, got %v", labels)
	}
}

func TestFetchOCILabels_InvalidRef(t *testing.T) {
	_, err := fetchOCILabels(":::invalid")
	if err == nil {
		t.Fatal("expected error for invalid image reference")
	}
}

func TestFetchOCILabels_Unreachable(t *testing.T) {
	_, err := fetchOCILabels("localhost:1/nonexistent:v1")
	if err == nil {
		t.Fatal("expected error for unreachable registry")
	}
}

func TestResolverWithRealOCIFetch(t *testing.T) {
	reg, host := setupTestRegistryWithImages(t)

	configYAML := "version: '2'\nimage_name: oci-resolved\napis:\n- inference\nproviders:\n  inference:\n  - provider_id: test\n    provider_type: remote::test\n    config: {}\n"
	encoded := base64.StdEncoding.EncodeToString([]byte(configYAML))

	reg.pushImage("distro/starter", "latest", map[string]string{
		OCIConfigLabel: encoded,
	})

	resolver := NewDefaultConfigResolver(fetchOCILabels)
	data, err := resolver.Resolve(fmt.Sprintf("%s/distro/starter:latest", host), "starter")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(data) != configYAML {
		t.Errorf("expected config %q, got %q", configYAML, string(data))
	}
}

func TestResolverOCIFallsBackWhenLabelMissing(t *testing.T) {
	reg, host := setupTestRegistryWithImages(t)

	reg.pushImage("distro/nolabel", "v1", map[string]string{
		"other": "label",
	})

	resolver := NewDefaultConfigResolver(fetchOCILabels)
	data, err := resolver.Resolve(fmt.Sprintf("%s/distro/nolabel:v1", host), "starter")
	if err != nil {
		t.Fatalf("expected fallback to embedded, got error: %v", err)
	}
	if len(data) == 0 {
		t.Error("expected non-empty config from embedded fallback")
	}
}

func TestNewOCILabelFetcher_ReturnsWorkingFetcher(t *testing.T) {
	fetcher := NewOCILabelFetcher()
	if fetcher == nil {
		t.Fatal("expected non-nil fetcher")
	}

	_, err := fetcher(":::invalid")
	if err == nil {
		t.Error("expected error for invalid reference")
	}
}

func TestResolverCachesOCIResult(t *testing.T) {
	callCount := 0
	fetcher := func(imageRef string) (map[string]string, error) {
		callCount++
		return map[string]string{
			OCIConfigLabel: base64.StdEncoding.EncodeToString([]byte("version: '2'\nimage_name: cached\n")),
		}, nil
	}

	resolver := NewDefaultConfigResolver(fetcher)

	_, err := resolver.Resolve("test-image:v1", "")
	if err != nil {
		t.Fatalf("first resolve: %v", err)
	}
	_, err = resolver.Resolve("test-image:v1", "")
	if err != nil {
		t.Fatalf("second resolve: %v", err)
	}

	if callCount != 1 {
		t.Errorf("expected fetcher called once (cached), called %d times", callCount)
	}
}
