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

package thundersvc

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"
)

// envThunderClientCacheTTL bounds how long a resolved ThunderClient is reused
// before its secret is re-read — otherwise a rotated secret needs a restart to take effect.
const envThunderClientCacheTTL = 15 * time.Minute

// ErrThunderNotProvisioned means add-environment-thunder.sh hasn't been run
// for this org/environment yet — the reader returns it (wrapped) for a missing row.
var ErrThunderNotProvisioned = errors.New("env-thunder not provisioned for this environment")

// ErrThunderUnreachable is returned when an env-Thunder's system-client secret
// exists (it has been provisioned) but no candidate base URL responds — unlike
// ErrThunderNotProvisioned, this is expected to be transient (e.g. a cold-starting
// pod, or a momentary network blip) and should be retried, not treated as a
// permanent failure.
var ErrThunderUnreachable = errors.New("env-thunder is provisioned but not reachable")

// EnvThunderResolver resolves a ready-to-use ThunderClient for a specific
// environment's Thunder instance. ouID scopes the credential lookup (a
// per-tenant identity, multi-tenant-safe); orgNamespace addresses the Thunder
// instance itself (namespace/hostname) and is deliberately NOT per-tenant —
// see services.ThunderOrgNamespace's doc comment for why. Callers should go
// through services.ResolveEnvThunderClient/Identity rather than pairing these
// two arguments themselves, so the pairing can't drift between call sites.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/env_thunder_resolver_fake.go . EnvThunderResolver:EnvThunderResolverMock
type EnvThunderResolver interface {
	Resolve(ctx context.Context, ouID, orgNamespace, envName string) (ThunderClient, error)
	// ResolveIdentity returns the same resolved client widened to identity
	// operations (the concrete client implements both interfaces).
	ResolveIdentity(ctx context.Context, ouID, orgNamespace, envName string) (EnvIdentityClient, error)
}

// EnvIdentityClient is the env-Thunder surface the agent-identity passthrough
// needs: full identity management plus default-OU lookup.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/env_identity_client_mock.go . EnvIdentityClient:EnvIdentityClientMock
type EnvIdentityClient interface {
	IdentityClient
	GetDefaultOUID(ctx context.Context) (string, error)
}

type (
	// ReadSystemClientFunc reads an env-Thunder's (clientID, clientSecret) by
	// decrypting it from AMS's own Postgres, keyed by (ouID, envName). Returns
	// ErrThunderNotProvisioned for a missing row.
	ReadSystemClientFunc func(ctx context.Context, ouID, envName string) (clientID, clientSecret string, err error)

	// ReadThunderURLFunc reads an env-Thunder's registered origin (see
	// EnvThunderURL.ThunderURL) from AMS's own Postgres, keyed by (ouID, envName).
	// A missing row means this environment was never provisioned — it returns
	// ("", false, nil), never a value computed from (ouID, envName).
	// callerSupplied reports whether the row's URL was supplied directly by the
	// caller (no handle) rather than computed by AMS itself from a handle —
	// needed so the candidate cascade below can tell an attacker-influenced
	// value from one AMS computed itself under its own trusted domain.
	ReadThunderURLFunc func(ctx context.Context, ouID, envName string) (thunderURL string, callerSupplied bool, err error)

	// resolveBaseURLFunc picks a reachable base URL for an env-Thunder instance —
	// injectable so tests don't depend on real network probing.
	resolveBaseURLFunc func(ctx context.Context, org, env, thunderURL string, callerSupplied bool) (baseURL, resolveToHost string, ok bool)
)

// envThunderResolver reads the system-client credential via the injected
// ReadSystemClientFunc (AMS's Postgres, not a key vault — cloud vaults are reveal-once),
// and the env's registered URL (if any) via the injected ReadThunderURLFunc.
type envThunderResolver struct {
	readSystemClient ReadSystemClientFunc
	readThunderURL   ReadThunderURLFunc
	resolveBaseURL   resolveBaseURLFunc
	ttl              time.Duration
	now              func() time.Time

	mu    sync.RWMutex
	cache map[string]cachedThunderClient // keyed by "org/env"
	sfg   singleflight.Group             // dedupes concurrent cache-miss resolves per key
}

type cachedThunderClient struct {
	client   ThunderClient
	cachedAt time.Time
}

// NewEnvThunderResolver creates an EnvThunderResolver backed by the given
// system-client reader (which decrypts the credential from AMS's Postgres) and
// URL reader (which looks up the registered origin, if any, from the same DB).
func NewEnvThunderResolver(readSystemClient ReadSystemClientFunc, readThunderURL ReadThunderURLFunc) EnvThunderResolver {
	return newEnvThunderResolverWithReader(readSystemClient, readThunderURL, ResolveThunderBaseURL)
}

