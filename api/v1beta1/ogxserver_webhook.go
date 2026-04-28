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

package v1beta1

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var ogxserverlog = logf.Log.WithName("ogxserver-webhook")

// OGXServerValidator validates OGXServer resources.
type OGXServerValidator struct {
	// EmbeddedDistributionNames is the list of known distribution names from
	// the embedded distribution registry. Injected at setup time to avoid
	// import cycles with pkg/cluster.
	EmbeddedDistributionNames []string
}

var _ admission.CustomValidator = &OGXServerValidator{}

// SetupWebhookWithManager registers the validating webhook.
// embeddedDistNames should be the keys from the embedded distributions map.
func SetupWebhookWithManager(mgr ctrl.Manager, embeddedDistNames []string) error {
	return ctrl.NewWebhookManagedBy(mgr).
		For(&OGXServer{}).
		WithValidator(&OGXServerValidator{
			EmbeddedDistributionNames: embeddedDistNames,
		}).
		Complete()
}

//nolint:lll // kubebuilder marker cannot be split across lines.
//+kubebuilder:webhook:path=/validate-ogx-io-v1beta1-ogxserver,mutating=false,failurePolicy=fail,sideEffects=None,groups=ogx.io,resources=ogxservers,verbs=create;update,versions=v1beta1,name=vogxserver.kb.io,admissionReviewVersions=v1

// ValidateCreate implements admission.CustomValidator.
func (v *OGXServerValidator) ValidateCreate(_ context.Context, obj runtime.Object) (admission.Warnings, error) {
	r, ok := obj.(*OGXServer)
	if !ok {
		return nil, fmt.Errorf("failed to validate: expected *OGXServer, got %T", obj)
	}
	ogxserverlog.Info("validating create", "name", r.Name)
	return v.validate(r)
}

// ValidateUpdate implements admission.CustomValidator.
func (v *OGXServerValidator) ValidateUpdate(_ context.Context, _, newObj runtime.Object) (admission.Warnings, error) {
	r, ok := newObj.(*OGXServer)
	if !ok {
		return nil, fmt.Errorf("failed to validate: expected *OGXServer, got %T", newObj)
	}
	ogxserverlog.Info("validating update", "name", r.Name)
	return v.validate(r)
}

// ValidateDelete implements admission.CustomValidator.
func (v *OGXServerValidator) ValidateDelete(_ context.Context, _ runtime.Object) (admission.Warnings, error) {
	return nil, nil
}

func (v *OGXServerValidator) validate(r *OGXServer) (admission.Warnings, error) {
	allErrs := v.collectValidationErrors(r)
	if len(allErrs) > 0 {
		return nil, allErrs.ToAggregate()
	}
	return nil, nil
}

func (v *OGXServerValidator) collectValidationErrors(r *OGXServer) field.ErrorList {
	var allErrs field.ErrorList

	if r.Spec.Distribution.Name != "" {
		allErrs = append(allErrs, validateDistributionName(r.Spec.Distribution.Name, v.EmbeddedDistributionNames)...)
	}

	if r.Spec.Providers != nil {
		allErrs = append(allErrs, validateProviderIDUniqueness(r.Spec.Providers)...)
	}

	if r.Spec.Resources != nil && r.Spec.Providers != nil {
		allErrs = append(allErrs, validateProviderReferences(r.Spec.Resources, r.Spec.Providers)...)
	}

	return allErrs
}

// validateProviderIDUniqueness ensures provider IDs are unique across all API types.
// Per-slice uniqueness is handled by CEL; this validates cross-slice uniqueness.
func validateProviderIDUniqueness(spec *ProvidersSpec) field.ErrorList {
	var errs field.ErrorList
	seenIDs := make(map[string]string)

	fields := []struct {
		name    string
		configs []ProviderConfig
	}{
		{"inference", spec.Inference},
		{"safety", spec.Safety},
		{"vectorIo", spec.VectorIo},
		{"toolRuntime", spec.ToolRuntime},
	}

	for _, f := range fields {
		for _, pc := range f.configs {
			id := pc.ID
			if id == "" {
				id = deriveProviderID(pc.Provider)
			}
			if existingAPI, exists := seenIDs[id]; exists {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "providers", f.name),
					id,
					fmt.Sprintf("provider ID %q conflicts with provider in %q; all provider IDs must be unique across all API types", id, existingAPI),
				))
			}
			seenIDs[id] = f.name
		}
	}

	return errs
}

// validateProviderReferences ensures model provider references point to configured providers.
func validateProviderReferences(resources *ResourcesSpec, providers *ProvidersSpec) field.ErrorList {
	var errs field.ErrorList

	providerIDs := collectAllProviderIDs(providers)

	for i, mc := range resources.Models {
		if mc.Provider != "" {
			if _, ok := providerIDs[mc.Provider]; !ok {
				errs = append(errs, field.Invalid(
					field.NewPath("spec", "resources", "models").Index(i).Child("provider"),
					mc.Provider,
					fmt.Sprintf("references unknown provider ID; available: %v", sortedMapKeys(providerIDs)),
				))
			}
		}
	}

	return errs
}

// validateDistributionName validates that distribution.name is in the embedded
// distribution registry.
func validateDistributionName(name string, knownNames []string) field.ErrorList {
	if len(knownNames) == 0 {
		return nil
	}

	for _, n := range knownNames {
		if n == name {
			return nil
		}
	}

	sorted := make([]string, len(knownNames))
	copy(sorted, knownNames)
	sort.Strings(sorted)

	var errs field.ErrorList
	errs = append(errs, field.Invalid(
		field.NewPath("spec", "distribution", "name"),
		name,
		fmt.Sprintf("unknown distribution %q; available distributions: %s", name, strings.Join(sorted, ", ")),
	))
	return errs
}

// deriveProviderID strips a "remote::" or similar prefix from a provider type
// to produce the same ID that the config pipeline generates.
func deriveProviderID(providerType string) string {
	if idx := strings.LastIndex(providerType, "::"); idx >= 0 {
		return providerType[idx+2:]
	}
	return providerType
}

func collectAllProviderIDs(spec *ProvidersSpec) map[string]bool {
	ids := make(map[string]bool)
	for _, slice := range [][]ProviderConfig{
		spec.Inference,
		spec.Safety,
		spec.VectorIo,
		spec.ToolRuntime,
	} {
		for _, pc := range slice {
			id := pc.ID
			if id == "" {
				id = deriveProviderID(pc.Provider)
			}
			ids[id] = true
		}
	}
	return ids
}

func sortedMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
