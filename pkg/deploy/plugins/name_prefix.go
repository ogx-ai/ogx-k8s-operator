package plugins

import (
	"fmt"
	"slices"
	"strings"

	"sigs.k8s.io/kustomize/api/resmap"
)

// NamePrefixConfig holds configuration for the name prefix plugin.
type NamePrefixConfig struct {
	// Prefix to add to resource names.
	Prefix string
	// IncludeKinds specifies which resource kinds to apply the prefix to.
	// If empty, the prefix is applied to all resource kinds not specified in ExcludeKinds.
	IncludeKinds []string
	// ExcludeKinds specifies which resource kinds to exclude from prefixing.
	// This list takes precedence over IncludeKinds; if a kind is in both lists,
	// it will be excluded.
	ExcludeKinds []string
}

// CreateNamePrefixPlugin creates a transformer plugin that adds a prefix to resource names.
// Acts as a constructor, ensuring the plugin is properly initialized with its configuration.
func CreateNamePrefixPlugin(config NamePrefixConfig) *namePrefixTransformer {
	return &namePrefixTransformer{config: config}
}

type namePrefixTransformer struct {
	config NamePrefixConfig
}

// Transform implements the TransformerPlugin interface.
// Iterates through resources and applies the configured name prefix.
func (t *namePrefixTransformer) Transform(m resmap.ResMap) error {
	for _, res := range m.Resources() {
		// Skip if the resource already has the prefix to prevent duplicates.
		if strings.HasPrefix(res.GetName(), t.config.Prefix+"-") {
			continue
		}

		// Check if we should apply prefix to this resource kind based on include/exclude rules.
		// Ensures only targeted resource kinds are modified.
		if !shouldApplyToKind(res.GetKind(), t.config.IncludeKinds, t.config.ExcludeKinds) {
			continue
		}

		prefixedName := makePrefixedName(t.config.Prefix, res.GetName())
		if err := ValidateK8sLabelName(prefixedName); err != nil {
			return fmt.Errorf("failed to make valid prefixed name: %w", err)
		}

		// Performs the actual name prefixing for the resource.
		if err := res.SetName(prefixedName); err != nil {
			return fmt.Errorf("failed to set resource name: %w", err)
		}
	}
	return nil
}

// Config implements the TransformerPlugin interface.
// This method is empty because the plugin's configuration is provided directly via `CreateNamePrefixPlugin`.
func (t *namePrefixTransformer) Config(h *resmap.PluginHelpers, _ []byte) error {
	return nil
}

// shouldApplyToKind uses explicit include/exclude logic for flexibility in applying transformations.
func shouldApplyToKind(kind string, includeKinds, excludeKinds []string) bool {
	// Exclusions take precedence.
	if len(excludeKinds) > 0 {
		if slices.Contains(excludeKinds, kind) {
			return false
		}
	}
	// If include list is empty, apply to all kinds (that weren't excluded).
	if len(includeKinds) == 0 {
		return true
	}
	// Otherwise, only apply if the kind is explicitly included.
	return slices.Contains(includeKinds, kind)
}

func makePrefixedName(prefix, name string) string {
	return prefix + "-" + name
}
