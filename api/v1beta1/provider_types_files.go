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

// S3Provider configures a remote::s3 files provider instance.
type S3Provider struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	BucketName string `json:"bucketName"`
	// +optional
	// +kubebuilder:default:="us-east-1"
	Region string `json:"region,omitempty"`
	// +optional
	AWSAccessKeyID *SecretKeyRef `json:"awsAccessKeyId,omitempty"`
	// +optional
	AWSSecretAccessKey *SecretKeyRef `json:"awsSecretAccessKey,omitempty"`
	// +optional
	EndpointURL string `json:"endpointUrl,omitempty"`
}

func (p S3Provider) DeriveID() string { return "remote-s3" }

// InlineLocalFSProvider configures inline::localfs.
type InlineLocalFSProvider struct{}

func (p InlineLocalFSProvider) DeriveID() string { return "inline-localfs" }

// FilesRemoteProviders groups remote files providers.
type FilesRemoteProviders struct {
	// +optional
	S3 *S3Provider `json:"s3,omitempty"`
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (r *FilesRemoteProviders) IDs() []string {
	if r == nil {
		return nil
	}
	var ids []string
	if r.S3 != nil {
		ids = append(ids, r.S3.DeriveID())
	}
	return append(ids, deriveSliceIDs(r.Custom)...)
}

// FilesInlineProviders groups inline files providers.
type FilesInlineProviders struct {
	// +optional
	LocalFS *InlineLocalFSProvider `json:"localfs,omitempty"`
	// +optional
	Custom []CustomProvider `json:"custom,omitempty"`
}

func (inl *FilesInlineProviders) IDs() []string {
	if inl == nil {
		return nil
	}
	var ids []string
	if inl.LocalFS != nil {
		ids = append(ids, inl.LocalFS.DeriveID())
	}
	return append(ids, deriveSliceIDs(inl.Custom)...)
}

// FilesProvidersSpec configures files providers.
type FilesProvidersSpec struct {
	// +optional
	Remote *FilesRemoteProviders `json:"remote,omitempty"`
	// +optional
	Inline *FilesInlineProviders `json:"inline,omitempty"`
}

func (s *FilesProvidersSpec) IDs() []string {
	if s == nil {
		return nil
	}
	return slices.Concat(s.Remote.IDs(), s.Inline.IDs())
}