// newEnvThunderResolverWithReader builds a resolver from injected readers and a
// base-URL resolver — real implementations in production, fakes in tests.
func newEnvThunderResolverWithReader(readSystemClient ReadSystemClientFunc, readThunderURL ReadThunderURLFunc, resolveBaseURL resolveBaseURLFunc) *envThunderResolver {
	return &envThunderResolver{
		readSystemClient: readSystemClient,
		readThunderURL:   readThunderURL,
		resolveBaseURL:   resolveBaseURL,
		ttl:              envThunderClientCacheTTL,
		now:              time.Now,
		cache:            make(map[string]cachedThunderClient),
	}
}

// Resolve returns a ThunderClient for the given environment, caching it per
// (ouID, orgNamespace, env) for envThunderClientCacheTTL to avoid re-reading
// the credential every call.
func (r *envThunderResolver) Resolve(ctx context.Context, ouID, orgNamespace, envName string) (ThunderClient, error) {
	// Fail fast on obviously invalid segments before they reach a DB query or
	// the cache key — cheap defence, keeps the cache key unambiguous.
	for _, seg := range []string{ouID, orgNamespace, envName} {
		if seg == "" || seg == "." || seg == ".." || strings.Contains(seg, "/") {
			return nil, fmt.Errorf("invalid org or environment name segment %q", seg)
		}
	}

	cacheKey := ouID + "/" + orgNamespace + "/" + envName

	r.mu.RLock()
	if entry, ok := r.cache[cacheKey]; ok && r.now().Sub(entry.cachedAt) < r.ttl {
		r.mu.RUnlock()
		return entry.client, nil
	}
	r.mu.RUnlock()

	// Singleflight so concurrent first-time resolves for the same key share one
	// credential read and base-URL probe instead of each paying the cost independently.
	result, err, _ := r.sfg.Do(cacheKey, func() (any, error) {
		r.mu.RLock()
		if entry, ok := r.cache[cacheKey]; ok && r.now().Sub(entry.cachedAt) < r.ttl {
			r.mu.RUnlock()
			return entry.client, nil
		}
		r.mu.RUnlock()

		clientID, clientSecret, err := r.readSystemClient(ctx, ouID, envName)
		if errors.Is(err, ErrThunderNotProvisioned) {
			return nil, ErrThunderNotProvisioned
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read env-thunder system-client secret for %s/%s: %w", ouID, envName, err)
		}
		if clientSecret == "" {
			return nil, ErrThunderNotProvisioned
		}

		// Every environment gets a registered URL at provisioning time — no
		// fallback to a pattern computed from org/env. A missing URL means
		// this env-Thunder was never provisioned, same as a missing
		// system-client secret above: ErrThunderNotProvisioned, not a guess.
		thunderURL, callerSupplied, err := r.readThunderURL(ctx, ouID, envName)
		if err != nil {
			return nil, fmt.Errorf("failed to read env-thunder url for %s/%s: %w", ouID, envName, err)
		}
		if thunderURL == "" {
			return nil, ErrThunderNotProvisioned
		}

		baseURL, resolveToHost, ok := r.resolveBaseURL(ctx, orgNamespace, envName, thunderURL, callerSupplied)
		if !ok {
			return nil, fmt.Errorf("%w: %s/%s", ErrThunderUnreachable, orgNamespace, envName)
		}
		// The System RS identifier is "<issuer>/mcp", derived from the env-Thunder issuer
		// URL — not the (possibly cluster-internal) dialable base URL selected above.
		systemResource := SystemResourceIdentifier(ThunderIssuerURL(thunderURL))
		// baseURL==thunderURL with no override identifies the plain-external
		// candidate; callerSupplied says whether ITS VALUE is attacker-influenced
		// (a caller-supplied row) rather than AMS's own trusted computation (a
		// handle-derived row, which can legitimately be a private address — see
		// thunderURLCandidate's doc comment). Both must hold to harden the dial.
		newClient := NewThunderClientWithDialOverride
		if resolveToHost == "" && baseURL == thunderURL && callerSupplied {
			newClient = NewEnvThunderClient
		}
		client := newClient(baseURL, clientID, clientSecret, resolveToHost, systemResource)

		r.mu.Lock()
		r.cache[cacheKey] = cachedThunderClient{client: client, cachedAt: r.now()}
		r.mu.Unlock()

		return client, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(ThunderClient), nil
}

// ResolveIdentity resolves the env-Thunder client and widens it to the identity
// surface. The concrete client returned by Resolve implements both interfaces.
func (r *envThunderResolver) ResolveIdentity(ctx context.Context, ouID, orgNamespace, envName string) (EnvIdentityClient, error) {
	c, err := r.Resolve(ctx, ouID, orgNamespace, envName)
	if err != nil {
		return nil, err
	}
	ic, ok := c.(EnvIdentityClient)
	if !ok {
		return nil, fmt.Errorf("resolved thunder client for %s/%s does not support identity operations", orgNamespace, envName)
	}
	return ic, nil
}
