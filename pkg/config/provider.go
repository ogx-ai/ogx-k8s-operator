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
	"encoding/json"
	"fmt"

	ogxiov1beta1 "github.com/ogx-ai/ogx-k8s-operator/api/v1beta1"
)

// providerExpander pairs an API type name with a function that expands the spec for that type.
type providerExpander struct {
	apiType string
	expand  func() ([]ConfigProvider, error)
}

// ExpandProviders converts the typed ProvidersSpec into config.yaml provider sections.
// Returns a map keyed by API type (e.g., "inference", "vector_io") to provider lists.
func ExpandProviders(providers *ogxiov1beta1.ProvidersSpec) (map[string][]ConfigProvider, error) {
	if providers == nil {
		return nil, nil
	}

	expanders := buildProviderExpanders(providers)
	result := make(map[string][]ConfigProvider, len(expanders))
	for _, e := range expanders {
		ps, err := e.expand()
		if err != nil {
			return nil, fmt.Errorf("failed to expand %s providers: %w", e.apiType, err)
		}
		if len(ps) > 0 {
			result[e.apiType] = ps
		}
	}
	return result, nil
}

// buildProviderExpanders returns one expander per configured API type.
// NOTE: Keep in sync with collectProviderSecrets in secret.go — when adding a
// new provider API type, both functions must be updated.
func buildProviderExpanders(p *ogxiov1beta1.ProvidersSpec) []providerExpander {
	var out []providerExpander
	if p.Inference != nil {
		spec := p.Inference
		out = append(out, providerExpander{"inference", func() ([]ConfigProvider, error) { return expandInferenceProviders(spec) }})
	}
	if p.VectorIo != nil {
		spec := p.VectorIo
		out = append(out, providerExpander{"vector_io", func() ([]ConfigProvider, error) { return expandVectorIOProviders(spec) }})
	}
	if p.ToolRuntime != nil {
		spec := p.ToolRuntime
		out = append(out, providerExpander{"tool_runtime", func() ([]ConfigProvider, error) { return expandToolRuntimeProviders(spec) }})
	}
	if p.Files != nil {
		spec := p.Files
		out = append(out, providerExpander{"files", func() ([]ConfigProvider, error) { return expandFilesProviders(spec) }})
	}
	if p.Batches != nil {
		spec := p.Batches
		out = append(out, providerExpander{"batches", func() ([]ConfigProvider, error) { return expandBatchesProviders(spec) }})
	}
	if p.Responses != nil {
		spec := p.Responses
		out = append(out, providerExpander{"responses", func() ([]ConfigProvider, error) { return expandResponsesProviders(spec) }})
	}
	if p.FileProcessors != nil {
		spec := p.FileProcessors
		out = append(out, providerExpander{"file_processors", func() ([]ConfigProvider, error) { return expandFileProcessorsProviders(spec) }})
	}
	return out
}

func expandInferenceProviders(spec *ogxiov1beta1.InferenceProvidersSpec) ([]ConfigProvider, error) {
	var providers []ConfigProvider

	if spec.Remote != nil {
		providers = expandRemoteInferenceProviders(spec.Remote, providers)
		if err := appendCustomProviders(&providers, spec.Remote.Custom); err != nil {
			return nil, err
		}
	}
	if spec.Inline != nil {
		if err := appendCustomProviders(&providers, spec.Inline.Custom); err != nil {
			return nil, err
		}
	}

	return providers, nil
}

func expandRemoteInferenceProviders(remote *ogxiov1beta1.InferenceRemoteProviders, providers []ConfigProvider) []ConfigProvider {
	for _, p := range remote.VLLM {
		providers = append(providers, expandVLLMProvider(p))
	}
	for _, p := range remote.OpenAI {
		providers = append(providers, expandOpenAIProvider(p))
	}
	for _, p := range remote.Azure {
		providers = append(providers, expandAzureProvider(p))
	}
	for _, p := range remote.Bedrock {
		providers = append(providers, expandBedrockProvider(p))
	}
	for _, p := range remote.VertexAI {
		providers = append(providers, expandVertexAIProvider(p))
	}
	for _, p := range remote.Watsonx {
		providers = append(providers, expandWatsonxProvider(p))
	}
	return providers
}

func appendCustomProviders(providers *[]ConfigProvider, custom []ogxiov1beta1.CustomProvider) error {
	for _, p := range custom {
		cp, err := expandCustomProvider(p)
		if err != nil {
			return err
		}
		*providers = append(*providers, cp)
	}
	return nil
}

