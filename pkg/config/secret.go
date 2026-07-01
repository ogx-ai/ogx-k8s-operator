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
	"fmt"
	"regexp"
	"sort"
	"strings"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
	corev1 "k8s.io/api/core/v1"
)

const envVarPrefix = "OGX"

var envVarNameRegex = regexp.MustCompile(`^[A-Z_][A-Z0-9_]*$`)

// CollectSecretRefs collects all secret references from the spec and returns
// the corresponding environment variable definitions for the Deployment.
func CollectSecretRefs(spec *ogxiov1beta1.OGXServerSpec) []corev1.EnvVar {
	rawEnvVars := collectSecretRefsRaw(spec)
	unique := make(map[string]corev1.EnvVar)
	addUniqueEnvVars(unique, rawEnvVars)

	envVars := make([]corev1.EnvVar, 0, len(unique))
	for _, env := range unique {
		envVars = append(envVars, env)
	}

	// Sort for determinism
	sort.Slice(envVars, func(i, j int) bool {
		return envVars[i].Name < envVars[j].Name
	})

	return envVars
}

func collectSecretRefsRaw(spec *ogxiov1beta1.OGXServerSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	if spec.Providers != nil {
		envVars = append(envVars, collectProviderSecrets(spec.Providers)...)
	}
	if spec.Storage != nil {
		envVars = append(envVars, collectStorageSecrets(spec.Storage)...)
	}
	return envVars
}

func addUniqueEnvVars(dst map[string]corev1.EnvVar, envVars []corev1.EnvVar) {
	for _, env := range envVars {
		if _, exists := dst[env.Name]; !exists {
			dst[env.Name] = env
		}
	}
}

// collectProviderSecrets walks all provider types and collects secret references.
// NOTE: Keep in sync with buildProviderExpanders in provider.go — when adding a
// new provider API type, both functions must be updated.
func collectProviderSecrets(providers *ogxiov1beta1.ProvidersSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar

	if providers.Inference != nil {
		envVars = append(envVars, collectInferenceSecrets(providers.Inference)...)
	}
	if providers.VectorIo != nil {
		envVars = append(envVars, collectVectorIOSecrets(providers.VectorIo)...)
	}
	if providers.ToolRuntime != nil {
		envVars = append(envVars, collectToolRuntimeSecrets(providers.ToolRuntime)...)
	}
	if providers.Files != nil {
		envVars = append(envVars, collectFilesSecrets(providers.Files)...)
	}
	if providers.Batches != nil {
		envVars = append(envVars, collectBatchesSecrets(providers.Batches)...)
	}
	if providers.Responses != nil {
		envVars = append(envVars, collectResponsesSecrets(providers.Responses)...)
	}
	if providers.FileProcessors != nil {
		envVars = append(envVars, collectFileProcessorsSecrets(providers.FileProcessors)...)
	}

	return envVars
}

func collectInferenceSecrets(spec *ogxiov1beta1.InferenceProvidersSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	if spec.Remote != nil {
		envVars = append(envVars, collectRemoteInferenceSecrets(spec.Remote)...)
	}
	if spec.Inline != nil {
		for _, p := range spec.Inline.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	return envVars
}

func collectRemoteInferenceSecrets(remote *ogxiov1beta1.InferenceRemoteProviders) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	for _, p := range remote.VLLM {
		if p.APIToken != nil {
			envVars = append(envVars, secretToEnvVar(p.DeriveID(), "API_TOKEN", *p.APIToken))
		}
	}
	for _, p := range remote.OpenAI {
		envVars = append(envVars, secretToEnvVar(p.DeriveID(), "API_KEY", p.APIKey))
	}
	for _, p := range remote.Azure {
		envVars = append(envVars, secretToEnvVar(p.DeriveID(), "API_KEY", p.APIKey))
	}
	for _, p := range remote.Bedrock {
		envVars = append(envVars, collectBedrockSecrets(p)...)
	}
	for _, p := range remote.Watsonx {
		envVars = append(envVars, secretToEnvVar(p.DeriveID(), "API_KEY", p.APIKey))
	}
	for _, p := range remote.Custom {
		envVars = append(envVars, collectCustomProviderSecrets(p)...)
	}
	return envVars
}

