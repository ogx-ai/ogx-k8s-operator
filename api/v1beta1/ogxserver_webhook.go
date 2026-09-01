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

	"k8s.io/apimachinery/pkg/util/validation/field"
	ctrl "sigs.k8s.io/controller-runtime"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/webhook/admission"
)

var ogxserverlog = logf.Log.WithName("ogxserver-webhook")

// OGXServerValidator validates OGXServer resources.
type OGXServerValidator struct {
	// KnownDistributionNames is the list of valid distribution names from the
	// operator's distribution registry. Injected at setup time to avoid import
	// cycles with pkg/cluster.
	KnownDistributionNames []string
}

var _ admission.Validator[*OGXServer] = &OGXServerValidator{}

// OGXServerDefaulter defaults newly created OGXServer resources.
type OGXServerDefaulter struct{}

var _ admission.Defaulter[*OGXServer] = &OGXServerDefaulter{}

// SetupWebhookWithManager registers the validating and mutating webhooks.
// knownDistNames should be the keys from the operator's distribution registry.
func SetupWebhookWithManager(mgr ctrl.Manager, knownDistNames []string) error {
	return ctrl.NewWebhookManagedBy(mgr, &OGXServer{}).
		WithValidator(&OGXServerValidator{
			KnownDistributionNames: knownDistNames,
		}).
		WithDefaulter(&OGXServerDefaulter{}).
		Complete()
}

//nolint:lll // kubebuilder marker cannot be split across lines.
//+kubebuilder:webhook:path=/mutate-ogx-io-v1beta1-ogxserver,mutating=true,failurePolicy=fail,sideEffects=None,groups=ogx.io,resources=ogxservers,verbs=create,versions=v1beta1,name=mogxserver.kb.io,admissionReviewVersions=v1

// Default implements admission.Defaulter. It only runs on CREATE (per the
// webhook's verbs=create rule), so existing OGXServer CRs from before this
// field existed are left untouched — PraxisMode is defaulted only for
// brand-new CRs, not retroactively applied to CRs upgrading from an older
// operator version.
func (d *OGXServerDefaulter) Default(_ context.Context, r *OGXServer) error {
	if r.Spec.PraxisMode == nil {
		enabled := true
		r.Spec.PraxisMode = &PraxisModeSpec{Enabled: &enabled}
	}
	return nil
}

//nolint:lll // kubebuilder marker cannot be split across lines.
//+kubebuilder:webhook:path=/validate-ogx-io-v1beta1-ogxserver,mutating=false,failurePolicy=fail,sideEffects=None,groups=ogx.io,resources=ogxservers,verbs=create;update,versions=v1beta1,name=vogxserver.kb.io,admissionReviewVersions=v1

// ValidateCreate implements admission.Validator.
func (v *OGXServerValidator) ValidateCreate(_ context.Context, r *OGXServer) (admission.Warnings, error) {
	ogxserverlog.Info("validating create", "name", r.Name)
	return v.validate(r)
}

// ValidateUpdate implements admission.Validator.
func (v *OGXServerValidator) ValidateUpdate(_ context.Context, _ *OGXServer, r *OGXServer) (admission.Warnings, error) {
	ogxserverlog.Info("validating update", "name", r.Name)
	return v.validate(r)
}

// ValidateDelete implements admission.Validator.
func (v *OGXServerValidator) ValidateDelete(_ context.Context, _ *OGXServer) (admission.Warnings, error) {
	return nil, nil
}

func (v *OGXServerValidator) validate(r *OGXServer) (admission.Warnings, error) {
	warnings := collectValidationWarnings(r)

	allErrs := v.collectValidationErrors(r)
	if len(allErrs) > 0 {
		return warnings, allErrs.ToAggregate()
	}
	return warnings, nil
}

// collectValidationWarnings returns non-fatal admission warnings. When an OGXServer opts into
// Praxis-fronted mode (spec.praxisMode.enabled: true), OGX is internal-only, so two settings that
// would weaken that guarantee are surfaced as warnings rather than rejections (so existing CRs and
// GitOps applies do not break): requesting external access (not honored — the operator creates no
// external exposure) and disabling the operator-managed NetworkPolicy (which removes the mandatory
// Praxis ingress lock-down).
//
// Warnings are emitted only when Praxis mode is enabled. On create the mutating webhook has
// already defaulted an unset spec.praxisMode to enabled, so this reflects the effective mode; when
// Praxis mode is disabled (or unset on an upgrade, meaning legacy), these settings may be honored,
// so a warning would be misleading.
func collectValidationWarnings(r *OGXServer) admission.Warnings {
	// Both warnings only apply in Praxis-fronted mode; in legacy mode these settings may be honored.
	if !isPraxisModeEnabled(r) {
		return nil
	}

	var warnings admission.Warnings

	if isExternalAccessRequested(r) {
		warnings = append(warnings,
			"spec.network.externalAccess.enabled is not honored: OGX is internal-only (Praxis-fronted, "+
				"spec.praxisMode.enabled: true) and the operator does not create external exposure. "+
				"This value is treated as false.")
	}

	// In Praxis mode the NetworkPolicy enforces the mandatory ingress lock-down (Praxis pods plus
	// the operator namespace). Setting spec.network.policy.enabled: false disables the policy
	// entirely, removing that lock-down and leaving OGX reachable by any co-located workload.
	if isNetworkPolicyDisabled(r) {
		warnings = append(warnings,
			"spec.network.policy.enabled: false disables the operator-managed NetworkPolicy entirely, "+
				"removing the mandatory Praxis-fronted ingress lock-down (OGX is internal-only, "+
				"spec.praxisMode.enabled: true). OGX may then be reachable directly by co-located "+
				"workloads, bypassing Praxis. Prefer additive spec.network.policy.ingress rules instead.")
	}

	return warnings
}