func expandVLLMProvider(p ogxiov1beta1.VLLMProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"base_url": p.Endpoint,
	}
	if p.MaxTokens != nil {
		cfg["max_tokens"] = *p.MaxTokens
	}
	if p.APIToken != nil {
		cfg["api_token"] = envVarRef(p.DeriveID(), "API_TOKEN")
	}
	applyCommonInferenceConfig(cfg, p.RemoteInferenceCommonConfig)

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::vllm",
		Config:       cfg,
	}
}

func expandOpenAIProvider(p ogxiov1beta1.OpenAIProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"api_key": envVarRef(p.DeriveID(), "API_KEY"),
	}
	if p.Endpoint != "" {
		cfg["base_url"] = p.Endpoint
	}
	applyCommonInferenceConfig(cfg, p.RemoteInferenceCommonConfig)

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::openai",
		Config:       cfg,
	}
}

func expandAzureProvider(p ogxiov1beta1.AzureProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"base_url": p.Endpoint,
		"api_key":  envVarRef(p.DeriveID(), "API_KEY"),
	}
	if p.APIVersion != "" {
		cfg["api_version"] = p.APIVersion
	}
	if p.APIType != "" {
		cfg["api_type"] = p.APIType
	}
	applyCommonInferenceConfig(cfg, p.RemoteInferenceCommonConfig)

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::azure",
		Config:       cfg,
	}
}

func expandBedrockProvider(p ogxiov1beta1.BedrockProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"region_name": p.Region,
	}
	id := p.DeriveID()
	setSecretRef(cfg, "api_key", id, "API_KEY", p.APIKey)
	setSecretRef(cfg, "aws_access_key_id", id, "AWS_ACCESS_KEY_ID", p.AWSAccessKeyID)
	setSecretRef(cfg, "aws_secret_access_key", id, "AWS_SECRET_ACCESS_KEY", p.AWSSecretAccessKey)
	setSecretRef(cfg, "aws_session_token", id, "AWS_SESSION_TOKEN", p.AWSSessionToken)
	setString(cfg, "aws_role_arn", p.AWSRoleArn)
	setString(cfg, "aws_web_identity_token_file", p.AWSWebIdentityTokenFile)
	setString(cfg, "aws_role_session_name", p.AWSRoleSessionName)
	setString(cfg, "profile_name", p.ProfileName)
	setString(cfg, "retry_mode", p.RetryMode)
	setIntPtr(cfg, "total_max_attempts", p.TotalMaxAttempts)
	setIntPtr(cfg, "connect_timeout", p.ConnectTimeout)
	setIntPtr(cfg, "read_timeout", p.ReadTimeout)
	setIntPtr(cfg, "session_ttl", p.SessionTTL)
	applyCommonInferenceConfig(cfg, p.RemoteInferenceCommonConfig)

	return ConfigProvider{
		ProviderID:   id,
		ProviderType: "remote::bedrock",
		Config:       cfg,
	}
}

func applyCommonInferenceConfig(cfg map[string]interface{}, c ogxiov1beta1.RemoteInferenceCommonConfig) {
	if c.RefreshModels != nil {
		cfg["refresh_models"] = *c.RefreshModels
	}
	if len(c.AllowedModels) > 0 {
		cfg["allowed_models"] = c.AllowedModels
	}
	applyNetworkConfig(cfg, c.Network)
}

func setSecretRef(cfg map[string]interface{}, key, providerID, field string, ref *ogxiov1beta1.SecretKeyRef) {
	if ref != nil {
		cfg[key] = envVarRef(providerID, field)
	}
}

func setString(cfg map[string]interface{}, key, val string) {
	if val != "" {
		cfg[key] = val
	}
}

func setIntPtr(cfg map[string]interface{}, key string, val *int) {
	if val != nil {
		cfg[key] = *val
	}
}

func expandVertexAIProvider(p ogxiov1beta1.VertexAIProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"project": p.Project,
	}
	if p.Location != "" {
		cfg["location"] = p.Location
	}
	applyCommonInferenceConfig(cfg, p.RemoteInferenceCommonConfig)

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::vertexai",
		Config:       cfg,
	}
}

func expandWatsonxProvider(p ogxiov1beta1.WatsonxProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"api_key": envVarRef(p.DeriveID(), "API_KEY"),
	}
	if p.Endpoint != "" {
		cfg["base_url"] = p.Endpoint
	}
	if p.ProjectID != "" {
		cfg["project_id"] = p.ProjectID
	}
	if p.Timeout != nil {
		cfg["timeout"] = *p.Timeout
	}
	applyCommonInferenceConfig(cfg, p.RemoteInferenceCommonConfig)

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::watsonx",
		Config:       cfg,
	}
}

