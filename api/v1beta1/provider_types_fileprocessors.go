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

// FileProcessorChunkConfig contains chunking fields shared by all file_processors providers.
type FileProcessorChunkConfig struct {
	// DefaultChunkSizeTokens is the default chunk size in tokens when
	// chunking_strategy type is 'auto'.
	// +optional
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=4096
	DefaultChunkSizeTokens *int `json:"defaultChunkSizeTokens,omitempty"`
	// DefaultChunkOverlapTokens is the default chunk overlap in tokens when
	// chunking_strategy type is 'auto'.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=2048
	DefaultChunkOverlapTokens *int `json:"defaultChunkOverlapTokens,omitempty"`
}

// InlineAutoFileProcessorProvider configures inline::auto for file_processors.
type InlineAutoFileProcessorProvider struct {
	FileProcessorChunkConfig `json:",inline"`
	// ExtractMetadata controls whether to extract PDF metadata
	// (title, author, etc.).
	// +optional
	ExtractMetadata *bool `json:"extractMetadata,omitempty"`
	// CleanText controls whether to clean extracted text
	// (remove extra whitespace, normalize line breaks).
	// +optional
	CleanText *bool `json:"cleanText,omitempty"`
}

func (p InlineAutoFileProcessorProvider) DeriveID() string { return "inline-auto" }

// InlinePyPDFFileProcessorProvider configures inline::pypdf for file_processors.
type InlinePyPDFFileProcessorProvider struct {
	FileProcessorChunkConfig `json:",inline"`
	// ExtractMetadata controls whether to extract PDF metadata
	// (title, author, etc.).
	// +optional
	ExtractMetadata *bool `json:"extractMetadata,omitempty"`
	// CleanText controls whether to clean extracted text
	// (remove extra whitespace, normalize line breaks).
	// +optional
	CleanText *bool `json:"cleanText,omitempty"`
}

func (p InlinePyPDFFileProcessorProvider) DeriveID() string { return "inline-pypdf" }

// InlineMarkItDownFileProcessorProvider configures inline::markitdown for file_processors.
type InlineMarkItDownFileProcessorProvider struct {
	FileProcessorChunkConfig `json:",inline"`
}

func (p InlineMarkItDownFileProcessorProvider) DeriveID() string { return "inline-markitdown" }

// InlineDoclingFileProcessorProvider configures inline::docling for file_processors.
type InlineDoclingFileProcessorProvider struct {
	FileProcessorChunkConfig `json:",inline"`
	// DoOCR controls whether to enable OCR for scanned documents.
	// +optional
	DoOCR *bool `json:"doOcr,omitempty"`
}

func (p InlineDoclingFileProcessorProvider) DeriveID() string { return "inline-docling" }

// DoclingServeProvider configures a remote::docling-serve file_processors provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.baseUrl) || self.baseUrl.size() > 0",message="baseUrl must not be empty if specified"
type DoclingServeProvider struct {
	// BaseURL is the base URL of the Docling Serve instance.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	BaseURL string `json:"baseUrl"`
	// APIKey is the API key for authenticating with Docling Serve.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +optional
	APIKey *SecretKeyRef `json:"apiKey,omitempty"`
	// DefaultChunkSizeTokens is the default chunk size in tokens when
	// chunking_strategy type is 'auto'.
	// +optional
	// +kubebuilder:validation:Minimum=100
	// +kubebuilder:validation:Maximum=4096
	DefaultChunkSizeTokens *int `json:"defaultChunkSizeTokens,omitempty"`
}

func (p DoclingServeProvider) DeriveID() string { return "remote-docling-serve" }

// FileProcessorsRemoteProviders groups remote file_processors providers.
type FileProcessorsRemoteProviders struct {
	// +optional
	DoclingServe *DoclingServeProvider `json:"doclingServe,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *FileProcessorsRemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	var ids []string
	if r.DoclingServe != nil {
		ids = append(ids, r.DoclingServe.DeriveID())
	}
	return append(ids, deriveSliceIDs(r.Custom)...)
}

// FileProcessorsInlineProviders groups inline file_processors providers.
type FileProcessorsInlineProviders struct {
	// +optional
	Auto *InlineAutoFileProcessorProvider `json:"auto,omitempty"`
	// +optional
	PyPDF *InlinePyPDFFileProcessorProvider `json:"pypdf,omitempty"`
	// +optional
	MarkItDown *InlineMarkItDownFileProcessorProvider `json:"markitdown,omitempty"`
	// +optional
	Docling *InlineDoclingFileProcessorProvider `json:"docling,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *FileProcessorsInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	var ids []string
	if inl.Auto != nil {
		ids = append(ids, inl.Auto.DeriveID())
	}
	if inl.PyPDF != nil {
		ids = append(ids, inl.PyPDF.DeriveID())
	}
	if inl.MarkItDown != nil {
		ids = append(ids, inl.MarkItDown.DeriveID())
	}
	if inl.Docling != nil {
		ids = append(ids, inl.Docling.DeriveID())
	}
	return append(ids, deriveSliceIDs(inl.Custom)...)
}

// FileProcessorsProvidersSpec configures file_processors providers.
type FileProcessorsProvidersSpec struct {
	// +optional
	Remote *FileProcessorsRemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *FileProcessorsInlineProviders `json:"inline,omitempty"`
}

func (s *FileProcessorsProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
