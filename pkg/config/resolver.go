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
	"embed"
	"encoding/base64"
	"fmt"
	"sync"
)

//go:embed configs/*/config.yaml
var embeddedConfigs embed.FS

const (
	// OCIConfigLabel is the OCI image label containing the base64-encoded config.yaml.
	OCIConfigLabel = "com.ogx.distribution.configs"
)

// EmbeddedDistributionNames returns the names of all embedded distributions
// by listing subdirectories under the embedded configs filesystem.
func EmbeddedDistributionNames() ([]string, error) {
	entries, err := embeddedConfigs.ReadDir("configs")
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded configs: %w", err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	return names, nil
}

// OCILabelFetcher fetches OCI image labels for a given image reference.
// Returns the label map, or an error if the image is inaccessible.
type OCILabelFetcher func(imageRef string) (map[string]string, error)

// ConfigResolver resolves a distribution's base config.yaml content.
type ConfigResolver interface {
	// Resolve returns the base config for the given distribution.
	// imageRef is the resolved container image reference (may be empty).
	// distributionName is the named distribution (may be empty for direct image refs).
	Resolve(imageRef string, distributionName string) ([]byte, error)
}

// DefaultConfigResolver implements a two-tier resolution strategy:
// 1. Try OCI image label (com.ogx.distribution.configs) from the resolved image
// 2. Fall back to embedded configs from pkg/config/configs/.
type DefaultConfigResolver struct {
	fs           embed.FS
	cache        *ociConfigCache
	labelFetcher OCILabelFetcher
	// lastOCIErr stores the last OCI resolution error when falling back to embedded.
	// Callers can check this via LastOCIError() for diagnostic logging.
	lastOCIErr error
}

// NewDefaultConfigResolver creates a resolver with OCI-first, embedded-fallback strategy.
// If labelFetcher is nil, OCI resolution is skipped and only embedded configs are used.
func NewDefaultConfigResolver(labelFetcher OCILabelFetcher) *DefaultConfigResolver {
	return &DefaultConfigResolver{
		fs:           embeddedConfigs,
		cache:        newOCIConfigCache(),
		labelFetcher: labelFetcher,
	}
}

func (r *DefaultConfigResolver) Resolve(imageRef string, distributionName string) ([]byte, error) {
	r.lastOCIErr = nil

	// Try OCI label first if we have an image reference and a label fetcher
	if imageRef != "" && r.labelFetcher != nil {
		data, ociErr := r.resolveFromOCI(imageRef)
		if ociErr == nil && len(data) > 0 {
			return data, nil
		}
		r.lastOCIErr = ociErr
	}

	// Fall back to embedded config
	if distributionName == "" {
		return nil, fmt.Errorf("failed to resolve config: OCI label %q not found on image %q and no distribution name for embedded fallback", OCIConfigLabel, imageRef)
	}
	return r.resolveFromEmbedded(distributionName)
}

// LastOCIError returns the error from the last OCI resolution attempt that
// fell back to embedded config. Returns nil if OCI was not attempted or succeeded.
func (r *DefaultConfigResolver) LastOCIError() error {
	return r.lastOCIErr
}

func (r *DefaultConfigResolver) resolveFromOCI(imageRef string) ([]byte, error) {
	// Check cache first
	if data, ok := r.cache.get(imageRef); ok {
		return data, nil
	}

	labels, err := r.labelFetcher(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OCI labels for %q: %w", imageRef, err)
	}

	labelValue, ok := labels[OCIConfigLabel]
	if !ok || labelValue == "" {
		return nil, fmt.Errorf("failed to find OCI label %q on image %q", OCIConfigLabel, imageRef)
	}

	data, err := base64.StdEncoding.DecodeString(labelValue)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 config from OCI label on %q: %w", imageRef, err)
	}

	// Cache the result
	r.cache.set(imageRef, data)

	return data, nil
}

func (r *DefaultConfigResolver) resolveFromEmbedded(distributionName string) ([]byte, error) {
	data, err := r.fs.ReadFile(fmt.Sprintf("configs/%s/config.yaml", distributionName))
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded config for distribution %q: %w", distributionName, err)
	}
	return data, nil
}

const maxOCICacheEntries = 64

// ociConfigCache provides thread-safe caching of OCI label configs by image reference.
// Evicts all entries when the cache exceeds maxOCICacheEntries to bound memory usage.
type ociConfigCache struct {
	mu    sync.RWMutex
	store map[string][]byte
}

func newOCIConfigCache() *ociConfigCache {
	return &ociConfigCache{store: make(map[string][]byte)}
}

func (c *ociConfigCache) get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	data, ok := c.store[key]
	return data, ok
}

func (c *ociConfigCache) set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.store) >= maxOCICacheEntries {
		c.store = make(map[string][]byte)
	}
	c.store[key] = data
}