func expandCustomProvider(p ogxiov1beta1.CustomProvider) (ConfigProvider, error) {
	cfg := make(map[string]interface{})

	for key := range p.SecretRefs {
		cfg[key] = envVarRef(p.DeriveID(), normalizeEnvVarField(key))
	}

	if p.Settings != nil && p.Settings.Raw != nil {
		var settings map[string]interface{}
		if err := json.Unmarshal(p.Settings.Raw, &settings); err != nil {
			return ConfigProvider{}, fmt.Errorf("failed to unmarshal settings JSON for provider %q: %w", p.DeriveID(), err)
		}
		for k, v := range settings {
			if _, conflict := p.SecretRefs[k]; conflict {
				return ConfigProvider{}, fmt.Errorf("failed to expand provider %q: key %q appears in both secretRefs and settings", p.DeriveID(), k)
			}
			cfg[k] = v
		}
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: p.Type,
		Config:       cfg,
	}, nil
}

func expandVectorIOProviders(spec *ogxiov1beta1.VectorIOProvidersSpec) ([]ConfigProvider, error) {
	var providers []ConfigProvider
	if spec.Remote != nil {
		for _, p := range spec.Remote.Pgvector {
			providers = append(providers, expandPgvectorProvider(p))
		}
		for _, p := range spec.Remote.Milvus {
			providers = append(providers, expandMilvusProvider(p))
		}
		for _, p := range spec.Remote.Qdrant {
			providers = append(providers, expandQdrantProvider(p))
		}
		if err := appendCustomProviders(&providers, spec.Remote.Custom); err != nil {
			return nil, err
		}
	}
	if spec.Inline != nil {
		if err := appendCustomProviders(&providers, spec.Inline.Custom); err != nil {
			return nil, err
		}
	}
	return providers, nil
}

func expandPgvectorProvider(p ogxiov1beta1.PgvectorProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	if p.Host != "" {
		cfg["host"] = p.Host
	}
	if p.Port != nil {
		cfg["port"] = *p.Port
	}
	if p.DB != "" {
		cfg["db"] = p.DB
	}
	if p.User != "" {
		cfg["user"] = p.User
	}
	cfg["password"] = envVarRef(p.DeriveID(), "PASSWORD")
	if p.DistanceMetric != "" {
		cfg["distance_metric"] = p.DistanceMetric
	}
	if vectorIndex := expandPgvectorVectorIndex(p.VectorIndex); vectorIndex != nil {
		cfg["vector_index"] = vectorIndex
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::pgvector",
		Config:       cfg,
	}
}

func expandPgvectorVectorIndex(vectorIndex *ogxiov1beta1.VectorIndexConfig) map[string]interface{} {
	if vectorIndex == nil {
		return nil
	}

	cfg := map[string]interface{}{}
	if hnsw := expandPgvectorHNSW(vectorIndex.HNSW); hnsw != nil {
		cfg["hnsw"] = hnsw
	}
	if ivfFlat := expandPgvectorIVFFlat(vectorIndex.IVFFlat); ivfFlat != nil {
		cfg["ivf_flat"] = ivfFlat
	}
	if len(cfg) == 0 {
		return nil
	}
	return cfg
}

func expandPgvectorHNSW(hnsw *ogxiov1beta1.HNSWConfig) map[string]interface{} {
	if hnsw == nil {
		return nil
	}

	cfg := map[string]interface{}{}
	if hnsw.M != nil {
		cfg["m"] = *hnsw.M
	}
	if hnsw.EfConstruction != nil {
		cfg["ef_construction"] = *hnsw.EfConstruction
	}
	if hnsw.EfSearch != nil {
		cfg["ef_search"] = *hnsw.EfSearch
	}
	if len(cfg) == 0 {
		return nil
	}
	return cfg
}

func expandPgvectorIVFFlat(ivfFlat *ogxiov1beta1.IVFFlatConfig) map[string]interface{} {
	if ivfFlat == nil {
		return nil
	}

	cfg := map[string]interface{}{}
	if ivfFlat.Nlist != nil {
		cfg["nlist"] = *ivfFlat.Nlist
	}
	if ivfFlat.Nprobe != nil {
		cfg["nprobe"] = *ivfFlat.Nprobe
	}
	if len(cfg) == 0 {
		return nil
	}
	return cfg
}

