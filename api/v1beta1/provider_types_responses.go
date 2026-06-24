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

import "slices"

// QualifiedModel identifies a model with its provider.
type QualifiedModel struct {
	// ProviderID is the provider to use for this model.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProviderID string `json:"providerId"`
	// ModelID is the model identifier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ModelID string `json:"modelId"`
	// EmbeddingDimensions is the dimensionality of the embedding vectors.
	// +optional
	// +kubebuilder:validation:Minimum=1
	EmbeddingDimensions *int `json:"embeddingDimensions,omitempty"`
}

// RerankerModel identifies a reranker model with its provider.
type RerankerModel struct {
	// ProviderID is the provider to use for this model.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ProviderID string `json:"providerId"`
	// ModelID is the model identifier.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ModelID string `json:"modelId"`
}

// RewriteQueryParams configures query rewriting/expansion for vector search.
type RewriteQueryParams struct {
	// Model is the LLM model used for query rewriting.
	// +optional
	Model *QualifiedModel `json:"model,omitempty"`
	// Prompt is the prompt template for query rewriting.
	// Use {query} as a placeholder for the original query.
	// +optional
	Prompt string `json:"prompt,omitempty"`
	// MaxTokens is the maximum number of tokens for query expansion responses.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxTokens *int `json:"maxTokens,omitempty"`
	// Temperature controls randomness in query rewriting (0.0 = deterministic, 1.0 = creative).
	// Specified as a decimal string (e.g., "0.7").
	// +optional
	Temperature *string `json:"temperature,omitempty"`
}

// FileSearchDisplayParams configures file search output formatting.
type FileSearchDisplayParams struct {
	// HeaderTemplate is the template for the header text before search results.
	// +optional
	HeaderTemplate string `json:"headerTemplate,omitempty"`
	// FooterTemplate is the template for the footer text after search results.
	// +optional
	FooterTemplate string `json:"footerTemplate,omitempty"`
}

// ContextPromptParams configures LLM prompt content and chunk formatting.
type ContextPromptParams struct {
	// ChunkAnnotationTemplate is the template for formatting individual chunks.
	// +optional
	ChunkAnnotationTemplate string `json:"chunkAnnotationTemplate,omitempty"`
	// ContextTemplate is the template for explaining search results to the model.
	// +optional
	ContextTemplate string `json:"contextTemplate,omitempty"`
}

// AnnotationPromptParams configures source annotation and attribution.
type AnnotationPromptParams struct {
	// EnableAnnotations controls whether source annotations are included.
	// +optional
	EnableAnnotations *bool `json:"enableAnnotations,omitempty"`
	// AnnotationInstructionTemplate provides instructions for citing sources.
	// +optional
	AnnotationInstructionTemplate string `json:"annotationInstructionTemplate,omitempty"`
	// ChunkAnnotationTemplate is the template for chunks with annotation info.
	// +optional
	ChunkAnnotationTemplate string `json:"chunkAnnotationTemplate,omitempty"`
}

// FileIngestionParams configures file processing during ingestion.
type FileIngestionParams struct {
	// DefaultChunkSizeTokens is the default chunk size in tokens.
	// +optional
	// +kubebuilder:validation:Minimum=1
	DefaultChunkSizeTokens *int `json:"defaultChunkSizeTokens,omitempty"`
	// DefaultChunkOverlapTokens is the default overlap between chunks in tokens.
	// +optional
	// +kubebuilder:validation:Minimum=0
	DefaultChunkOverlapTokens *int `json:"defaultChunkOverlapTokens,omitempty"`
}

