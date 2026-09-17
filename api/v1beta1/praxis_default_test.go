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

// RHAIENG-6602 — Praxis mode is off by default.
//
// RHAIENG-7517 flipped the operator default: every OGXServer CR, new and upgraded alike, runs in
// legacy mode unless its author writes spec.praxisMode. Responses in Praxis is not ready for 3.6,
// so OGX must keep serving /v1/responses standalone, and nothing in the admission path may quietly
// opt a CR in.
//
// These are static reads of generated artifacts plus a pure-function table, so they cost
// milliseconds and cannot flake. They fail at PR time — on the `make manifests` / installer diff —
// before a cluster is involved. The live counterparts (the deployed webhook actually leaving
// spec.praxisMode unset, OGX actually serving /v1/responses) are in tests/e2e/greenfield_default_test.go.

package v1beta1

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"sigs.k8s.io/yaml"
)

// admissionManifestDoc is the subset of an admission webhook configuration these tests read.
type admissionManifestDoc struct {
	Kind     string `json:"kind"`
	Metadata struct {
		Name string `json:"name"`
	} `json:"metadata"`
	Webhooks []struct {
		Name  string `json:"name"`
		Rules []struct {
			APIGroups  []string `json:"apiGroups"`
			Resources  []string `json:"resources"`
			Operations []string `json:"operations"`
		} `json:"rules"`
	} `json:"webhooks"`
}

// generatedManifests are the artifacts that decide what a cluster actually admits: the
// controller-gen output and the two installers users apply directly.
func generatedManifests() []string {
	return []string{
		filepath.Join("..", "..", "config", "webhook", "manifests.yaml"),
		filepath.Join("..", "..", "release", "operator.yaml"),
		filepath.Join("..", "..", "release", "operator-openshift.yaml"),
	}
}

// TestNoMutatingWebhookTouchesOGXServers is the cheapest durable guard on the opt-out default: a
// cluster with no mutating webhook registered for ogxservers cannot default spec.praxisMode to
// enabled, on create or on any later update. It replaces the pre-RHAIENG-7517 guard that merely
// pinned the defaulter to CREATE.
//
// It reads the installers as well as the controller-gen output, because a stale release/ manifest
// would keep registering the webhook in a real cluster however clean config/ looks.
func TestNoMutatingWebhookTouchesOGXServers(t *testing.T) {
	for _, path := range generatedManifests() {
		t.Run(filepath.Base(path), func(t *testing.T) {
			docs := parseAdmissionDocs(t, path)

			sawValidating := false
			for _, doc := range docs {
				switch doc.Kind {
				case "ValidatingWebhookConfiguration":
					if webhookCoversOGXServers(doc) {
						sawValidating = true
					}
				case "MutatingWebhookConfiguration":
					reportMutatingWebhooksOnOGXServers(t, doc)
				}
			}

			// Without this the test would pass just as happily against an empty or misparsed file.
			if !sawValidating {
				t.Fatalf("found no ValidatingWebhookConfiguration for ogxservers in %s; the "+
					"no-mutating-webhook assertion above would be vacuous", path)
			}
		})
	}
}

// reportMutatingWebhooksOnOGXServers fails the test for every rule in doc that would let a
// mutating webhook rewrite an OGXServer.
func reportMutatingWebhooksOnOGXServers(t *testing.T, doc admissionManifestDoc) {
	t.Helper()

	for _, wh := range doc.Webhooks {
		for _, rule := range wh.Rules {
			if !ruleCoversOGXServers(rule.APIGroups, rule.Resources) {
				continue
			}
			t.Errorf("mutating webhook %q in %s/%s matches ogx.io/ogxservers on %v; nothing may "+
				"default spec.praxisMode — Praxis mode is opt-in (RHAIENG-7517)",
				wh.Name, doc.Kind, doc.Metadata.Name, rule.Operations)
		}
	}
}

func parseAdmissionDocs(t *testing.T, path string) []admissionManifestDoc {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	var docs []admissionManifestDoc
	for _, raw := range strings.Split(string(data), "\n---\n") {
		if strings.TrimSpace(raw) == "" {
			continue
		}
		var doc admissionManifestDoc
		if err := yaml.Unmarshal([]byte(raw), &doc); err != nil {
			// The installers carry every kind the operator ships; only admission configs need to
			// parse into this shape.
			continue
		}
		if doc.Kind == "MutatingWebhookConfiguration" || doc.Kind == "ValidatingWebhookConfiguration" {
			docs = append(docs, doc)
		}
	}
	return docs
}

func webhookCoversOGXServers(doc admissionManifestDoc) bool {
	for _, wh := range doc.Webhooks {
		for _, rule := range wh.Rules {
			if ruleCoversOGXServers(rule.APIGroups, rule.Resources) {
				return true
			}
		}
	}
	return false
}

// ruleCoversOGXServers reports whether an admission rule selects OGXServer, wildcards included.
func ruleCoversOGXServers(apiGroups, resources []string) bool {
	return matchesOrWildcard(apiGroups, GroupVersion.Group) && matchesOrWildcard(resources, "ogxservers")
}