func expandMilvusProvider(p ogxiov1beta1.MilvusProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"uri": p.URI,
	}
	if p.Token != nil {
		cfg["token"] = envVarRef(p.DeriveID(), "TOKEN")
	}
	if p.ConsistencyLevel != "" {
		cfg["consistency_level"] = p.ConsistencyLevel
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::milvus",
		Config:       cfg,
	}
}

func expandQdrantProvider(p ogxiov1beta1.QdrantProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	applyQdrantConnectionConfig(cfg, p)
	applyQdrantAuthConfig(cfg, p)
	applyQdrantRuntimeConfig(cfg, p)

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::qdrant",
		Config:       cfg,
	}
}

func applyQdrantConnectionConfig(cfg map[string]interface{}, p ogxiov1beta1.QdrantProvider) {
	if p.URL != "" {
		cfg["url"] = p.URL
	}
	if p.Host != "" {
		cfg["host"] = p.Host
	}
	if p.Port != nil {
		cfg["port"] = *p.Port
	}
	if p.Location != "" {
		cfg["location"] = p.Location
	}
}

func applyQdrantAuthConfig(cfg map[string]interface{}, p ogxiov1beta1.QdrantProvider) {
	if p.APIKey != nil {
		cfg["api_key"] = envVarRef(p.DeriveID(), "API_KEY")
	}
}

func applyQdrantRuntimeConfig(cfg map[string]interface{}, p ogxiov1beta1.QdrantProvider) {
	if p.GRPCPort != nil {
		cfg["grpc_port"] = *p.GRPCPort
	}
	if p.PreferGRPC != nil {
		cfg["prefer_grpc"] = *p.PreferGRPC
	}
	if p.HTTPS != nil {
		cfg["https"] = *p.HTTPS
	}
	if p.Prefix != "" {
		cfg["prefix"] = p.Prefix
	}
	if p.Timeout != nil {
		cfg["timeout"] = *p.Timeout
	}
}

func expandToolRuntimeProviders(spec *ogxiov1beta1.ToolRuntimeProvidersSpec) ([]ConfigProvider, error) {
	var providers []ConfigProvider
	if spec.Remote != nil {
		for _, p := range spec.Remote.BraveSearch {
			providers = append(providers, expandBraveSearchProvider(p))
		}
		for _, p := range spec.Remote.TavilySearch {
			providers = append(providers, expandTavilySearchProvider(p))
		}
		for _, p := range spec.Remote.ModelContextProtocol {
			providers = append(providers, expandMCPProvider(p))
		}
		if err := appendCustomProviders(&providers, spec.Remote.Custom); err != nil {
			return nil, err
		}
	}
	if spec.Inline != nil {
		for _, p := range spec.Inline.FileSearch {
			providers = append(providers, expandInlineFileSearchProvider(p))
		}
		if err := appendCustomProviders(&providers, spec.Inline.Custom); err != nil {
			return nil, err
		}
	}
	return providers, nil
}

func expandBraveSearchProvider(p ogxiov1beta1.BraveSearchProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"api_key": envVarRef(p.DeriveID(), "API_KEY"),
	}
	if p.MaxResults != nil {
		cfg["max_results"] = *p.MaxResults
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::brave-search",
		Config:       cfg,
	}
}

func expandTavilySearchProvider(p ogxiov1beta1.TavilySearchProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"api_key": envVarRef(p.DeriveID(), "API_KEY"),
	}
	if p.MaxResults != nil {
		cfg["max_results"] = *p.MaxResults
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::tavily-search",
		Config:       cfg,
	}
}

func expandMCPProvider(p ogxiov1beta1.ModelContextProtocolProvider) ConfigProvider {
	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::model-context-protocol",
		Config:       map[string]interface{}{},
	}
}

func expandInlineFileSearchProvider(p ogxiov1beta1.InlineFileSearchProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	if p.VectorStoresConfig != nil {
		cfg["vector_stores_config"] = expandVectorStoresConfig(p.VectorStoresConfig)
	}
	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "inline::rag-runtime",
		Config:       cfg,
	}
}

func expandFilesProviders(spec *ogxiov1beta1.FilesProvidersSpec) ([]ConfigProvider, error) {
	var providers []ConfigProvider
	if spec.Remote != nil {
		if spec.Remote.S3 != nil {
			providers = append(providers, expandS3Provider(*spec.Remote.S3))
		}
		if err := appendCustomProviders(&providers, spec.Remote.Custom); err != nil {
			return nil, err
		}
	}
	if spec.Inline != nil {
		if spec.Inline.LocalFS != nil {
			providers = append(providers, expandLocalFSProvider(*spec.Inline.LocalFS))
		}
		if err := appendCustomProviders(&providers, spec.Inline.Custom); err != nil {
			return nil, err
		}
	}
	return providers, nil
}