func collectBedrockSecrets(p ogxiov1beta1.BedrockProvider) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	id := p.DeriveID()
	if p.APIKey != nil {
		envVars = append(envVars, secretToEnvVar(id, "API_KEY", *p.APIKey))
	}
	if p.AWSAccessKeyID != nil {
		envVars = append(envVars, secretToEnvVar(id, "AWS_ACCESS_KEY_ID", *p.AWSAccessKeyID))
	}
	if p.AWSSecretAccessKey != nil {
		envVars = append(envVars, secretToEnvVar(id, "AWS_SECRET_ACCESS_KEY", *p.AWSSecretAccessKey))
	}
	if p.AWSSessionToken != nil {
		envVars = append(envVars, secretToEnvVar(id, "AWS_SESSION_TOKEN", *p.AWSSessionToken))
	}
	return envVars
}

func collectVectorIOSecrets(spec *ogxiov1beta1.VectorIOProvidersSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	if spec.Remote != nil {
		for _, p := range spec.Remote.Pgvector {
			// Password is required (not a pointer)
			envVars = append(envVars, secretToEnvVar(p.DeriveID(), "PASSWORD", p.Password))
		}
		for _, p := range spec.Remote.Milvus {
			if p.Token != nil {
				envVars = append(envVars, secretToEnvVar(p.DeriveID(), "TOKEN", *p.Token))
			}
		}
		for _, p := range spec.Remote.Qdrant {
			if p.APIKey != nil {
				envVars = append(envVars, secretToEnvVar(p.DeriveID(), "API_KEY", *p.APIKey))
			}
		}
		for _, p := range spec.Remote.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	if spec.Inline != nil {
		for _, p := range spec.Inline.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	return envVars
}

func collectToolRuntimeSecrets(spec *ogxiov1beta1.ToolRuntimeProvidersSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	if spec.Remote != nil {
		for _, p := range spec.Remote.BraveSearch {
			envVars = append(envVars, secretToEnvVar(p.DeriveID(), "API_KEY", p.APIKey))
		}
		for _, p := range spec.Remote.TavilySearch {
			envVars = append(envVars, secretToEnvVar(p.DeriveID(), "API_KEY", p.APIKey))
		}
		for _, p := range spec.Remote.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	if spec.Inline != nil {
		for _, p := range spec.Inline.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	return envVars
}

func collectFilesSecrets(spec *ogxiov1beta1.FilesProvidersSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	if spec.Remote != nil {
		if spec.Remote.S3 != nil {
			p := spec.Remote.S3
			if p.AWSAccessKeyID != nil {
				envVars = append(envVars, secretToEnvVar(p.DeriveID(), "AWS_ACCESS_KEY_ID", *p.AWSAccessKeyID))
			}
			if p.AWSSecretAccessKey != nil {
				envVars = append(envVars, secretToEnvVar(p.DeriveID(), "AWS_SECRET_ACCESS_KEY", *p.AWSSecretAccessKey))
			}
		}
		for _, p := range spec.Remote.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	if spec.Inline != nil {
		for _, p := range spec.Inline.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	return envVars
}

func collectFileProcessorsSecrets(spec *ogxiov1beta1.FileProcessorsProvidersSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	if spec.Remote != nil {
		if spec.Remote.DoclingServe != nil && spec.Remote.DoclingServe.APIKey != nil {
			envVars = append(envVars, secretToEnvVar(spec.Remote.DoclingServe.DeriveID(), "API_KEY", *spec.Remote.DoclingServe.APIKey))
		}
		for _, p := range spec.Remote.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	if spec.Inline != nil {
		for _, p := range spec.Inline.Custom {
			envVars = append(envVars, collectCustomProviderSecrets(p)...)
		}
	}
	return envVars
}

func collectBatchesSecrets(spec *ogxiov1beta1.BatchesProvidersSpec) []corev1.EnvVar {
	var remote, inline []ogxiov1beta1.CustomProvider
	if spec.Remote != nil {
		remote = spec.Remote.Custom
	}
	if spec.Inline != nil {
		inline = spec.Inline.Custom
	}
	return collectCustomSecretsFromSlices(remote, inline)
}

func collectResponsesSecrets(spec *ogxiov1beta1.ResponsesProvidersSpec) []corev1.EnvVar {
	var remote, inline []ogxiov1beta1.CustomProvider
	if spec.Remote != nil {
		remote = spec.Remote.Custom
	}
	if spec.Inline != nil {
		inline = spec.Inline.Custom
	}
	return collectCustomSecretsFromSlices(remote, inline)
}

// collectCustomSecretsFromSlices collects secrets from remote and inline custom
// provider slices. Used by API types that only support custom providers.
func collectCustomSecretsFromSlices(remote, inline []ogxiov1beta1.CustomProvider) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0, len(remote)+len(inline))
	for _, p := range remote {
		envVars = append(envVars, collectCustomProviderSecrets(p)...)
	}
	for _, p := range inline {
		envVars = append(envVars, collectCustomProviderSecrets(p)...)
	}
	return envVars
}

func collectCustomProviderSecrets(p ogxiov1beta1.CustomProvider) []corev1.EnvVar {
	envVars := make([]corev1.EnvVar, 0, len(p.SecretRefs))
	keys := make([]string, 0, len(p.SecretRefs))
	for k := range p.SecretRefs {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, key := range keys {
		ref := p.SecretRefs[key]
		envVars = append(envVars, secretToEnvVar(p.DeriveID(), normalizeEnvVarField(key), ref))
	}
	return envVars
}

func collectStorageSecrets(storage *ogxiov1beta1.StateStorageSpec) []corev1.EnvVar {
	var envVars []corev1.EnvVar
	if storage.KV != nil && storage.KV.Password != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: fmt.Sprintf("%s_STORAGE_KV_PASSWORD", envVarPrefix),
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: storage.KV.Password.Name},
					Key:                  storage.KV.Password.Key,
				},
			},
		})
	}
	if storage.SQL != nil && storage.SQL.ConnectionString != nil {
		envVars = append(envVars, corev1.EnvVar{
			Name: fmt.Sprintf("%s_STORAGE_SQL_CONNECTION_STRING", envVarPrefix),
			ValueFrom: &corev1.EnvVarSource{
				SecretKeyRef: &corev1.SecretKeySelector{
					LocalObjectReference: corev1.LocalObjectReference{Name: storage.SQL.ConnectionString.Name},
					Key:                  storage.SQL.ConnectionString.Key,
				},
			},
		})
	}
	return envVars
}

