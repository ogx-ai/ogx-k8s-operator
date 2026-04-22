package featureflags

type FeatureFlag struct {
	Enabled bool `yaml:"enabled"`
}

// FeatureFlags represents the configuration for feature flags in the operator.
// Add more feature flags later.
type FeatureFlags struct {
	// EnableNetworkPolicy controls whether NetworkPolicy resources should be created.
	EnableNetworkPolicy FeatureFlag `yaml:"enableNetworkPolicy"`
}

const (
	// FeatureFlagsKey is the key used in the ConfigMap to store feature flags.
	FeatureFlagsKey = "featureFlags"
	// EnableNetworkPolicyKey is the key for the network policy feature flag.
	EnableNetworkPolicyKey = "enableNetworkPolicy"
	// NetworkPolicyDefaultValue is the default value for the network policy feature flag.
	NetworkPolicyDefaultValue = false
	// ResetOnUpgrade controls whether feature flags are reset to defaults on
	// operator version change.
	ResetOnUpgrade = false
)
