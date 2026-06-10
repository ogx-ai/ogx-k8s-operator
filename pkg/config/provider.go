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
	if p.VectorIndex != nil {
		vi := map[string]interface{}{}
		if p.VectorIndex.HNSW != nil {
			hnsw := map[string]interface{}{}
			if p.VectorIndex.HNSW.M != nil {
				hnsw["m"] = *p.VectorIndex.HNSW.M
			}
			if p.VectorIndex.HNSW.EfConstruction != nil {
				hnsw["ef_construction"] = *p.VectorIndex.HNSW.EfConstruction
			}
			if p.VectorIndex.HNSW.EfSearch != nil {
				hnsw["ef_search"] = *p.VectorIndex.HNSW.EfSearch
			}
			vi["hnsw"] = hnsw
		}
		if p.VectorIndex.IVFFlat != nil {
			ivf := map[string]interface{}{}
			if p.VectorIndex.IVFFlat.Nlist != nil {
				ivf["nlist"] = *p.VectorIndex.IVFFlat.Nlist
			}
			if p.VectorIndex.IVFFlat.Nprobe != nil {
				ivf["nprobe"] = *p.VectorIndex.IVFFlat.Nprobe
			}
			vi["ivf_flat"] = ivf
		}
		cfg["vector_index"] = vi
	}

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::pgvector",
		Config:       cfg,
	}
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
	if p.URL != "" {
		cfg["url"] = p.URL
	}
	if p.Host != "" {
		cfg["host"] = p.Host
	}
	if p.Port != nil {
		cfg["port"] = *p.Port
	}
	if p.APIKey != nil {
		cfg["api_key"] = envVarRef(p.DeriveID(), "API_KEY")
	}
	if p.Location != "" {
		cfg["location"] = p.Location
	}
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

	return ConfigProvider{
		ProviderID:   p.DeriveID(),
		ProviderType: "remote::qdrant",
		Config:       cfg,
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
	if vs.DefaultProviderID != "" {
		m["default_provider_id"] = vs.DefaultProviderID
	}
	if vs.DefaultEmbeddingModel != nil {
		m["default_embedding_model"] = expandQualifiedModel(vs.DefaultEmbeddingModel)
	}
	if vs.DefaultRerankerModel != nil {
		rm := map[string]interface{}{
			"provider_id": vs.DefaultRerankerModel.ProviderID,
			"model_id":    vs.DefaultRerankerModel.ModelID,
		}
		m["default_reranker_model"] = rm
	}
	if vs.RewriteQueryParams != nil {
		rqp := map[string]interface{}{}
		if vs.RewriteQueryParams.Model != nil {
			rqp["model"] = expandQualifiedModel(vs.RewriteQueryParams.Model)
		}
		if vs.RewriteQueryParams.Prompt != "" {
			rqp["prompt"] = vs.RewriteQueryParams.Prompt
		}
		if vs.RewriteQueryParams.MaxTokens != nil {
			rqp["max_tokens"] = *vs.RewriteQueryParams.MaxTokens
		}
		if vs.RewriteQueryParams.Temperature != nil {
			rqp["temperature"] = *vs.RewriteQueryParams.Temperature
		}
		m["rewrite_query_params"] = rqp
	}
	if vs.FileSearchParams != nil {
		fsp := map[string]interface{}{}
		if vs.FileSearchParams.HeaderTemplate != "" {
			fsp["header_template"] = vs.FileSearchParams.HeaderTemplate
		}
		if vs.FileSearchParams.FooterTemplate != "" {
			fsp["footer_template"] = vs.FileSearchParams.FooterTemplate
		}
		m["file_search_params"] = fsp
	}
	if vs.ContextPromptParams != nil {
		cpp := map[string]interface{}{}
		if vs.ContextPromptParams.ChunkAnnotationTemplate != "" {
			cpp["chunk_annotation_template"] = vs.ContextPromptParams.ChunkAnnotationTemplate
		}
		if vs.ContextPromptParams.ContextTemplate != "" {
			cpp["context_template"] = vs.ContextPromptParams.ContextTemplate
		}
		m["context_prompt_params"] = cpp
	}
	if vs.AnnotationPromptParams != nil {
		app := map[string]interface{}{}
		if vs.AnnotationPromptParams.EnableAnnotations != nil {
			app["enable_annotations"] = *vs.AnnotationPromptParams.EnableAnnotations
		}
		if vs.AnnotationPromptParams.AnnotationInstructionTemplate != "" {
			app["annotation_instruction_template"] = vs.AnnotationPromptParams.AnnotationInstructionTemplate
		}
		if vs.AnnotationPromptParams.ChunkAnnotationTemplate != "" {
			app["chunk_annotation_template"] = vs.AnnotationPromptParams.ChunkAnnotationTemplate
		}
		m["annotation_prompt_params"] = app
	}
	if vs.FileIngestionParams != nil {
		fip := map[string]interface{}{}
		if vs.FileIngestionParams.DefaultChunkSizeTokens != nil {
			fip["default_chunk_size_tokens"] = *vs.FileIngestionParams.DefaultChunkSizeTokens
		}
		if vs.FileIngestionParams.DefaultChunkOverlapTokens != nil {
			fip["default_chunk_overlap_tokens"] = *vs.FileIngestionParams.DefaultChunkOverlapTokens
		}
		m["file_ingestion_params"] = fip
	}
	if vs.ChunkRetrievalParams != nil {
		crp := map[string]interface{}{}
		if vs.ChunkRetrievalParams.ChunkMultiplier != nil {
			crp["chunk_multiplier"] = *vs.ChunkRetrievalParams.ChunkMultiplier
		}
		if vs.ChunkRetrievalParams.MaxTokensInContext != nil {
			crp["max_tokens_in_context"] = *vs.ChunkRetrievalParams.MaxTokensInContext
		}
		if vs.ChunkRetrievalParams.DefaultRerankerStrategy != nil {
			crp["default_reranker_strategy"] = *vs.ChunkRetrievalParams.DefaultRerankerStrategy
		}
		if vs.ChunkRetrievalParams.RRFImpactFactor != nil {
			crp["rrf_impact_factor"] = *vs.ChunkRetrievalParams.RRFImpactFactor
		}
		if vs.ChunkRetrievalParams.WeightedSearchAlpha != nil {
			crp["weighted_search_alpha"] = *vs.ChunkRetrievalParams.WeightedSearchAlpha
		}
		if vs.ChunkRetrievalParams.DefaultSearchMode != nil {
			crp["default_search_mode"] = *vs.ChunkRetrievalParams.DefaultSearchMode
		}
		m["chunk_retrieval_params"] = crp
	}
	if vs.FileBatchParams != nil {
		fbp := map[string]interface{}{}
		if vs.FileBatchParams.MaxConcurrentFilesPerBatch != nil {
			fbp["max_concurrent_files_per_batch"] = *vs.FileBatchParams.MaxConcurrentFilesPerBatch
		}
		if vs.FileBatchParams.FileBatchChunkSize != nil {
			fbp["file_batch_chunk_size"] = *vs.FileBatchParams.FileBatchChunkSize
		}
		if vs.FileBatchParams.CleanupIntervalSeconds != nil {
			fbp["cleanup_interval_seconds"] = *vs.FileBatchParams.CleanupIntervalSeconds
		}
		m["file_batch_params"] = fbp
	}
	if vs.ContextualRetrievalParams != nil {
		ctx := map[string]interface{}{}
		if vs.ContextualRetrievalParams.Model != nil {
			ctx["model"] = expandQualifiedModel(vs.ContextualRetrievalParams.Model)
		}
		if vs.ContextualRetrievalParams.DefaultTimeoutSeconds != nil {
			ctx["default_timeout_seconds"] = *vs.ContextualRetrievalParams.DefaultTimeoutSeconds
		}
		if vs.ContextualRetrievalParams.DefaultMaxConcurrency != nil {
			ctx["default_max_concurrency"] = *vs.ContextualRetrievalParams.DefaultMaxConcurrency
		}
		if vs.ContextualRetrievalParams.MaxDocumentTokens != nil {
			ctx["max_document_tokens"] = *vs.ContextualRetrievalParams.MaxDocumentTokens
		}
		m["contextual_retrieval_params"] = ctx
	}
	return m
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