func expandS3Provider(p ogxiov1beta1.S3Provider) ConfigProvider {
	cfg := map[string]interface{}{
		"bucket_name": p.BucketName,
	}
	if p.Region != "" {
		cfg["region"] = p.Region
	}
	if p.EndpointURL != "" {
		cfg["endpoint_url"] = p.EndpointURL
	}
	if p.AWSAccessKeyID != nil {
		cfg["aws_access_key_id"] = envVarRef(p.DeriveID(), "AWS_ACCESS_KEY_ID")
	}
	if p.AWSSecretAccessKey != nil {
		cfg["aws_secret_access_key"] = envVarRef(p.DeriveID(), "AWS_SECRET_ACCESS_KEY")
	}
	if p.AutoCreateBucket != nil {
		cfg["auto_create_bucket"] = *p.AutoCreateBucket
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::s3",
		Config:       cfg,
	}
}

func expandLocalFSProvider(p ogxiov1beta1.InlineLocalFSProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	if p.TTLSecs != nil {
		cfg["ttl_secs"] = *p.TTLSecs
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "inline::localfs",
		Config:       cfg,
	}
}

func expandBatchesProviders(spec *ogxiov1beta1.BatchesProvidersSpec) ([]ConfigProvider, error) {
	var providers []ConfigProvider
	if spec.Remote != nil {
		if err := appendCustomProviders(&providers, spec.Remote.Custom); err != nil {
			return nil, err
		}
	}
	if spec.Inline != nil {
		if spec.Inline.Reference != nil {
			providers = append(providers, expandReferenceProvider(*spec.Inline.Reference))
		}
		if err := appendCustomProviders(&providers, spec.Inline.Custom); err != nil {
			return nil, err
		}
	}
	return providers, nil
}

func expandReferenceProvider(p ogxiov1beta1.InlineReferenceProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	if p.MaxConcurrentBatches != nil {
		cfg["max_concurrent_batches"] = *p.MaxConcurrentBatches
	}
	if p.MaxConcurrentRequestsPerBatch != nil {
		cfg["max_concurrent_requests_per_batch"] = *p.MaxConcurrentRequestsPerBatch
	}
	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "inline::batch-reference",
		Config:       cfg,
	}
}

func expandResponsesProviders(spec *ogxiov1beta1.ResponsesProvidersSpec) ([]ConfigProvider, error) {
	var providers []ConfigProvider
	if spec.Remote != nil {
		if err := appendCustomProviders(&providers, spec.Remote.Custom); err != nil {
			return nil, err
		}
	}
	if spec.Inline != nil {
		if spec.Inline.Builtin != nil {
			providers = append(providers, expandBuiltinResponsesProvider(*spec.Inline.Builtin))
		}
		if err := appendCustomProviders(&providers, spec.Inline.Custom); err != nil {
			return nil, err
		}
	}
	return providers, nil
}

func expandBuiltinResponsesProvider(p ogxiov1beta1.InlineBuiltinResponsesProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	if p.VectorStoresConfig != nil {
		cfg["vector_stores_config"] = expandVectorStoresConfig(p.VectorStoresConfig)
	}
	if p.CompactionConfig != nil {
		cfg["compaction_config"] = expandCompactionConfig(p.CompactionConfig)
	}
	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "inline::responses",
		Config:       cfg,
	}
}

func expandFileProcessorsProviders(spec *ogxiov1beta1.FileProcessorsProvidersSpec) ([]ConfigProvider, error) {
	var providers []ConfigProvider
	if spec.Remote != nil {
		if spec.Remote.DoclingServe != nil {
			providers = append(providers, expandDoclingServeProvider(*spec.Remote.DoclingServe))
		}
		if err := appendCustomProviders(&providers, spec.Remote.Custom); err != nil {
			return nil, err
		}
	}
	if spec.Inline != nil {
		if spec.Inline.Auto != nil {
			providers = append(providers, expandAutoFileProcessorProvider(*spec.Inline.Auto))
		}
		if spec.Inline.PyPDF != nil {
			providers = append(providers, expandPyPDFFileProcessorProvider(*spec.Inline.PyPDF))
		}
		if spec.Inline.MarkItDown != nil {
			providers = append(providers, expandMarkItDownFileProcessorProvider(*spec.Inline.MarkItDown))
		}
		if spec.Inline.Docling != nil {
			providers = append(providers, expandDoclingFileProcessorProvider(*spec.Inline.Docling))
		}
		if err := appendCustomProviders(&providers, spec.Inline.Custom); err != nil {
			return nil, err
		}
	}
	return providers, nil
}

