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
	"errors"
	"fmt"
	"log/slog"
	"math/big"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"golang.org/x/sync/singleflight"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// Sentinels for the token rejections this package decides itself. They exist so
// the audit trail's reason label is derived from the failure, not from message
// text: several of these messages interpolate claim values taken straight from
// the token, so classifying by substring let the token's own contents choose
// how the rejection was labelled.
var (
	// ErrTokenExpired is a token past its exp claim.
	ErrTokenExpired = errors.New("token has expired")
	// ErrBadIssuer is a token whose iss claim is not configured as trusted.
	ErrBadIssuer = errors.New("token issuer is not allowed")
	// ErrBadAudience is a token whose aud claim matches no configured audience.
	ErrBadAudience = errors.New("token audience is not allowed")
	// ErrUnknownKid is a token whose kid names no key in the JWKS.
	ErrUnknownKid = errors.New("no signing key matches the token kid")
	// ErrBadSignature is a token signed with an unexpected method.
	ErrBadSignature = errors.New("token signature is invalid")
	// ErrMalformedToken is a token that cannot be parsed into claims.
	ErrMalformedToken = errors.New("token is malformed")
	// ErrTokenInvalid is a token the library rejected without a more specific
	// reason.
	ErrTokenInvalid = errors.New("token is not valid")
	// ErrAuthNotConfigured is a rejection caused by this service's own
	// configuration rather than by anything about the token.
	ErrAuthNotConfigured = errors.New("authentication is not configured")
	// ErrKeySetUnavailable is a failure to obtain the JWKS. Also this service's
	// fault, and separated from ErrAuthNotConfigured because it is transient.
	ErrKeySetUnavailable = errors.New("signing key set is unavailable")
)

type TokenClaims struct {
	Sub      string `json:"sub"`
	Scope    string `json:"scope"`
	OuId     string `json:"ouId"`
	OuHandle string `json:"ouHandle"`
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

	jwksRefreshGroup singleflight.Group
	// validKidPattern allows alphanumeric, hyphens, underscores, dots, colons,
	// equals (base64 padding), plus, forward slash, and tilde — covering base64
	// standard and URL-safe encodings commonly used in kid values.
	validKidPattern          = regexp.MustCompile(`^[a-zA-Z0-9._:=+/~-]{1,256}$`)
	validPublisherAudPattern = regexp.MustCompile(`^amp-publisher-[a-zA-Z0-9][a-zA-Z0-9._-]*$`)
)

