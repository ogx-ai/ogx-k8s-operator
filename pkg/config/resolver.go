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
	"container/list"
	"encoding/base64"
	"errors"
	"fmt"
	"strings"
	"sync"
)

const (
	// OCIConfigListLabel is the OCI metadata label listing available config filenames.
	OCIConfigListLabel = "com.ogx.distribution.configs"
	// OCIDefaultConfigLabel identifies which config filename is the runtime default.
	OCIDefaultConfigLabel = "com.ogx.distribution.default-config"
	// OCIConfigLabelPrefix prefixes the per-file base64-encoded config labels.
	OCIConfigLabelPrefix = "com.ogx.config."
)

// OCILabelFetcher fetches OCI image labels for a given image reference.
// Returns the label map, or an error if the image is inaccessible.
type OCILabelFetcher func(imageRef string) (map[string]string, error)

// ConfigResolver resolves a distribution's base config.yaml content from OCI labels.
type ConfigResolver interface {
	// Resolve returns the base config for the given distribution.
	// imageRef is the resolved container image reference.
	// distributionName is accepted for compatibility with existing callers.
	Resolve(imageRef string, distributionName string) ([]byte, error)
}

// DefaultConfigResolver resolves base config content from OCI image labels.
type DefaultConfigResolver struct {
	cache        *ociConfigCache
	labelFetcher OCILabelFetcher
}

// NewDefaultConfigResolver creates a resolver backed by OCI label lookups.
func NewDefaultConfigResolver(labelFetcher OCILabelFetcher) *DefaultConfigResolver {
	return &DefaultConfigResolver{
		cache:        newOCIConfigCache(),
		labelFetcher: labelFetcher,
	}
}

func (r *DefaultConfigResolver) Resolve(imageRef string, _ string) ([]byte, error) {
	if imageRef == "" {
		return nil, errors.New("failed to resolve base config from OCI labels: distribution image reference is empty")
	}
	if r.labelFetcher == nil {
		return nil, fmt.Errorf("failed to resolve base config from OCI labels for %q: OCI label fetcher is not configured", imageRef)
	}
	return r.resolveFromOCI(imageRef)
}

func (r *DefaultConfigResolver) resolveFromOCI(imageRef string) ([]byte, error) {
	// Cache only digest-pinned references. Mutable tags (for example :latest)
	// must be refetched so updated OCI labels are observed without a restart.
	if shouldCacheOCIConfig(imageRef) {
		if data, ok := r.cache.get(imageRef); ok {
			return data, nil
		}
	}

	labels, err := r.labelFetcher(imageRef)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch OCI labels for %q: %w", imageRef, err)
	}

	defaultConfigName, ok := labels[OCIDefaultConfigLabel]
	if !ok || defaultConfigName == "" {
		return nil, fmt.Errorf("failed to find OCI label %q on image %q", OCIDefaultConfigLabel, imageRef)
	}

	configLabel := OCIConfigLabelPrefix + defaultConfigName
	labelValue, ok := labels[configLabel]
	if !ok || labelValue == "" {
		return nil, fmt.Errorf("failed to find OCI label %q on image %q (available configs: %q)", configLabel, imageRef, labels[OCIConfigListLabel])
	}
	data, err := base64.StdEncoding.DecodeString(labelValue)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 config from OCI label %q on %q: %w", configLabel, imageRef, err)
	}

	if shouldCacheOCIConfig(imageRef) {
		r.cache.set(imageRef, data)
	}

	return data, nil
}

func shouldCacheOCIConfig(imageRef string) bool {
	return strings.Contains(imageRef, "@sha256:")
}

const maxOCICacheEntries = 64

type ociCacheEntry struct {
	key  string
	data []byte
}

// ociConfigCache provides thread-safe LRU caching of OCI label configs by image reference.
type ociConfigCache struct {
	mu    sync.Mutex
	items map[string]*list.Element
	order list.List
}

func newOCIConfigCache() *ociConfigCache {
	return &ociConfigCache{items: make(map[string]*list.Element)}
}

func (c *ociConfigCache) get(key string) ([]byte, bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	elem, ok := c.items[key]
	if !ok {
		return nil, false
	}
	c.order.MoveToFront(elem)
	entry := cacheEntry(elem)
	return entry.data, true
}

func (c *ociConfigCache) set(key string, data []byte) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if elem, ok := c.items[key]; ok {
		c.order.MoveToFront(elem)
		entry := cacheEntry(elem)
		entry.data = data
		return
	}
	if len(c.items) >= maxOCICacheEntries {
		oldest := c.order.Back()
		oldestEntry := cacheEntry(oldest)
		_ = c.order.Remove(oldest)
		delete(c.items, oldestEntry.key)
	}
	elem := c.order.PushFront(&ociCacheEntry{key: key, data: data})
	c.items[key] = elem
}

func cacheEntry(elem *list.Element) *ociCacheEntry {
	entry, ok := elem.Value.(*ociCacheEntry)
	if !ok {
		panic("unexpected OCI cache entry type")
	}
	return entry
}
