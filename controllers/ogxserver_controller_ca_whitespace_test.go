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

package controllers

import (
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

// generateTestCertPEM creates a self-signed PEM certificate for testing.
func generateTestCertPEM(t *testing.T) string {
	t.Helper()
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	require.NoError(t, err)
	serial, err := rand.Int(rand.Reader, new(big.Int).Lsh(big.NewInt(1), 128))
	require.NoError(t, err)
	tmpl := x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "test-ca"},
		NotBefore:    time.Now(),
		NotAfter:     time.Now().Add(24 * time.Hour),
		IsCA:         true,
		KeyUsage:     x509.KeyUsageCertSign,
	}
	der, err := x509.CreateCertificate(rand.Reader, &tmpl, &tmpl, &key.PublicKey, key)
	require.NoError(t, err)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}))
}

// TestExtractValidCertificates_WhitespaceOnly verifies that extractValidCertificates()
// correctly handles whitespace-only data, which is a valid state for auto-detected
// ODH CA bundle keys when no custom CAs are configured.
func TestExtractValidCertificates_WhitespaceOnly(t *testing.T) {
	tests := []struct {
		name        string
		data        []byte
		keyName     string
		expectCerts int
	}{
		{
			name:        "empty string should return success with zero certificates",
			data:        []byte(""),
			keyName:     "empty-key",
			expectCerts: 0,
		},
		{
			name:        "single newline should return success with zero certificates",
			data:        []byte("\n"),
			keyName:     "odh-ca-bundle.crt",
			expectCerts: 0,
		},
		{
			name:        "multiple newlines should return success with zero certificates",
			data:        []byte("\n\n\n"),
			keyName:     "test-key",
			expectCerts: 0,
		},
		{
			name:        "spaces only should return success with zero certificates",
			data:        []byte("   "),
			keyName:     "test-key",
			expectCerts: 0,
		},
		{
			name:        "tabs only should return success with zero certificates",
			data:        []byte("\t\t\t"),
			keyName:     "test-key",
			expectCerts: 0,
		},
		{
			name:        "mixed whitespace should return success with zero certificates",
			data:        []byte("  \n\t  \n  "),
			keyName:     "test-key",
			expectCerts: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			certs, size, count, err := extractValidCertificates(tt.data, tt.keyName)
			require.NoError(t, err, "expected no error for test case: %s", tt.name)
			require.Equal(t, tt.expectCerts, count, "certificate count should match expected value")
			require.Nil(t, certs, "certificates should be nil for zero count")
			require.Equal(t, 0, size, "size should be 0 for zero certificates")
		})
	}
}

// TestExtractValidCertificates_InvalidPEM verifies that invalid PEM data returns an error.
func TestExtractValidCertificates_InvalidPEM(t *testing.T) {
	certs, size, count, err := extractValidCertificates([]byte("not a certificate"), "invalid-key")
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to find valid certificates")
	require.Nil(t, certs)
	require.Zero(t, size)
	require.Zero(t, count)
}

// TestExtractValidCertificates_ValidCerts verifies that valid PEM certificates
// are correctly extracted, including when surrounded by whitespace.
func TestExtractValidCertificates_ValidCerts(t *testing.T) {
	certPEM := generateTestCertPEM(t)

	t.Run("single certificate", func(t *testing.T) {
		certs, size, count, err := extractValidCertificates([]byte(certPEM), "valid-cert")
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.NotNil(t, certs)
		require.Positive(t, size)
		require.Len(t, certs, 1)
	})

	t.Run("certificate with surrounding whitespace", func(t *testing.T) {
		data := []byte("\n\n" + certPEM + "\n\n")
		certs, size, count, err := extractValidCertificates(data, "cert-with-whitespace")
		require.NoError(t, err)
		require.Equal(t, 1, count)
		require.NotNil(t, certs)
		require.Positive(t, size)
		require.Len(t, certs, 1)
	})
}

// TestExtractValidCertificates_MultipleValidCertificates tests that multiple valid
// certificates are correctly extracted and concatenated.
func TestExtractValidCertificates_MultipleValidCertificates(t *testing.T) {
	cert1 := generateTestCertPEM(t)
	cert2 := generateTestCertPEM(t)

	data := []byte(cert1 + "\n" + cert2)

	certs, size, count, err := extractValidCertificates(data, "multi-cert-key")

	require.NoError(t, err, "should successfully extract multiple certificates")
	require.Equal(t, 2, count, "should find 2 certificates")
	require.Len(t, certs, 2, "certificates slice should contain 2 items")
	require.Positive(t, size, "total size should be greater than 0")
}
