// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package jwtassertion

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

type TokenClaims struct {
	Sub      uuid.UUID        `json:"sub"`
	Scope    string           `json:"scope"`
	Exp      int64            `json:"exp"`
	Issuer   string           `json:"iss"`
	Audience jwt.ClaimStrings `json:"aud"`
	jwt.RegisteredClaims
}

type tokenClaimsCtxKey struct{}

type Middleware func(http.Handler) http.Handler

var assertionTokenClaimsKey tokenClaimsCtxKey

type jwtTokenCtx struct{}

var jwtToken jwtTokenCtx

type ctxKeyName string

const (
	scopesKey ctxKeyName = "scopes"
)

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JSONWebKey `json:"keys"`
}

// JSONWebKey represents a single key in a JWKS
type JSONWebKey struct {
	Kty string   `json:"kty"`
	Kid string   `json:"kid"`
	Use string   `json:"use"`
	N   string   `json:"n"`
	E   string   `json:"e"`
	Alg string   `json:"alg"`
	X5c []string `json:"x5c,omitempty"`
}

var (
	jwksCache      *JWKS
	jwksCacheMutex sync.RWMutex
	jwksCacheTime  time.Time
	jwksCacheTTL   = 1 * time.Hour
)

func JWTAuthMiddleware(header string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tokenString := r.Header.Get(header)
			if tokenString == "" {
				utils.WriteErrorResponse(w, http.StatusUnauthorized, fmt.Sprintf("missing header: %s", header))
				return
			}
			// replace "Bearer " prefix
			tokenString = strings.Replace(tokenString, "Bearer ", "", 1)

			// Validate the token using JWKS
			claims, err := validateJWTWithJWKS(tokenString)
			if err != nil {
				slog.Error("JWT validation failed", "error", err)
				utils.WriteErrorResponse(w, http.StatusUnauthorized, "invalid jwt")
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, assertionTokenClaimsKey, claims)
			ctx = context.WithValue(ctx, jwtToken, tokenString)
			ctx = context.WithValue(ctx, scopesKey, claims.Scope)
			r = r.WithContext(ctx)
			next.ServeHTTP(w, r)
		})
	}
}

func GetTokenClaims(ctx context.Context) *TokenClaims {
	claims, ok := ctx.Value(assertionTokenClaimsKey).(*TokenClaims)
	if !ok {
		return nil
	}
	return claims
}

func GetJWTFromContext(ctx context.Context) string {
	token, ok := ctx.Value(jwtToken).(string)
	if !ok {
		return ""
	}
	return token
}

func HasAllScopes(ctx context.Context, requiredScopes []string) bool {
	scopes, ok := ctx.Value(scopesKey).(string)
	if !ok {
		return false
	}
	scopeSet := make(map[string]struct{})
	for _, s := range strings.Fields(scopes) {
		scopeSet[s] = struct{}{}
	}
	for _, scope := range requiredScopes {
		if _, exists := scopeSet[scope]; !exists {
			// as soon as one is missing return false
			return false
		}
	}
	// all required scopes found
	return true
}

// validateJWTWithJWKS validates a JWT token using JWKS and validates issuer and audience
func validateJWTWithJWKS(tokenString string) (*TokenClaims, error) {
	cfg := config.GetConfig()
	if cfg == nil {
		return nil, fmt.Errorf("configuration not loaded")
	}

	var claims *TokenClaims

	// If JWKS URL is configured, validate signature with JWKS
	if cfg.KeyManagerConfigurations.JWKSUrl != "" {
		// Perform full JWKS validation with signature verification
		token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
			// Verify signing method is RSA
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}

			// Get the key ID from the token header
			kid, ok := token.Header["kid"].(string)
			if !ok {
				return nil, fmt.Errorf("kid not found in token header")
			}

			// Fetch JWKS and get the public key
			jwks, err := fetchJWKS(cfg.KeyManagerConfigurations.JWKSUrl)
			if err != nil {
				return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
			}

			// Find the key with matching kid
			for _, key := range jwks.Keys {
				if key.Kid == kid {
					return convertJWKToPublicKey(&key)
				}
			}

			return nil, fmt.Errorf("unable to find key with kid: %s", kid)
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse token: %w", err)
		}

		if !token.Valid {
			return nil, fmt.Errorf("token is not valid")
		}

		validatedClaims, ok := token.Claims.(*TokenClaims)
		if !ok {
			return nil, fmt.Errorf("failed to extract claims")
		}
		claims = validatedClaims
	} else {
		// No JWKS URL configured, skip signature validation
		extractedClaims, err := extractClaimsFromJWT(tokenString)
		if err != nil {
			return nil, fmt.Errorf("failed to extract claims: %w", err)
		}
		claims = extractedClaims

	}

	// Validate issuer
	if err := validateIssuer(claims.Issuer, cfg.KeyManagerConfigurations.Issuer); err != nil {
		return nil, err
	}

	// Validate audience
	if err := validateAudience(claims.Audience, cfg.KeyManagerConfigurations.Audience); err != nil {
		return nil, err
	}

	return claims, nil
}