func expandDoclingServeProvider(p ogxiov1beta1.DoclingServeProvider) ConfigProvider {
	cfg := map[string]interface{}{
		"base_url": p.BaseURL,
	}
	if p.APIKey != nil {
		cfg["api_key"] = envVarRef(p.DeriveID(), "API_KEY")
	}
	setIntPtr(cfg, "default_chunk_size_tokens", p.DefaultChunkSizeTokens)

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::docling-serve",
		Config:       cfg,
	}
}

func expandAutoFileProcessorProvider(p ogxiov1beta1.InlineAutoFileProcessorProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	setIntPtr(cfg, "default_chunk_size_tokens", p.DefaultChunkSizeTokens)
	setIntPtr(cfg, "default_chunk_overlap_tokens", p.DefaultChunkOverlapTokens)
	if p.ExtractMetadata != nil {
		cfg["extract_metadata"] = *p.ExtractMetadata
	}
	if p.CleanText != nil {
		cfg["clean_text"] = *p.CleanText
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "inline::auto",
		Config:       cfg,
	}
}

func expandPyPDFFileProcessorProvider(p ogxiov1beta1.InlinePyPDFFileProcessorProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	setIntPtr(cfg, "default_chunk_size_tokens", p.DefaultChunkSizeTokens)
	setIntPtr(cfg, "default_chunk_overlap_tokens", p.DefaultChunkOverlapTokens)
	if p.ExtractMetadata != nil {
		cfg["extract_metadata"] = *p.ExtractMetadata
	}
	if p.CleanText != nil {
		cfg["clean_text"] = *p.CleanText
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "inline::pypdf",
		Config:       cfg,
	}
}

func expandMarkItDownFileProcessorProvider(p ogxiov1beta1.InlineMarkItDownFileProcessorProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	setIntPtr(cfg, "default_chunk_size_tokens", p.DefaultChunkSizeTokens)
	setIntPtr(cfg, "default_chunk_overlap_tokens", p.DefaultChunkOverlapTokens)

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "inline::markitdown",
		Config:       cfg,
	}
}

func expandDoclingFileProcessorProvider(p ogxiov1beta1.InlineDoclingFileProcessorProvider) ConfigProvider {
	cfg := map[string]interface{}{}
	setIntPtr(cfg, "default_chunk_size_tokens", p.DefaultChunkSizeTokens)
	setIntPtr(cfg, "default_chunk_overlap_tokens", p.DefaultChunkOverlapTokens)
	if p.DoOCR != nil {
		cfg["do_ocr"] = *p.DoOCR
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "inline::docling",
		Config:       cfg,
	}
}

func applyNetworkConfig(cfg map[string]interface{}, network *ogxiov1beta1.NetworkConfig) {
	if network == nil {
		return
	}
	n := map[string]interface{}{}
	applyTLSConfig(n, network.TLS)
	applyTimeoutConfig(n, network.Timeout)
	if len(network.Headers) > 0 {
		n["headers"] = network.Headers
	}
	applyProxyConfig(n, network.Proxy)
	if len(n) > 0 {
		cfg["network"] = n
	}
}

func applyProxyConfig(cfg map[string]interface{}, proxy *ogxiov1beta1.ProxyConfig) {
	if proxy == nil {
		return
	}
	m := map[string]interface{}{}
	if proxy.URL != nil {
		m["url"] = *proxy.URL
	}
	if proxy.HTTP != nil {
		m["http"] = *proxy.HTTP
	}
	if proxy.HTTPS != nil {
		m["https"] = *proxy.HTTPS
	}
	if proxy.CACert != nil {
		m["ca_cert"] = *proxy.CACert
	}
	if len(proxy.NoProxy) > 0 {
		m["no_proxy"] = proxy.NoProxy
	}
	if len(m) > 0 {
		cfg["proxy"] = m
	}
}

func expandVectorStoresConfig(vs *ogxiov1beta1.VectorStoresConfig) map[string]interface{} {
	m := map[string]interface{}{}
	applyVectorStoreDefaults(m, vs)
	applyVectorStoreSearchConfig(m, vs)
	applyVectorStorePromptConfig(m, vs)
	applyVectorStoreIngestionConfig(m, vs)
	return m
}

