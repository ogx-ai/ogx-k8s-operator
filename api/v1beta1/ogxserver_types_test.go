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
	"strings"
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestGetAdoptStorageSource(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			want:        "",
		},
		{
			name:        "empty annotations",
			annotations: map[string]string{},
			want:        "",
		},
		{
			name:        "annotation not present",
			annotations: map[string]string{"other": "value"},
			want:        "",
		},
		{
			name:        "annotation present",
			annotations: map[string]string{AdoptStorageAnnotation: "my-old-llsd"},
			want:        "my-old-llsd",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			if got := r.GetAdoptStorageSource(); got != tt.want {
				t.Errorf("GetAdoptStorageSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetAdoptNetworkingSource(t *testing.T) {
	tests := []struct {
		name        string
		annotations map[string]string
		want        string
	}{
		{
			name:        "nil annotations",
			annotations: nil,
			want:        "",
		},
		{
			name:        "annotation present",
			annotations: map[string]string{AdoptNetworkingAnnotation: "legacy-server"},
			want:        "legacy-server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: tt.annotations,
				},
			}
			if got := r.GetAdoptNetworkingSource(); got != tt.want {
				t.Errorf("GetAdoptNetworkingSource() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestGetEffectivePVCName(t *testing.T) {
	tests := []struct {
		name         string
		instanceName string
		annotations  map[string]string
		want         string
	}{
		{
			name:         "no adoption annotation uses instance name",
			instanceName: "my-server",
			annotations:  nil,
			want:         "my-server-pvc",
		},
		{
			name:         "empty annotations uses instance name",
			instanceName: "my-server",
			annotations:  map[string]string{},
			want:         "my-server-pvc",
		},
		{
			name:         "adoption annotation present uses legacy name",
			instanceName: "my-server",
			annotations:  map[string]string{AdoptStorageAnnotation: "old-llsd"},
			want:         "old-llsd-pvc",
		},
		{
			name:         "adoption annotation same as instance name",
			instanceName: "same-name",
			annotations:  map[string]string{AdoptStorageAnnotation: "same-name"},
			want:         "same-name-pvc",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := &OGXServer{
				ObjectMeta: metav1.ObjectMeta{
					Name:        tt.instanceName,
					Annotations: tt.annotations,
				},
			}
			if got := r.GetEffectivePVCName(); got != tt.want {
				t.Errorf("GetEffectivePVCName() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestValidateAdoptionAnnotation(t *testing.T) {
	tests := []struct {
		name    string
		value   string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "valid simple name",
			value:   "my-llsd",
			wantErr: false,
		},
		{
			name:    "valid single character",
			value:   "a",
			wantErr: false,
		},
		{
			name:    "valid numeric",
			value:   "123",
			wantErr: false,
		},
		{
			name:    "valid max length (63 chars)",
			value:   strings.Repeat("a", 63),
			wantErr: false,
		},
		{
			name:    "empty string",
			value:   "",
			wantErr: true,
			errMsg:  "must not be empty",
		},
		{
			name:    "exceeds 63 characters",
			value:   strings.Repeat("a", 64),
			wantErr: true,
			errMsg:  "exceeds 63 characters",
		},
		{
			name:    "uppercase letters",
			value:   "MyLLSD",
			wantErr: true,
			errMsg:  "not a valid RFC 1123",
		},
		{
			name:    "starts with hyphen",
			value:   "-invalid",
			wantErr: true,
			errMsg:  "not a valid RFC 1123",
		},
		{
			name:    "ends with hyphen",
			value:   "invalid-",
			wantErr: true,
			errMsg:  "not a valid RFC 1123",
		},
		{
			name:    "contains underscore",
			value:   "my_llsd",
			wantErr: true,
			errMsg:  "not a valid RFC 1123",
		},
		{
			name:    "contains dot",
			value:   "my.llsd",
			wantErr: true,
			errMsg:  "not a valid RFC 1123",
		},
		{
			name:    "contains space",
			value:   "my llsd",
			wantErr: true,
			errMsg:  "not a valid RFC 1123",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := ValidateAdoptionAnnotation(tt.value)
			if (err != nil) != tt.wantErr {
				t.Errorf("ValidateAdoptionAnnotation(%q) error = %v, wantErr %v", tt.value, err, tt.wantErr)
				return
			}
			if tt.wantErr && tt.errMsg != "" {
				if !strings.Contains(err.Error(), tt.errMsg) {
					t.Errorf("ValidateAdoptionAnnotation(%q) error = %q, want substring %q", tt.value, err.Error(), tt.errMsg)
				}
			}
		})
	}
}