// PublisherClientAuthMiddleware enforces that at least one JWT audience matches a valid publisher client identity.
// Must be applied after JWTAuthMiddleware so that claims are already in context.
func PublisherClientAuthMiddleware() func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := GetTokenClaims(r.Context())
			if claims == nil || !hasValidPublisherAudience(claims.Audience) {
				utils.WriteErrorResponse(w, http.StatusForbidden, "insufficient permissions")
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

func hasValidPublisherAudience(audiences jwt.ClaimStrings) bool {
	for _, aud := range audiences {
		if validPublisherAudPattern.MatchString(aud) {
			return true
		}
	}
	return false
}

// AuthFailureHook is notified when a request is rejected at the edge. The
// reason is a classified label (see classifyAuthFailure), never the token or
// any fragment of it.
//
// This exists as a hook rather than a direct call because the audit package
// imports this one to read token claims; calling audit from here would be an
// import cycle. The hook is installed once at startup from api/app.go.
type AuthFailureHook func(r *http.Request, reason string)

var authFailureHook atomic.Pointer[AuthFailureHook]

// SetAuthFailureHook installs the handler notified on authentication failure.
// Passing nil removes it.
func SetAuthFailureHook(h AuthFailureHook) {
	if h == nil {
		authFailureHook.Store(nil)
		return
	}
	authFailureHook.Store(&h)
}

// notifyAuthFailure invokes the installed hook, if any.
func notifyAuthFailure(r *http.Request, reason string) {
	if h := authFailureHook.Load(); h != nil {
		(*h)(r, reason)
	}
}

// classifyAuthFailure reduces a validation error to a stable label.
//
// The label is deliberately coarse. It has to distinguish an expired token
// (routine) from a bad signature or unknown issuer (an attack signal) without
// echoing attacker-controlled error text into the audit trail.
//
// Matching is on sentinels, never on message text. Several rejection messages
// interpolate the token's own iss or aud claim, so a substring match let the
// token decide its own label: an issuer containing "expired" was reported as a
// routine expiry rather than as a rejected issuer.
//
// Order matters where one failure wraps another. jwt reports an expired token
// inside ErrTokenInvalidClaims, and reports anything the keyfunc returned
// inside ErrTokenUnverifiable, so the specific causes are tested before the
// general wrappers they arrive in.
func classifyAuthFailure(err error) string {
	switch {
	case err == nil:
		return "unknown"
	case errors.Is(err, ErrTokenExpired), errors.Is(err, jwt.ErrTokenExpired):
		return "expired"
	case errors.Is(err, ErrBadIssuer), errors.Is(err, jwt.ErrTokenInvalidIssuer):
		return "bad-issuer"
	case errors.Is(err, ErrBadAudience), errors.Is(err, jwt.ErrTokenInvalidAudience):
		return "bad-audience"
	case errors.Is(err, ErrUnknownKid):
		return "unknown-kid"
	// This service's own fault, not the caller's. Kept ahead of bad-signature
	// because both arrive wrapped in ErrTokenUnverifiable, and a broken JWKS
	// fetch reported as a bad signature points an investigation at the caller.
	case errors.Is(err, ErrAuthNotConfigured), errors.Is(err, ErrKeySetUnavailable):
		return "server-error"
	case errors.Is(err, ErrBadSignature), errors.Is(err, jwt.ErrTokenSignatureInvalid),
		errors.Is(err, jwt.ErrTokenUnverifiable):
		return "bad-signature"
	case errors.Is(err, ErrMalformedToken), errors.Is(err, jwt.ErrTokenMalformed):
		return "malformed"
	case errors.Is(err, ErrTokenInvalid), errors.Is(err, jwt.ErrTokenNotValidYet),
		errors.Is(err, jwt.ErrTokenInvalidClaims):
		return "invalid"
	default:
		return "unknown"
	}
}

func buildBearerChallenge(resourceMetadataURL, errorCode string) string {
	parts := []string{`realm="agent-manager"`}
	if errorCode != "" {
		parts = append(parts, `error="`+errorCode+`"`)
	}
	if resourceMetadataURL != "" {
		parts = append(parts, `resource_metadata="`+resourceMetadataURL+`"`)
	}
	return "Bearer " + strings.Join(parts, ", ")
}

func JWTAuthMiddleware(header, resourceMetadataURL string) func(http.Handler) http.Handler {
	return JWTAuthMiddlewareWithResourceMetadataResolver(header, func(*http.Request) string {
		return resourceMetadataURL
	})
}

// JWTAuthMiddlewareWithResourceMetadataResolver validates bearer tokens and
// lets each protected resource advertise its own RFC 9728 metadata document.
// This is needed when one HTTP server exposes both its REST API and an MCP
// resource, which intentionally have different resources and scope catalogs.
func JWTAuthMiddlewareWithResourceMetadataResolver(
	header string,
	resourceMetadataURL func(*http.Request) string,
) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			metadataURL := ""
			if resourceMetadataURL != nil {
				metadataURL = resourceMetadataURL(r)
			}
			tokenString := r.Header.Get(header)
			if tokenString == "" {
				notifyAuthFailure(r, "missing-header")
				w.Header().Set("WWW-Authenticate", buildBearerChallenge(metadataURL, ""))
				utils.WriteErrorResponse(w, http.StatusUnauthorized, fmt.Sprintf("missing header: %s", header))
				return
			}
			// replace "Bearer " prefix
			tokenString = strings.Replace(tokenString, "Bearer ", "", 1)

			// Validate the token using JWKS
			claims, err := validateJWTWithJWKS(tokenString)
			if err != nil {
				// The path and client IP make this actionable: the previous form
				// logged only the error, which left credential stuffing against
				// the API undetectable.
				slog.Error("JWT validation failed",
					"error", err,
					"reason", classifyAuthFailure(err),
					"path", utils.SanitizeForLog(r.URL.Path),
					"clientIp", utils.ClientIP(r))
				notifyAuthFailure(r, classifyAuthFailure(err))
				w.Header().Set("WWW-Authenticate", buildBearerChallenge(metadataURL, "invalid_token"))
				utils.WriteErrorResponse(w, http.StatusUnauthorized, "invalid jwt")
				return
			}
			ctx := r.Context()
			ctx = context.WithValue(ctx, assertionTokenClaimsKey, claims)
			ctx = context.WithValue(ctx, jwtToken, tokenString)
			ctx = ContextWithScopes(ctx, claims.Scope)
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

// ContextWithTokenClaims returns ctx carrying the given claims, as if the
// assertion middleware had validated a token. Intended for tests and
// non-HTTP entry points that establish identity out of band.
func ContextWithTokenClaims(ctx context.Context, claims *TokenClaims) context.Context {
	return context.WithValue(ctx, assertionTokenClaimsKey, claims)
}

func GetJWTFromContext(ctx context.Context) string {
	token, ok := ctx.Value(jwtToken).(string)
	if !ok {
		return ""
	}
	return token
}

// effectiveScopes is the parsed scope set that decides one request. It is what
// sits under scopesKey, rather than the raw claim, because a single request
// consults it several times — once per permission the route declares, then
// again in the service layer's environment-tier gate — while a real token
// carries on the order of a hundred scopes. Parsing at the entry point makes
// every one of those checks a map lookup instead of a re-parse.
type effectiveScopes struct {
	set map[string]struct{}
}

func newEffectiveScopes(scopes []string) *effectiveScopes {
	set := make(map[string]struct{}, len(scopes))
	for _, scope := range scopes {
		if scope != "" {
			set[scope] = struct{}{}
		}
	}
	return &effectiveScopes{set: set}
}

// ContextWithScopes returns ctx carrying a space-separated scope string as the
// set HasAllScopes judges against. Use it at any entry point that establishes an
// effective scope set out of band from the HTTP assertion middleware — the MCP
// tool gate does, because its per-request token, not the session, decides the
// call.
func ContextWithScopes(ctx context.Context, scopes string) context.Context {
	return ContextWithScopeList(ctx, strings.Fields(scopes))
}

// ContextWithScopeList is ContextWithScopes for a caller that already holds the
// scopes as a slice — the MCP token verifier hands them over that way — so they
// are not joined into a string only to be split apart again.
func ContextWithScopeList(ctx context.Context, scopes []string) context.Context {
	return context.WithValue(ctx, scopesKey, newEffectiveScopes(scopes))
}

// scopesFromContext returns the effective scope set, or nil if none was
// established. Reading it is how a caller gets the answer the gate gave rather
// than TokenClaims.Scope: on the MCP surface those two differ by design, and the
// whole point of ContextWithScopes is that this is the authoritative set.
func scopesFromContext(ctx context.Context) *effectiveScopes {
	scopes, _ := ctx.Value(scopesKey).(*effectiveScopes)
	return scopes
}

// GrantedScopeCount reports how many distinct scopes decide this request. Audit
// denials record the count instead of the scopes themselves: it distinguishes
// "token with no scopes at all" from "token missing this one scope" without
// copying a potentially huge claim into every record.
func GrantedScopeCount(ctx context.Context) int {
	scopes := scopesFromContext(ctx)
	if scopes == nil {
		return 0
	}
	return len(scopes.set)
}

// FirstMissingScope returns the first permission in perms whose scope the
// request's effective scope set does not carry, and whether there was one.
//
// Every gate on this codebase's AND paths — the route middleware, the MCP tool
// gate, the service layer's environment-tier check — needs the same two facts:
// whether the caller is short, and which grant to name if so. Naming the *first*
// one is deliberate where permissions build on each other: telling someone who
// lacks the environment floor to go get the production grant sends them after
// the wrong permission.
func FirstMissingScope(ctx context.Context, perms ...rbac.Permission) (rbac.Permission, bool) {
	scopes := scopesFromContext(ctx)
	for _, perm := range perms {
		// No scope set was established at all — no token, or an entry point that
		// never installed one. Everything the route asks for is missing, so the
		// first one asked for is the one to name.
		if scopes == nil {
			return perm, true
		}
		if _, held := scopes.set[perm.Scope()]; !held {
			return perm, true
		}
	}
	return "", false
}

// HoldsAnyScope reports whether the effective scope set carries at least one of
// perms — the OR counterpart to FirstMissingScope.
func HoldsAnyScope(ctx context.Context, perms ...rbac.Permission) bool {
	scopes := scopesFromContext(ctx)
	if scopes == nil {
		return false
	}
	for _, perm := range perms {
		if _, held := scopes.set[perm.Scope()]; held {
			return true
		}
	}
	return false
}

func HasAllScopes(ctx context.Context, requiredScopes []string) bool {
	scopes := scopesFromContext(ctx)
	if scopes == nil {
		return false
	}
	for _, scope := range requiredScopes {
		if _, exists := scopes.set[scope]; !exists {
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
		return nil, fmt.Errorf("%w: configuration not loaded", ErrAuthNotConfigured)
	}

	var claims *TokenClaims

	// If JWKS URL is configured, validate signature with JWKS
	if cfg.KeyManagerConfigurations.JWKSUrl != "" {
		// Perform full JWKS validation with signature verification
		token, err := jwt.ParseWithClaims(tokenString, &TokenClaims{}, func(token *jwt.Token) (interface{}, error) {
			// Verify signing method is RSA
			if _, ok := token.Method.(*jwt.SigningMethodRSA); !ok {
				return nil, fmt.Errorf("%w: unexpected signing method: %v", ErrBadSignature, token.Header["alg"])
			}

			// Get the key ID from the token header
			kid, ok := token.Header["kid"].(string)
			if !ok {
				return nil, fmt.Errorf("%w: kid not found in token header", ErrUnknownKid)
			}

			// Fetch JWKS and get the public key
			jwks, err := fetchJWKS(cfg.KeyManagerConfigurations.JWKSUrl)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to fetch JWKS: %w", ErrKeySetUnavailable, err)
			}

			// Find the key with matching kid
			for _, key := range jwks.Keys {
				if key.Kid == kid {
					return convertJWKToPublicKey(&key)
				}
			}

			// kid not found — fetchJWKS may have returned a cached or fresh result
			// depending on TTL. Only attempt a forced refresh if the kid looks
			// plausible (to avoid network calls for garbage values).
			if !validKidPattern.MatchString(kid) {
				return nil, fmt.Errorf("%w: kid has an invalid format", ErrUnknownKid)
			}

			slog.Warn("kid not found in JWKS, attempting refresh", slog.String("kid", kid))
			refreshed, err := refreshJWKS(cfg.KeyManagerConfigurations.JWKSUrl)
			if err != nil {
				return nil, fmt.Errorf("%w: failed to refresh JWKS: %w", ErrKeySetUnavailable, err)
			}
			for _, key := range refreshed.Keys {
				if key.Kid == kid {
					return convertJWKToPublicKey(&key)
				}
			}

			return nil, fmt.Errorf("%w: kid absent after JWKS refresh", ErrUnknownKid)
		})
		if err != nil {
			return nil, fmt.Errorf("failed to parse token: %w", err)
		}

		if !token.Valid {
			return nil, fmt.Errorf("%w", ErrTokenInvalid)
		}

		validatedClaims, ok := token.Claims.(*TokenClaims)
		if !ok {
			return nil, fmt.Errorf("%w: failed to extract claims", ErrMalformedToken)
		}
		claims = validatedClaims
	} else if cfg.IsLocalDevEnv {
		// Dev-only: no JWKS URL configured — extract claims without signature validation.
		// Only reachable when IS_LOCAL_DEV_ENV=true; fail closed in all other environments.
		extractedClaims, err := extractClaimsFromJWT(tokenString)
		if err != nil {
			return nil, fmt.Errorf("%w: failed to extract claims: %w", ErrMalformedToken, err)
		}
		claims = extractedClaims

		if claims.ExpiresAt != nil && !claims.ExpiresAt.After(time.Now()) {
			return nil, fmt.Errorf("%w", ErrTokenExpired)
		}
	} else {
		return nil, fmt.Errorf("%w: KEY_MANAGER_JWKS_URL must be configured for JWT validation", ErrAuthNotConfigured)
	}

	if err := validateIssuer(claims.Issuer, cfg.KeyManagerConfigurations.Issuer); err != nil {
		return nil, err
	}

	if err := validateAudience(claims.Audience, cfg.KeyManagerConfigurations.Audience); err != nil {
		return nil, err
	}

	return claims, nil
}

// validateIssuer validates the issuer claim against allowed issuers
func validateIssuer(issuer string, allowedIssuers []string) error {
	if len(allowedIssuers) == 0 {
		return fmt.Errorf("%w: no allowed issuers configured", ErrAuthNotConfigured)
	}

	trimmedIssuer := strings.TrimSpace(issuer)
	for _, allowed := range allowedIssuers {
		if strings.TrimSpace(allowed) == trimmedIssuer {
			return nil
		}
	}
	return fmt.Errorf("%w: got %s", ErrBadIssuer, issuer)
}

// validateAudience validates the audience claim against allowed audiences.
// Supports exact matches and prefix matches (entries ending with "*").
func validateAudience(audiences jwt.ClaimStrings, allowedAudiences []string) error {
	if len(allowedAudiences) == 0 {
		return fmt.Errorf("%w: no allowed audiences configured", ErrAuthNotConfigured)
	}

	exactAllowed := make(map[string]struct{})
	var prefixAllowed []string
	for _, allowed := range allowedAudiences {
		a := strings.TrimSpace(allowed)
		if a == "*" {
			return fmt.Errorf("%w: bare wildcard \"*\" is not allowed in audience configuration", ErrAuthNotConfigured)
		}
		if strings.HasSuffix(a, "*") {
			prefix := strings.TrimSuffix(a, "*")
			if prefix == "" {
				return fmt.Errorf("%w: bare wildcard \"*\" is not allowed in audience configuration", ErrAuthNotConfigured)
			}
			prefixAllowed = append(prefixAllowed, prefix)
		} else {
			exactAllowed[a] = struct{}{}
		}
	}

	// Check if any token audience matches an allowed entry
	for _, aud := range audiences {
		trimmed := strings.TrimSpace(aud)
		if _, ok := exactAllowed[trimmed]; ok {
			return nil
		}
		for _, prefix := range prefixAllowed {
			if strings.HasPrefix(trimmed, prefix) {
				return nil
			}
		}
	}

	return fmt.Errorf("%w: got %v", ErrBadAudience, audiences)
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
	defer func() { _ = resp.Body.Close() }()

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

// refreshJWKS forces a re-fetch of JWKS, bypassing the cache TTL.
// Concurrent callers are coalesced via singleflight, and a minimum interval
// between refreshes prevents amplification from many unknown-kid requests.
func refreshJWKS(jwksURL string) (*JWKS, error) {
	const minRefreshInterval = 30 * time.Second

	// If we refreshed very recently, return the current cache instead of fetching again.
	jwksCacheMutex.RLock()
	if jwksCache != nil && time.Since(jwksCacheTime) < minRefreshInterval {
		cached := jwksCache
		jwksCacheMutex.RUnlock()
		return cached, nil
	}
	jwksCacheMutex.RUnlock()

	// Deduplicate concurrent refresh attempts.
	result, err, _ := jwksRefreshGroup.Do("refresh", func() (interface{}, error) {
		// Double-check inside singleflight — another goroutine may have just refreshed.
		jwksCacheMutex.RLock()
		if jwksCache != nil && time.Since(jwksCacheTime) < minRefreshInterval {
			cached := jwksCache
			jwksCacheMutex.RUnlock()
			return cached, nil
		}
		jwksCacheMutex.RUnlock()

		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(jwksURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
		}
		defer func() { _ = resp.Body.Close() }()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("JWKS endpoint returned status: %d", resp.StatusCode)
		}

		var jwks JWKS
		if err := json.NewDecoder(resp.Body).Decode(&jwks); err != nil {
			return nil, fmt.Errorf("failed to decode JWKS: %w", err)
		}

		jwksCacheMutex.Lock()
		jwksCache = &jwks
		jwksCacheTime = time.Now()
		jwksCacheMutex.Unlock()

		return &jwks, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*JWKS), nil
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