func matchesOrWildcard(values []string, want string) bool {
	for _, v := range values {
		if v == "*" || v == want || strings.HasPrefix(v, want+"/") {
			return true
		}
	}
	return false
}

// TestCRDDoesNotDefaultPraxisMode pins the other half of the opt-out default. With the mutating
// webhook gone, a CRD-level default on spec.praxisMode would be the remaining way to opt every CR
// in — the apiserver applies structural-schema defaults to existing objects on read, so it would
// reach upgraded CRs too, not just new ones.
//
// The nested enabled: true default is correct and deliberate: it only fires once an author has
// written spec.praxisMode, which is the explicit opt-in. Asserting both halves pins the semantics
// rather than just the absence.
func TestCRDDoesNotDefaultPraxisMode(t *testing.T) {
	praxisMode := praxisModeSchema(t)

	if _, hasDefault := praxisMode["default"]; hasDefault {
		t.Errorf("spec.praxisMode declares a CRD default (%v); Praxis mode must be opt-in, and a "+
			"structural-schema default would opt in every CR including upgraded ones",
			praxisMode["default"])
	}

	properties, ok := praxisMode["properties"].(map[string]interface{})
	if !ok {
		t.Fatal("spec.praxisMode declares no properties; the enabled assertion below would be vacuous")
	}
	enabled, ok := properties["enabled"].(map[string]interface{})
	if !ok {
		t.Fatal("spec.praxisMode.enabled is missing from the CRD schema")
	}
	if enabled["default"] != true {
		t.Errorf("spec.praxisMode.enabled default = %v, want true — writing praxisMode: {} is the "+
			"documented way to opt in", enabled["default"])
	}
}

// praxisModeSchema returns the generated OpenAPI schema for spec.praxisMode.
func praxisModeSchema(t *testing.T) map[string]interface{} {
	t.Helper()

	path := filepath.Join("..", "..", "config", "crd", "bases", "ogx.io_ogxservers.yaml")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed to read %s: %v", path, err)
	}

	var crd struct {
		Spec struct {
			Versions []struct {
				Name   string `json:"name"`
				Schema struct {
					OpenAPIV3Schema struct {
						Properties struct {
							Spec struct {
								Properties map[string]interface{} `json:"properties"`
							} `json:"spec"`
						} `json:"properties"`
					} `json:"openAPIV3Schema"`
				} `json:"schema"`
			} `json:"versions"`
		} `json:"spec"`
	}
	if err := yaml.Unmarshal(data, &crd); err != nil {
		t.Fatalf("failed to parse the OGXServer CRD: %v", err)
	}

	for _, version := range crd.Spec.Versions {
		if version.Name != GroupVersion.Version {
			continue
		}
		praxisMode, ok := version.Schema.OpenAPIV3Schema.Properties.Spec.Properties["praxisMode"].(map[string]interface{})
		if !ok {
			t.Fatalf("spec.praxisMode is missing from the %s CRD schema", version.Name)
		}
		return praxisMode
	}

	t.Fatalf("CRD declares no %s version", GroupVersion.Version)
	return nil
}

// TestIsPraxisModeEnabled_DefaultsToDisabled covers the resolver every consumer funnels through
// (Spec.IsPraxisModeEnabled is the same predicate). An absent spec.praxisMode — what a greenfield
// CR and an upgraded CR both carry — reads as legacy, which is what keeps OGX serving
// /v1/responses in 3.6. Writing spec.praxisMode at all is the explicit opt-in: enabled then
// defaults to true, matching the CRD's structural-schema default pinned in
// TestCRDDoesNotDefaultPraxisMode.
func TestIsPraxisModeEnabled_DefaultsToDisabled(t *testing.T) {
	tests := []struct {
		name       string
		praxisMode *PraxisModeSpec
		want       bool
	}{
		{
			name:       "spec.praxisMode absent (greenfield and upgraded CRs): disabled",
			praxisMode: nil,
			want:       false,
		},
		{
			name:       "spec.praxisMode present but enabled unset: the opt-in, defaults to enabled",
			praxisMode: &PraxisModeSpec{},
			want:       true,
		},
		{
			name:       "spec.praxisMode.enabled false: disabled",
			praxisMode: &PraxisModeSpec{Enabled: ptr(false)},
			want:       false,
		},
		{
			name:       "spec.praxisMode.enabled true: the explicit opt-in",
			praxisMode: &PraxisModeSpec{Enabled: ptr(true)},
			want:       true,
		},
		{
			name: "praxisSelector without enabled still opts in, via the same default",
			praxisMode: &PraxisModeSpec{
				PraxisSelector: &PraxisSelector{Namespace: "praxis"},
			},
			want: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := &OGXServer{Spec: OGXServerSpec{
				Distribution: DistributionSpec{Name: "starter"},
				PraxisMode:   tt.praxisMode,
			}}
			if got := server.Spec.IsPraxisModeEnabled(); got != tt.want {
				t.Errorf("IsPraxisModeEnabled() = %v, want %v", got, tt.want)
			}
		})
	}
}