func secretToEnvVar(providerID, field string, ref ogxiov1beta1.SecretKeyRef) corev1.EnvVar {
	return corev1.EnvVar{
		Name: buildEnvVarName(providerID, field),
		ValueFrom: &corev1.EnvVarSource{
			SecretKeyRef: &corev1.SecretKeySelector{
				LocalObjectReference: corev1.LocalObjectReference{Name: ref.Name},
				Key:                  ref.Key,
			},
		},
	}
}

// buildEnvVarName constructs the env var name: OGX_<PROVIDER_ID>_<FIELD>.
func buildEnvVarName(providerID, field string) string {
	return fmt.Sprintf("%s_%s_%s", envVarPrefix, normalizeEnvVarField(providerID), field)
}

// envVarRef returns the config.yaml env substitution reference string.
func envVarRef(providerID, field string) string {
	return fmt.Sprintf("${env.%s}", buildEnvVarName(providerID, field))
}

// ValidateSecretRefEnvVarNames validates generated env var names and reports collisions.
func ValidateSecretRefEnvVarNames(spec *ogxiov1beta1.OGXServerSpec) error {
	envVars := collectSecretRefsRaw(spec)
	seen := make(map[string]corev1.SecretKeySelector, len(envVars))
	for _, env := range envVars {
		if !envVarNameRegex.MatchString(env.Name) {
			return fmt.Errorf("failed to validate generated env var name %q", env.Name)
		}
		if env.ValueFrom == nil || env.ValueFrom.SecretKeyRef == nil {
			continue
		}

		ref := *env.ValueFrom.SecretKeyRef
		if existing, ok := seen[env.Name]; ok {
			if existing.Name != ref.Name || existing.Key != ref.Key {
				return fmt.Errorf(
					"failed to validate env var name collision for %q: %s/%s conflicts with %s/%s",
					env.Name, existing.Name, existing.Key, ref.Name, ref.Key,
				)
			}
			continue
		}
		seen[env.Name] = ref
	}
	return nil
}

// normalizeEnvVarField uppercases and replaces non-alphanumeric characters with underscores.
func normalizeEnvVarField(s string) string {
	upper := strings.ToUpper(s)
	out := trimAndCollapseToUnderscore(upper)
	return ensureValidEnvVarPrefix(out)
}

func trimAndCollapseToUnderscore(s string) string {
	var b strings.Builder
	b.Grow(len(s))
	lastUnderscore := false
	for _, r := range s {
		if isEnvVarAlphaNum(r) {
			b.WriteRune(r)
			lastUnderscore = false
			continue
		}
		if !lastUnderscore {
			b.WriteByte('_')
			lastUnderscore = true
		}
	}
	return strings.Trim(b.String(), "_")
}

func ensureValidEnvVarPrefix(s string) string {
	if s == "" {
		return "_"
	}
	if s[0] >= '0' && s[0] <= '9' {
		return "_" + s
	}
	return s
}

func isEnvVarAlphaNum(r rune) bool {
	return (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
}