// validateIssuer validates the issuer claim against allowed issuers
func validateIssuer(issuer string, allowedIssuers []string) error {
	if len(allowedIssuers) == 0 {
		return fmt.Errorf("no allowed issuers configured")
	}

	trimmedIssuer := strings.TrimSpace(issuer)
	for _, allowed := range allowedIssuers {
		if strings.TrimSpace(allowed) == trimmedIssuer {
			return nil
		}
	}
	return fmt.Errorf("invalid issuer: expected one of %v, got %s", allowedIssuers, issuer)
}

// validateAudience validates the audience claim against allowed audiences
func validateAudience(audiences jwt.ClaimStrings, allowedAudiences []string) error {
	if len(allowedAudiences) == 0 {
		return fmt.Errorf("no allowed audiences configured")
	}

	allowedMap := make(map[string]struct{})
	for _, allowed := range allowedAudiences {
		allowedMap[strings.TrimSpace(allowed)] = struct{}{}
	}

	// Check if any token audience is in the allowed list
	for _, aud := range audiences {
		if _, ok := allowedMap[strings.TrimSpace(aud)]; ok {
			return nil
		}
	}

	return fmt.Errorf("invalid audience: expected one of %v, got %v", allowedAudiences, audiences)
}

// fetchJWKS fetches the JWKS from the provided URL with caching
func fetchJWKS(jwksURL string) (*JWKS, error) {
	jwksCacheMutex.RLock()
	if jwksCache != nil && time.Since(jwksCacheTime) < jwksCacheTTL {
		defer jwksCacheMutex.RUnlock()
		return jwksCache, nil
	}
	jwksCacheMutex.RUnlock()

	// Fetch new JWKS
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Get(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("JWKS endpoint returned status: %d", resp.StatusCode)
	}

	var jwks JWKS
	if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
		return nil, fmt.Errorf("failed to decode JWKS: %w", err)
	}

	// Update cache
	jwksCacheMutex.Lock()
	jwksCache = &jwks
	jwksCacheTime = time.Now()
	jwksCacheMutex.Unlock()

	return &jwks, nil
}

// convertJWKToPublicKey converts a JWK to an RSA public key
func convertJWKToPublicKey(jwk *JSONWebKey) (*rsa.PublicKey, error) {
	// Decode the modulus (n)
	nBytes, err := base64.RawURLEncoding.DecodeString(jwk.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}

	// Decode the exponent (e)
	eBytes, err := base64.RawURLEncoding.DecodeString(jwk.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	// Convert bytes to big.Int for modulus
	n := new(big.Int).SetBytes(nBytes)

	// Convert bytes to int for exponent
	var e int
	for _, b := range eBytes {
		e = e<<8 + int(b)
	}

	return &rsa.PublicKey{
		N: n,
		E: e,
	}, nil
}

func extractClaimsFromJWT(tokenString string) (*TokenClaims, error) {
	parts := strings.Split(tokenString, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("invalid jwt, failed to parse, found %d parts", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("invalid jwt, failed to decode payload: %w", err)
	}

	var claims TokenClaims
	if err := json.Unmarshal(payload, &claims); err != nil {
		return nil, fmt.Errorf("invalid jwt, failed to unmarshal payload: %w", err)
	}
	return &claims, nil
}