func applyVectorStoreDefaults(cfg map[string]interface{}, vs *ogxiov1beta1.VectorStoresConfig) {
	if vs.DefaultProviderID != "" {
		cfg["default_provider_id"] = vs.DefaultProviderID
	}
	if vs.DefaultEmbeddingModel != nil {
		cfg["default_embedding_model"] = expandQualifiedModel(vs.DefaultEmbeddingModel)
	}
	if vs.DefaultRerankerModel != nil {
		cfg["default_reranker_model"] = expandRerankerModel(vs.DefaultRerankerModel)
	}
}

func applyVectorStoreSearchConfig(cfg map[string]interface{}, vs *ogxiov1beta1.VectorStoresConfig) {
	if vs.RewriteQueryParams != nil {
		cfg["rewrite_query_params"] = expandRewriteQueryParams(vs.RewriteQueryParams)
	}
	if vs.FileSearchParams != nil {
		cfg["file_search_params"] = expandFileSearchParams(vs.FileSearchParams)
	}
	if vs.ChunkRetrievalParams != nil {
		cfg["chunk_retrieval_params"] = expandChunkRetrievalParams(vs.ChunkRetrievalParams)
	}
}

func applyVectorStorePromptConfig(cfg map[string]interface{}, vs *ogxiov1beta1.VectorStoresConfig) {
	if vs.ContextPromptParams != nil {
		cfg["context_prompt_params"] = expandContextPromptParams(vs.ContextPromptParams)
	}
	if vs.AnnotationPromptParams != nil {
		cfg["annotation_prompt_params"] = expandAnnotationPromptParams(vs.AnnotationPromptParams)
	}
}

func applyVectorStoreIngestionConfig(cfg map[string]interface{}, vs *ogxiov1beta1.VectorStoresConfig) {
	if vs.FileIngestionParams != nil {
		cfg["file_ingestion_params"] = expandFileIngestionParams(vs.FileIngestionParams)
	}
	if vs.FileBatchParams != nil {
		cfg["file_batch_params"] = expandFileBatchParams(vs.FileBatchParams)
	}
	if vs.ContextualRetrievalParams != nil {
		cfg["contextual_retrieval_params"] = expandContextualRetrievalParams(vs.ContextualRetrievalParams)
	}
}

func expandRerankerModel(model *ogxiov1beta1.RerankerModel) map[string]interface{} {
	return map[string]interface{}{
		"provider_id": model.ProviderID,
		"model_id":    model.ModelID,
	}
}

func expandRewriteQueryParams(params *ogxiov1beta1.RewriteQueryParams) map[string]interface{} {
	cfg := map[string]interface{}{}
	if params.Model != nil {
		cfg["model"] = expandQualifiedModel(params.Model)
	}
	if params.Prompt != "" {
		cfg["prompt"] = params.Prompt
	}
	if params.MaxTokens != nil {
		cfg["max_tokens"] = *params.MaxTokens
	}
	if params.Temperature != nil {
		cfg["temperature"] = *params.Temperature
	}
	return cfg
}

func expandFileSearchParams(params *ogxiov1beta1.FileSearchDisplayParams) map[string]interface{} {
	cfg := map[string]interface{}{}
	if params.HeaderTemplate != "" {
		cfg["header_template"] = params.HeaderTemplate
	}
	if params.FooterTemplate != "" {
		cfg["footer_template"] = params.FooterTemplate
	}
	return cfg
}

func expandContextPromptParams(params *ogxiov1beta1.ContextPromptParams) map[string]interface{} {
	cfg := map[string]interface{}{}
	if params.ChunkAnnotationTemplate != "" {
		cfg["chunk_annotation_template"] = params.ChunkAnnotationTemplate
	}
	if params.ContextTemplate != "" {
		cfg["context_template"] = params.ContextTemplate
	}
	return cfg
}

func expandAnnotationPromptParams(params *ogxiov1beta1.AnnotationPromptParams) map[string]interface{} {
	cfg := map[string]interface{}{}
	if params.EnableAnnotations != nil {
		cfg["enable_annotations"] = *params.EnableAnnotations
	}
	if params.AnnotationInstructionTemplate != "" {
		cfg["annotation_instruction_template"] = params.AnnotationInstructionTemplate
	}
	if params.ChunkAnnotationTemplate != "" {
		cfg["chunk_annotation_template"] = params.ChunkAnnotationTemplate
	}
	return cfg
}

func expandFileIngestionParams(params *ogxiov1beta1.FileIngestionParams) map[string]interface{} {
	cfg := map[string]interface{}{}
	if params.DefaultChunkSizeTokens != nil {
		cfg["default_chunk_size_tokens"] = *params.DefaultChunkSizeTokens
	}
	if params.DefaultChunkOverlapTokens != nil {
		cfg["default_chunk_overlap_tokens"] = *params.DefaultChunkOverlapTokens
	}
	return cfg
}