// ChunkRetrievalParams configures chunk retrieval and ranking during search.
type ChunkRetrievalParams struct {
	// ChunkMultiplier multiplies the number of chunks retrieved for over-retrieval.
	// +optional
	// +kubebuilder:validation:Minimum=1
	ChunkMultiplier *int `json:"chunkMultiplier,omitempty"`
	// MaxTokensInContext limits total tokens allowed in RAG context.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxTokensInContext *int `json:"maxTokensInContext,omitempty"`
	// DefaultRerankerStrategy is the default reranking strategy.
	// +optional
	// +kubebuilder:validation:Enum=rrf;weighted;normalized
	DefaultRerankerStrategy *string `json:"defaultRerankerStrategy,omitempty"`
	// RRFImpactFactor is the impact factor for Reciprocal Rank Fusion reranking.
	// Specified as a decimal string (e.g., "60.0").
	// +optional
	RRFImpactFactor *string `json:"rrfImpactFactor,omitempty"`
	// WeightedSearchAlpha is the alpha weight for weighted search reranking (0.0-1.0).
	// Specified as a decimal string (e.g., "0.5").
	// +optional
	WeightedSearchAlpha *string `json:"weightedSearchAlpha,omitempty"`
	// DefaultSearchMode is the default search mode.
	// +optional
	// +kubebuilder:validation:Enum=vector;keyword;hybrid
	DefaultSearchMode *string `json:"defaultSearchMode,omitempty"`
}

// FileBatchParams configures file batch processing.
type FileBatchParams struct {
	// MaxConcurrentFilesPerBatch limits concurrent files processed per batch.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxConcurrentFilesPerBatch *int `json:"maxConcurrentFilesPerBatch,omitempty"`
	// FileBatchChunkSize is the number of files to process in each batch chunk.
	// +optional
	// +kubebuilder:validation:Minimum=1
	FileBatchChunkSize *int `json:"fileBatchChunkSize,omitempty"`
	// CleanupIntervalSeconds is the interval between expired batch cleanup runs.
	// +optional
	// +kubebuilder:validation:Minimum=1
	CleanupIntervalSeconds *int `json:"cleanupIntervalSeconds,omitempty"`
}

// ContextualRetrievalParams configures contextual retrieval during file ingestion.
type ContextualRetrievalParams struct {
	// Model is the default LLM model for contextual retrieval.
	// +optional
	Model *QualifiedModel `json:"model,omitempty"`
	// DefaultTimeoutSeconds is the timeout per LLM contextualization call.
	// +optional
	// +kubebuilder:validation:Minimum=1
	DefaultTimeoutSeconds *int `json:"defaultTimeoutSeconds,omitempty"`
	// DefaultMaxConcurrency limits concurrent LLM calls for contextualization.
	// +optional
	// +kubebuilder:validation:Minimum=1
	DefaultMaxConcurrency *int `json:"defaultMaxConcurrency,omitempty"`
	// MaxDocumentTokens limits document size in tokens for contextual retrieval.
	// +optional
	// +kubebuilder:validation:Minimum=1
	MaxDocumentTokens *int `json:"maxDocumentTokens,omitempty"`
}

// VectorStoresConfig configures vector store behavior for responses and file search.
// +kubebuilder:validation:XValidation:rule="!has(self.defaultProviderId) || self.defaultProviderId.size() > 0",message="defaultProviderId must not be empty if specified"
type VectorStoresConfig struct {
	// DefaultProviderID is the vector_io provider to use when multiple
	// providers are available and none is specified.
	// +optional
	DefaultProviderID string `json:"defaultProviderId,omitempty"`
	// DefaultEmbeddingModel configures the default embedding model.
	// +optional
	DefaultEmbeddingModel *QualifiedModel `json:"defaultEmbeddingModel,omitempty"`
	// DefaultRerankerModel configures the default reranker model.
	// +optional
	DefaultRerankerModel *RerankerModel `json:"defaultRerankerModel,omitempty"`
	// RewriteQueryParams configures query rewriting/expansion. Nil disables rewriting.
	// +optional
	RewriteQueryParams *RewriteQueryParams `json:"rewriteQueryParams,omitempty"`
	// FileSearchParams configures file search output formatting.
	// +optional
	FileSearchParams *FileSearchDisplayParams `json:"fileSearchParams,omitempty"`
	// ContextPromptParams configures context prompt templates.
	// +optional
	ContextPromptParams *ContextPromptParams `json:"contextPromptParams,omitempty"`
	// AnnotationPromptParams configures source annotation settings.
	// +optional
	AnnotationPromptParams *AnnotationPromptParams `json:"annotationPromptParams,omitempty"`
	// FileIngestionParams configures file ingestion chunk settings.
	// +optional
	FileIngestionParams *FileIngestionParams `json:"fileIngestionParams,omitempty"`
	// ChunkRetrievalParams configures chunk retrieval and ranking.
	// +optional
	ChunkRetrievalParams *ChunkRetrievalParams `json:"chunkRetrievalParams,omitempty"`
	// FileBatchParams configures file batch processing.
	// +optional
	FileBatchParams *FileBatchParams `json:"fileBatchParams,omitempty"`
	// ContextualRetrievalParams configures contextual retrieval during ingestion.
	// +optional
	ContextualRetrievalParams *ContextualRetrievalParams `json:"contextualRetrievalParams,omitempty"`
}