// isPraxisModeEnabled reports whether spec.praxisMode.enabled is explicitly true.
func isPraxisModeEnabled(r *OGXServer) bool {
	return r.Spec.PraxisMode != nil &&
		r.Spec.PraxisMode.Enabled != nil && *r.Spec.PraxisMode.Enabled
}

// isExternalAccessRequested reports whether spec.network.externalAccess.enabled is true.
func isExternalAccessRequested(r *OGXServer) bool {
	return r.Spec.Network != nil &&
		r.Spec.Network.ExternalAccess != nil &&
		r.Spec.Network.ExternalAccess.Enabled
}

// isNetworkPolicyDisabled reports whether spec.network.policy.enabled is explicitly false.
func isNetworkPolicyDisabled(r *OGXServer) bool {
	return r.Spec.Network != nil &&
		r.Spec.Network.Policy != nil &&
		r.Spec.Network.Policy.Enabled != nil && !*r.Spec.Network.Policy.Enabled
}

func (v *OGXServerValidator) collectValidationErrors(r *OGXServer) field.ErrorList {
	var allErrs field.ErrorList

	if r.Spec.Distribution.Name != "" {
		allErrs = append(allErrs, validateDistributionName(r.Spec.Distribution.Name, v.KnownDistributionNames)...)
	}

	if r.Spec.Providers != nil {
		allErrs = append(allErrs, validateProviderIDs(r.Spec.Providers)...)
	}

	if r.Spec.Resources != nil && r.Spec.Providers != nil {
		allErrs = append(allErrs, validateProviderReferences(r.Spec.Resources, r.Spec.Providers)...)
	}

	allErrs = append(allErrs, validateAdoptionAnnotations(r)...)

	return allErrs
}

// validateAdoptionAnnotations rejects adoption annotations whose value equals
// the CR name. Same-name adoption causes Deployment name conflicts and is not
// a supported migration path.
func validateAdoptionAnnotations(r *OGXServer) field.ErrorList {
	var errs field.ErrorList
	annotationPath := field.NewPath("metadata", "annotations")

	for _, key := range []string{AdoptStorageAnnotation, AdoptNetworkingAnnotation} {
		if val, ok := r.Annotations[key]; ok && val == r.Name {
			errs = append(errs, field.Invalid(
				annotationPath.Key(key), val,
				"adoption annotation value must not equal the CR name; same-name adoption causes resource conflicts",
			))
		}
	}

	return errs
}

// collectAllProviderIDs returns all provider IDs and any duplicate ID errors.
func collectAllProviderIDs(spec *ProvidersSpec) (map[string]bool, field.ErrorList) {
	if spec == nil {
		return nil, nil
	}

	all := spec.IDs()
	seen := make(map[string]bool, len(all))
	var errs field.ErrorList
	for _, id := range all {
		if seen[id] {
			errs = append(errs, field.Invalid(
				field.NewPath("spec", "providers"), id,
				fmt.Sprintf("duplicate provider ID %q", id),
			))
		}
		seen[id] = true
	}
	return seen, errs
}

// validateProviderIDs validates provider ID uniqueness and that all
// multi-instance provider slices have explicit IDs.
func validateProviderIDs(spec *ProvidersSpec) field.ErrorList {
	_, errs := collectAllProviderIDs(spec)
	return errs
}

// validateProviderReferences ensures model provider references point to configured providers.
func validateProviderReferences(resources *ResourcesSpec, providers *ProvidersSpec) field.ErrorList {
	var errs field.ErrorList

	providerIDs, _ := collectAllProviderIDs(providers)

	for i, mc := range resources.Models {
		if mc.Provider != "" {
			if !providerIDs[mc.Provider] {
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

// validateDistributionName validates that distribution.name is in the operator
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

func sortedMapKeys(m map[string]bool) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
