// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

package models

// TokenResponse represents the response for token generation
type TokenResponse struct {
	// Token is the signed JWT token
	Token string `json:"token"`
	// ExpiresAt is the Unix timestamp when the token expires
	ExpiresAt int64 `json:"expires_at"`
	// IssuedAt is the Unix timestamp when the token was issued
	IssuedAt int64 `json:"issued_at"`
	// TokenType is the type of token (always "Bearer")
	TokenType string `json:"token_type"`
}

// TokenRequest represents the optional request body for token generation
type TokenRequest struct {
	// ExpiresIn is the optional expiry duration in Go duration format (e.g., "720h")
	ExpiresIn string `json:"expires_in,omitempty"`
}

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JWK `json:"keys"`
}

// JWK represents a JSON Web Key
type JWK struct {
	// Kty is the key type (RSA)
	Kty string `json:"kty"`
	// Alg is the algorithm (RS256)
	Alg string `json:"alg"`
	// Use is the intended use (sig for signature)
	Use string `json:"use"`
	// Kid is the key ID
	Kid string `json:"kid"`
	// N is the modulus value for the RSA public key (Base64urlUInt-encoded)
	N string `json:"n"`
	// E is the exponent value for the RSA public key (Base64urlUInt-encoded)
	E string `json:"e"`
}
