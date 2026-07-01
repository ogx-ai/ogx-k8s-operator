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

// HNSWConfig configures an HNSW vector index for PGVector.
type HNSWConfig struct {
	// M is the maximum number of connections per element in the HNSW graph.
	// +optional
	// +kubebuilder:validation:Minimum=1
	M *int `json:"m,omitempty"`
	// EfConstruction controls the index build-time accuracy/speed tradeoff.
	// +optional
	// +kubebuilder:validation:Minimum=1
	EfConstruction *int `json:"efConstruction,omitempty"`
	// EfSearch controls the query-time accuracy/speed tradeoff.
	// +optional
	// +kubebuilder:validation:Minimum=1
	EfSearch *int `json:"efSearch,omitempty"`
}

// IVFFlatConfig configures an IVFFlat vector index for PGVector.
type IVFFlatConfig struct {
	// Nlist is the number of inverted lists (clusters).
	// +optional
	// +kubebuilder:validation:Minimum=1
	Nlist *int `json:"nlist,omitempty"`
	// Nprobe is the number of clusters to search at query time.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Nprobe *int `json:"nprobe,omitempty"`
}

// VectorIndexConfig configures the vector index strategy for PGVector.
// Exactly one of hnsw or ivfFlat must be specified.
// +kubebuilder:validation:XValidation:rule="has(self.hnsw) || has(self.ivfFlat)",message="one of hnsw or ivfFlat must be specified"
// +kubebuilder:validation:XValidation:rule="!(has(self.hnsw) && has(self.ivfFlat))",message="only one of hnsw or ivfFlat can be specified"
type VectorIndexConfig struct {
	// HNSW configures an HNSW index.
	// +optional
	HNSW *HNSWConfig `json:"hnsw,omitempty"`
	// IVFFlat configures an IVFFlat index.
	// +optional
	IVFFlat *IVFFlatConfig `json:"ivfFlat,omitempty"`
}

// PgvectorProvider configures a remote::pgvector vector I/O provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.host) || self.host.size() > 0",message="host must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.db) || self.db.size() > 0",message="db must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.user) || self.user.size() > 0",message="user must not be empty if specified"
type PgvectorProvider struct {
	RoutedProviderBase `json:",inline"`
	// Host is the PostgreSQL server hostname.
	// +optional
	Host string `json:"host,omitempty"`
	// Port is the PostgreSQL server port.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port *int `json:"port,omitempty"`
	// DB is the PostgreSQL database name.
	// +optional
	DB string `json:"db,omitempty"`
	// User is the PostgreSQL username.
	// +optional
	User string `json:"user,omitempty"`
	// Password is the PostgreSQL password.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +kubebuilder:validation:Required
	Password SecretKeyRef `json:"password"`
	// DistanceMetric is the distance metric used for vector search.
	// +optional
	// +kubebuilder:validation:Enum=COSINE;L2;L1;INNER_PRODUCT
	DistanceMetric string `json:"distanceMetric,omitempty"`
	// VectorIndex configures the vector index strategy for
	// Approximate Nearest Neighbor (ANN) search.
	// +optional
	VectorIndex *VectorIndexConfig `json:"vectorIndex,omitempty"`
}

func (p PgvectorProvider) DeriveID() string { return p.deriveOrDefault("remote-pgvector") }

// MilvusProvider configures a remote::milvus vector I/O provider instance.
// +kubebuilder:validation:XValidation:rule="!has(self.consistencyLevel) || self.consistencyLevel.size() > 0",message="consistencyLevel must not be empty if specified"
type MilvusProvider struct {
	RoutedProviderBase `json:",inline"`
	// URI is the URI of the Milvus server.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	URI string `json:"uri"`
	// Token is the authentication token for the Milvus server.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +optional
	Token *SecretKeyRef `json:"token,omitempty"`
	// ConsistencyLevel is the consistency level of the Milvus server.
	// +optional
	ConsistencyLevel string `json:"consistencyLevel,omitempty"`
}

func (p MilvusProvider) DeriveID() string { return p.deriveOrDefault("remote-milvus") }

// QdrantProvider configures a remote::qdrant vector I/O provider instance.
// +kubebuilder:validation:XValidation:rule="has(self.url) || has(self.host)",message="at least one of url or host must be specified"
// +kubebuilder:validation:XValidation:rule="!has(self.url) || self.url.size() > 0",message="url must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.host) || self.host.size() > 0",message="host must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.location) || self.location.size() > 0",message="location must not be empty if specified"
// +kubebuilder:validation:XValidation:rule="!has(self.prefix) || self.prefix.size() > 0",message="prefix must not be empty if specified"
type QdrantProvider struct {
	RoutedProviderBase `json:",inline"`
	// URL is the URL of the Qdrant server.
	// +optional
	URL string `json:"url,omitempty"`
	// Host is the hostname of the Qdrant server.
	// +optional
	Host string `json:"host,omitempty"`
	// Port is the REST API port of the Qdrant server.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	Port *int `json:"port,omitempty"`
	// APIKey is the authentication key for the Qdrant server.
	// The Secret must be in the same namespace as the OGXServer
	// and must have the label ogx.io/watch: "true".
	// +optional
	APIKey *SecretKeyRef `json:"apiKey,omitempty"`
	// Location is the Qdrant server location identifier.
	// +optional
	Location string `json:"location,omitempty"`
	// GRPCPort is the gRPC port of the Qdrant server.
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=65535
	GRPCPort *int `json:"grpcPort,omitempty"`
	// PreferGRPC controls whether to prefer gRPC over REST for communication.
	// +optional
	PreferGRPC *bool `json:"preferGrpc,omitempty"`
	// HTTPS controls whether to use HTTPS for the connection.
	// +optional
	HTTPS *bool `json:"https,omitempty"`
	// Prefix is the URL path prefix for the Qdrant server.
	// +optional
	Prefix string `json:"prefix,omitempty"`
	// Timeout is the connection timeout in seconds.
	// +optional
	// +kubebuilder:validation:Minimum=1
	Timeout *int `json:"timeout,omitempty"`
}

func (p QdrantProvider) DeriveID() string { return p.deriveOrDefault("remote-qdrant") }

// VectorIORemoteProviders groups remote vector I/O providers.
type VectorIORemoteProviders struct {
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Pgvector []PgvectorProvider `json:"pgvector,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Milvus []MilvusProvider `json:"milvus,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Qdrant []QdrantProvider `json:"qdrant,omitempty"`
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *VectorIORemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	return slices.Concat(
		deriveSliceIDs(r.Pgvector), deriveSliceIDs(r.Milvus),
		deriveSliceIDs(r.Qdrant), deriveSliceIDs(r.Custom),
	)
}

// VectorIOInlineProviders groups inline vector I/O providers.
type VectorIOInlineProviders struct {
	// +optional
	// +kubebuilder:validation:MinItems=1
	// +kubebuilder:validation:MaxItems=100
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *VectorIOInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	return deriveSliceIDs(inl.Custom)
}

// VectorIOProvidersSpec configures vector I/O providers.
type VectorIOProvidersSpec struct {
	// +optional
	Remote *VectorIORemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *VectorIOInlineProviders `json:"inline,omitempty"`
}

func (s *VectorIOProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
