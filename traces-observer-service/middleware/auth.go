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

package middleware

import (
	"context"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/wso2/ai-agent-management-platform/traces-observer-service/config"
)

// JWKS represents a JSON Web Key Set
type JWKS struct {
	Keys []JSONWebKey `json:"keys"`
}

// JSONWebKey represents a single key in a JWKS
type JSONWebKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	N   string `json:"n"`
	E   string `json:"e"`
}

type ErrorBody struct {
	Error   string `json:"error"`
	Message string `json:"message"`
}

var (
	jwksCache      *JWKS
	jwksCacheMutex sync.RWMutex
	jwksCacheTime  time.Time
	jwksCacheTTL   = 1 * time.Hour
	jwksHTTPClient = &http.Client{Timeout: 10 * time.Second}
)

func writeAuthError(w http.ResponseWriter, status int, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	body := ErrorBody{Error: http.StatusText(status), Message: message}
	_ = json.NewEncoder(w).Encode(body)
}

// JWTAuth returns a middleware that validates Bearer JWTs on every request.
// Routes wrapped by this middleware require a valid token in the Authorization header.
func JWTAuth(cfg config.AuthConfig) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHeader := r.Header.Get("Authorization")
			if authHeader == "" {
				writeAuthError(w, http.StatusUnauthorized, "missing Authorization header")
				return
			}

			tokenString := strings.TrimPrefix(authHeader, "Bearer ")
			if tokenString == authHeader {
				writeAuthError(w, http.StatusUnauthorized, "Authorization header must use Bearer scheme")
				return
			}

			if err := validateJWT(r.Context(), tokenString, cfg); err != nil {
				slog.Error("JWT validation failed", "error", err)
				writeAuthError(w, http.StatusUnauthorized, "invalid or expired token")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func validateJWT(ctx context.Context, tokenString string, cfg config.AuthConfig) error {
	if cfg.JWKSUrl != "" {
		return validateWithJWKS(ctx, tokenString, cfg)
	}
	if cfg.IsLocalDevEnv {
		return validateLocalDev(tokenString)
	}
	return fmt.Errorf("KEY_MANAGER_JWKS_URL must be configured for JWT validation")
}

func validateWithJWKS(ctx context.Context, tokenString string, cfg config.AuthConfig) error {
	type claims struct {
		jwt.RegisteredClaims
	}

	token, err := jwt.ParseWithClaims(tokenString, &claims{}, func(token *jwt.Token) (interface{}, error) {
		if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
			return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
		}
		kid, ok := token.Header["kid"].(string)
		if !ok {
			return nil, fmt.Errorf("kid not found in token header")
		}
		jwks, err := fetchJWKS(ctx, cfg.JWKSUrl, false)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
		}
		if pub, ok := findJWK(jwks, kid); ok {
			return jwkToRSAPublicKey(pub)
		}
		jwks, err = fetchJWKS(ctx, cfg.JWKSUrl, true)
		if err != nil {
			return nil, fmt.Errorf("failed to refresh JWKS: %w", err)
		}
		if pub, ok := findJWK(jwks, kid); ok {
			return jwkToRSAPublicKey(pub)
		}
		return nil, fmt.Errorf("no key found for kid: %s", kid)
	})
	if err != nil {
		return fmt.Errorf("failed to parse token: %w", err)
	}
	if !token.Valid {
		return fmt.Errorf("token is not valid")
	}

	c, ok := token.Claims.(*claims)
	if !ok {
		return fmt.Errorf("failed to extract claims")
	}

	if err := validateIssuer(c.Issuer, cfg.Issuer); err != nil {
		return err
	}
	if err := validateAudience(c.Audience, cfg.Audience); err != nil {
		return err
	}
	return nil
}

// validateLocalDev parses the token without signature verification (dev-only).
func validateLocalDev(tokenString string) error {
	p := jwt.NewParser()
	token, _, err := p.ParseUnverified(tokenString, jwt.MapClaims{})
	if err != nil {
		return fmt.Errorf("failed to parse token: %w", err)
	}

	claims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return fmt.Errorf("failed to extract claims")
	}

	expVal, hasExp := claims["exp"]
	if hasExp {
		switch v := expVal.(type) {
		case float64:
			if time.Now().Unix() > int64(v) {
				return fmt.Errorf("token has expired")
			}
		}
	}
	return nil
}

func validateIssuer(issuer string, allowed []string) error {
	for _, a := range allowed {
		if strings.TrimSpace(a) == strings.TrimSpace(issuer) {
			return nil
		}
	}
	return fmt.Errorf("invalid issuer: %s", issuer)
}

func validateAudience(audiences jwt.ClaimStrings, allowed []string) error {
	allowedSet := make(map[string]struct{}, len(allowed))
	for _, a := range allowed {
		allowedSet[strings.TrimSpace(a)] = struct{}{}
	}
	for _, aud := range audiences {
		if _, ok := allowedSet[strings.TrimSpace(aud)]; ok {
			return nil
		}
	}
	return fmt.Errorf("no valid audience found")
}

func findJWK(jwks *JWKS, kid string) (*JSONWebKey, bool) {
	for i := range jwks.Keys {
		if jwks.Keys[i].Kid == kid {
			return &jwks.Keys[i], true
		}
	}
	return nil, false
}

func fetchJWKS(ctx context.Context, jwksURL string, force bool) (*JWKS, error) {
	if !force {
		jwksCacheMutex.RLock()
		if jwksCache != nil && time.Since(jwksCacheTime) < jwksCacheTTL {
			cached := jwksCache
			jwksCacheMutex.RUnlock()
			return cached, nil
		}
		jwksCacheMutex.RUnlock()
	}

	jwksCacheMutex.Lock()
	defer jwksCacheMutex.Unlock()

	if !force && jwksCache != nil && time.Since(jwksCacheTime) < jwksCacheTTL {
		return jwksCache, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, jwksURL, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to build JWKS request: %w", err)
	}
	resp, err := jwksHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("JWKS endpoint returned status: %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read JWKS response: %w", err)
	}

	var jwks JWKS
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("failed to parse JWKS: %w", err)
	}

	jwksCache = &jwks
	jwksCacheTime = time.Now()
	return &jwks, nil
}

func jwkToRSAPublicKey(key *JSONWebKey) (*rsa.PublicKey, error) {
	nBytes, err := base64.RawURLEncoding.DecodeString(key.N)
	if err != nil {
		return nil, fmt.Errorf("failed to decode modulus: %w", err)
	}
	eBytes, err := base64.RawURLEncoding.DecodeString(key.E)
	if err != nil {
		return nil, fmt.Errorf("failed to decode exponent: %w", err)
	}

	n := new(big.Int).SetBytes(nBytes)
	eInt := new(big.Int).SetBytes(eBytes)

	return &rsa.PublicKey{N: n, E: int(eInt.Int64())}, nil
}