// CompactionConfig configures conversation compaction behavior for responses.
// +kubebuilder:validation:XValidation:rule="!has(self.summarizationPrompt) || self.summarizationPrompt.size() > 0",message="summarizationPrompt must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.summaryPrefix) || self.summaryPrefix.size() > 0",message="summaryPrefix must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.summarizationModel) || self.summarizationModel.size() > 0",message="summarizationModel must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.tokenizerEncoding) || self.tokenizerEncoding.size() > 0",message="tokenizerEncoding must not be empty if specified"
type CompactionConfig struct {
	// SummarizationPrompt is the prompt used to instruct the model to
	// summarize conversation history during compaction.
	// +optional
	SummarizationPrompt string `json:"summarizationPrompt,omitempty"`
	// SummaryPrefix is text prepended to the compaction summary to frame
	// it as a handoff for the next LLM context window.
	// +optional
	SummaryPrefix string `json:"summaryPrefix,omitempty"`
	// SummarizationModel is the model to use for generating compaction
	// summaries. If unset, uses the same model as the conversation.
	// +optional
	SummarizationModel string `json:"summarizationModel,omitempty"`
	// DefaultCompactThreshold is the token count threshold for auto-compaction.
	// Conversations exceeding this count will be automatically compacted.
	// +optional
	// +kubebuilder:validation:Minimum=1
	DefaultCompactThreshold *int `json:"defaultCompactThreshold,omitempty"`
	// TokenizerEncoding is the tiktoken encoding name for token counting
	// (e.g., "o200k_base", "cl100k_base").
	// +optional
	TokenizerEncoding string `json:"tokenizerEncoding,omitempty"`
}

// InlineBuiltinResponsesProvider configures inline::builtin for responses.
type InlineBuiltinResponsesProvider struct {
	// VectorStoresConfig configures vector store behavior for file search
	// and retrieval-augmented generation.
	// +optional
	VectorStoresConfig *VectorStoresConfig `json:"vectorStoresConfig,omitempty"`
	// CompactionConfig configures conversation compaction behavior
	// and prompt templates.
	// +optional
	CompactionConfig *CompactionConfig `json:"compactionConfig,omitempty"`
}

func (p InlineBuiltinResponsesProvider) DeriveID() string { return "inline-builtin" }

// ResponsesRemoteProviders groups remote responses providers.
type ResponsesRemoteProviders struct {
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *ResponsesRemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	return deriveSliceIDs(r.Custom)
}

// ResponsesInlineProviders groups inline responses providers.
type ResponsesInlineProviders struct {
	// +optional
	Builtin *InlineBuiltinResponsesProvider `json:"builtin,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *ResponsesInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	var ids []string
	if inl.Builtin != nil {
		ids = append(ids, inl.Builtin.DeriveID())
	}
	return append(ids, deriveSliceIDs(inl.Custom)...)
}

// ResponsesProvidersSpec configures responses providers.
type ResponsesProvidersSpec struct {
	// +optional
	Remote *ResponsesRemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *ResponsesInlineProviders `json:"inline,omitempty"`
}

func (s *ResponsesProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