func expandChunkRetrievalParams(params *ogxiov1beta1.ChunkRetrievalParams) map[string]interface{} {
	cfg := map[string]interface{}{}
	if params.ChunkMultiplier != nil {
		cfg["chunk_multiplier"] = *params.ChunkMultiplier
	}
	if params.MaxTokensInContext != nil {
		cfg["max_tokens_in_context"] = *params.MaxTokensInContext
	}
	if params.DefaultRerankerStrategy != nil {
		cfg["default_reranker_strategy"] = *params.DefaultRerankerStrategy
	}
	if params.RRFImpactFactor != nil {
		cfg["rrf_impact_factor"] = *params.RRFImpactFactor
	}
	if params.WeightedSearchAlpha != nil {
		cfg["weighted_search_alpha"] = *params.WeightedSearchAlpha
	}
	if params.DefaultSearchMode != nil {
		cfg["default_search_mode"] = *params.DefaultSearchMode
	}
	return cfg
}

func expandFileBatchParams(params *ogxiov1beta1.FileBatchParams) map[string]interface{} {
	cfg := map[string]interface{}{}
	if params.MaxConcurrentFilesPerBatch != nil {
		cfg["max_concurrent_files_per_batch"] = *params.MaxConcurrentFilesPerBatch
	}
	if params.FileBatchChunkSize != nil {
		cfg["file_batch_chunk_size"] = *params.FileBatchChunkSize
	}
	if params.CleanupIntervalSeconds != nil {
		cfg["cleanup_interval_seconds"] = *params.CleanupIntervalSeconds
	}
	return cfg
}

func expandContextualRetrievalParams(params *ogxiov1beta1.ContextualRetrievalParams) map[string]interface{} {
	cfg := map[string]interface{}{}
	if params.Model != nil {
		cfg["model"] = expandQualifiedModel(params.Model)
	}
	if params.DefaultTimeoutSeconds != nil {
		cfg["default_timeout_seconds"] = *params.DefaultTimeoutSeconds
	}
	if params.DefaultMaxConcurrency != nil {
		cfg["default_max_concurrency"] = *params.DefaultMaxConcurrency
	}
	if params.MaxDocumentTokens != nil {
		cfg["max_document_tokens"] = *params.MaxDocumentTokens
	}
	return cfg
}

func expandQualifiedModel(qm *ogxiov1beta1.QualifiedModel) map[string]interface{} {
	m := map[string]interface{}{
		"provider_id": qm.ProviderID,
		"model_id":    qm.ModelID,
	}
	if qm.EmbeddingDimensions != nil {
		m["embedding_dimensions"] = *qm.EmbeddingDimensions
	}
	return m
}

func expandCompactionConfig(cc *ogxiov1beta1.CompactionConfig) map[string]interface{} {
	m := map[string]interface{}{}
	if cc.SummarizationPrompt != "" {
		m["summarization_prompt"] = cc.SummarizationPrompt
	}
	if cc.SummaryPrefix != "" {
		m["summary_prefix"] = cc.SummaryPrefix
	}
	if cc.SummarizationModel != "" {
		m["summarization_model"] = cc.SummarizationModel
	}
	if cc.DefaultCompactThreshold != nil {
		m["default_compact_threshold"] = *cc.DefaultCompactThreshold
	}
	if cc.TokenizerEncoding != "" {
		m["tokenizer_encoding"] = cc.TokenizerEncoding
	}
	return m
}

func applyTLSConfig(cfg map[string]interface{}, tls *ogxiov1beta1.TLSConfig) {
	if tls == nil {
		return
	}
	m := map[string]interface{}{}
	if tls.Verify != nil {
		m["verify"] = *tls.Verify
	}
	if tls.MinVersion != "" {
		m["min_version"] = tls.MinVersion
	}
	if len(tls.Ciphers) > 0 {
		m["ciphers"] = tls.Ciphers
	}
	if len(m) > 0 {
		cfg["tls"] = m
	}
}

func applyTimeoutConfig(cfg map[string]interface{}, timeout *ogxiov1beta1.TimeoutConfig) {
	if timeout == nil {
		return
	}
	m := map[string]interface{}{}
	if timeout.Connect != nil {
		m["connect"] = *timeout.Connect
	}
	if timeout.Read != nil {
		m["read"] = *timeout.Read
	}
	if len(m) > 0 {
		cfg["timeout"] = m
	}
}
