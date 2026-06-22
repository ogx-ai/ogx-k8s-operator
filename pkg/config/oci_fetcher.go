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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
)

const (
	ociHTTPTimeout  = 30 * time.Second
	configBlobLimit = 1 << 20 // 1 MB
)

var ociHTTPClient = &http.Client{Timeout: ociHTTPTimeout}

// NewOCILabelFetcher returns an OCILabelFetcher that retrieves image labels
// from a container registry using the OCI Distribution HTTP API directly,
// avoiding heavy dependencies on docker/containerd client libraries.
//
// Authentication: currently supports anonymous/public registry access.
// Private registries will fall back to embedded configs via the resolver.
func NewOCILabelFetcher() OCILabelFetcher {
	return fetchOCILabels
}

func fetchOCILabels(imageRef string) (map[string]string, error) {
	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to parse image reference %q: %w", imageRef, err)
	}

	registry := ref.Context().RegistryStr()
	repo := ref.Context().RepositoryStr()
	identifier := ref.Identifier()

	base, err := detectRegistryScheme(registry)
	if err != nil {
		return nil, fmt.Errorf("failed to reach registry %q: %w", registry, err)
	}

	configDigest, err := fetchManifestConfigDigest(base, repo, identifier, imageRef)
	if err != nil {
		return nil, err
	}
	if configDigest == "" {
		return map[string]string{}, nil
	}

	return fetchConfigLabels(base, repo, configDigest, imageRef)
}

func fetchManifestConfigDigest(base, repo, identifier, imageRef string) (string, error) {
	manifestURL := fmt.Sprintf("%s/v2/%s/manifests/%s", base, repo, identifier)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, manifestURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to build manifest request: %w", err)
	}
	req.Header.Set("Accept", "application/vnd.oci.image.manifest.v1+json, application/vnd.docker.distribution.manifest.v2+json")

	resp, err := ociHTTPClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to fetch manifest for %q: %w", imageRef, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("failed to fetch manifest for %q: HTTP %d", imageRef, resp.StatusCode)
	}

	var manifest struct {
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if decodeErr := json.NewDecoder(resp.Body).Decode(&manifest); decodeErr != nil {
		return "", fmt.Errorf("failed to decode manifest for %q: %w", imageRef, decodeErr)
	}

	return manifest.Config.Digest, nil
}

func fetchConfigLabels(base, repo, digest, imageRef string) (map[string]string, error) {
	blobURL := fmt.Sprintf("%s/v2/%s/blobs/%s", base, repo, digest)
	req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, blobURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build config blob request: %w", err)
	}

	resp, err := ociHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch config blob for %q: %w", imageRef, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to fetch config blob for %q: HTTP %d", imageRef, resp.StatusCode)
	}

	body, err := io.ReadAll(io.LimitReader(resp.Body, configBlobLimit))
	if err != nil {
		return nil, fmt.Errorf("failed to read config blob for %q: %w", imageRef, err)
	}

	var imgConfig struct {
		Config struct {
			Labels map[string]string `json:"Labels"`
		} `json:"config"`
	}
	if err := json.Unmarshal(body, &imgConfig); err != nil {
		return nil, fmt.Errorf("failed to decode config for %q: %w", imageRef, err)
	}

	if imgConfig.Config.Labels == nil {
		return map[string]string{}, nil
	}
	return imgConfig.Config.Labels, nil
}

// detectRegistryScheme probes the registry's /v2/ endpoint over HTTPS first,
// falling back to HTTP if HTTPS fails. This mirrors Docker's behavior and
// avoids hardcoding hostname patterns for plain-HTTP registries.
func detectRegistryScheme(registry string) (string, error) {
	httpsErr := false
	for _, scheme := range []string{"https", "http"} {
		probeURL := fmt.Sprintf("%s://%s/v2/", scheme, registry)
		req, err := http.NewRequestWithContext(context.Background(), http.MethodGet, probeURL, nil)
		if err != nil {
			if scheme == "https" {
				httpsErr = true
			}
			continue
		}
		resp, err := ociHTTPClient.Do(req)
		if err != nil {
			if scheme == "https" {
				httpsErr = true
			}
			continue
		}
		_ = resp.Body.Close()
		if scheme == "http" && httpsErr {
			slog.Warn("HTTPS probe failed, falling back to HTTP for registry", "registry", registry)
		}
		return fmt.Sprintf("%s://%s", scheme, registry), nil
	}
	return "", errors.New("failed to connect on https or http")
}
