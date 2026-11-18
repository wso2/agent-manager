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

package gwsvc

import (
	"log/slog"
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// TokenCacheEntry represents a cached token with its gateway and verification data
type TokenCacheEntry struct {
	GatewayUUID uuid.UUID
	Gateway     *models.PlatformGateway // Cache full gateway to avoid second DB lookup
	TokenHash   string                  // Stored hash for verification
	Salt        string
	CachedAt    time.Time
}

// TokenCache provides thread-safe caching of valid gateway tokens
// Uses token prefix (UUID) as cache key for consistency with DB index
type TokenCache struct {
	mu          sync.RWMutex
	tokens      map[string]*TokenCacheEntry // tokenPrefix (UUID) -> entry
	misses      map[string]time.Time        // tokenPrefix (UUID) -> time of last confirmed miss
	lastRefresh time.Time
	ttl         time.Duration
	missTTL     time.Duration
}

// negativeCacheTTL is deliberately much shorter than the positive-entry ttl:
// a real token can appear later (rotation, delayed provisioning), so a stale
// negative entry must not outlive that window for long. Its only job is to
// stop a burst of repeated fake-but-valid-shaped keys from each triggering a
// real DB query.
const negativeCacheTTL = 30 * time.Second

// maxMissEntries bounds the miss cache. Any caller can submit an arbitrary
// UUID-shaped prefix with no authentication, so without a cap a sustained
// stream of unique fake prefixes would grow c.misses without bound. Once the
// cap is hit, RecordMiss evicts the oldest entries first.
const maxMissEntries = 10000

// NewTokenCache creates a new token cache with specified TTL
func NewTokenCache(ttl time.Duration) *TokenCache {
	return &TokenCache{
		tokens:  make(map[string]*TokenCacheEntry),
		misses:  make(map[string]time.Time),
		ttl:     ttl,
		missTTL: negativeCacheTTL,
	}
}

// IsKnownMiss reports whether tokenPrefix was confirmed absent from the DB
// recently enough that a repeat lookup can be skipped.
func (c *TokenCache) IsKnownMiss(tokenPrefix string) bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	missedAt, exists := c.misses[tokenPrefix]
	if !exists {
		return false
	}
	return time.Since(missedAt) <= c.missTTL
}

// RecordMiss remembers that tokenPrefix had no active token in the DB, so
// repeated lookups for the same (usually fake) prefix within missTTL are
// served from memory instead of hitting the database every time.
func (c *TokenCache) RecordMiss(tokenPrefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.purgeExpiredMissesLocked()
	if len(c.misses) >= maxMissEntries {
		c.evictOldestMissesLocked(len(c.misses) - maxMissEntries + 1)
	}
	c.misses[tokenPrefix] = time.Now()
}

// purgeExpiredMissesLocked removes miss entries older than missTTL. Callers
// must hold c.mu for writing.
func (c *TokenCache) purgeExpiredMissesLocked() {
	now := time.Now()
	for prefix, missedAt := range c.misses {
		if now.Sub(missedAt) > c.missTTL {
			delete(c.misses, prefix)
		}
	}
}

// evictOldestMissesLocked removes the n oldest miss entries. Callers must
// hold c.mu for writing.
func (c *TokenCache) evictOldestMissesLocked(n int) {
	if n <= 0 {
		return
	}
	type prefixAge struct {
		prefix   string
		missedAt time.Time
	}
	entries := make([]prefixAge, 0, len(c.misses))
	for prefix, missedAt := range c.misses {
		entries = append(entries, prefixAge{prefix, missedAt})
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].missedAt.Before(entries[j].missedAt) })
	if n > len(entries) {
		n = len(entries)
	}
	for i := 0; i < n; i++ {
		delete(c.misses, entries[i].prefix)
	}
}

// Get retrieves a token entry from cache by prefix if valid
func (c *TokenCache) Get(tokenPrefix string) (*TokenCacheEntry, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.tokens[tokenPrefix]
	if !exists {
		return nil, false
	}

	// Check if entry is still valid based on TTL
	if time.Since(entry.CachedAt) > c.ttl {
		return nil, false
	}

	return entry, true
}

// Set adds or updates a token entry in the cache using prefix as key
func (c *TokenCache) Set(tokenPrefix string, gatewayUUID uuid.UUID, gateway *models.PlatformGateway, tokenHash string, salt string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tokens[tokenPrefix] = &TokenCacheEntry{
		GatewayUUID: gatewayUUID,
		Gateway:     gateway,
		TokenHash:   tokenHash,
		Salt:        salt,
		CachedAt:    time.Now(),
	}
	delete(c.misses, tokenPrefix)
}

// Invalidate removes a specific token from cache by prefix (used on revocation)
func (c *TokenCache) Invalidate(tokenPrefix string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.tokens, tokenPrefix)
	slog.Info("token cache invalidated", "tokenPrefix", tokenPrefix)
}

// InvalidateGateway removes all tokens for a specific gateway
func (c *TokenCache) InvalidateGateway(gatewayUUID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	count := 0
	for hash, entry := range c.tokens {
		if entry.GatewayUUID == gatewayUUID {
			delete(c.tokens, hash)
			count++
		}
	}

	if count > 0 {
		slog.Info("gateway tokens invalidated from cache", "gatewayUUID", gatewayUUID, "count", count)
	}
}

// Clear removes all entries from cache
func (c *TokenCache) Clear() {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tokens = make(map[string]*TokenCacheEntry)
	c.misses = make(map[string]time.Time)
	c.lastRefresh = time.Time{}
	slog.Info("token cache cleared")
}

// Size returns the current number of cached tokens
func (c *TokenCache) Size() int {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return len(c.tokens)
}

// Refresh reloads the cache with current active tokens
func (c *TokenCache) Refresh(tokens map[string]*TokenCacheEntry) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.tokens = tokens
	c.lastRefresh = time.Now()
	slog.Info("token cache refreshed", "count", len(tokens))
}

// NeedsRefresh checks if cache should be refreshed based on TTL
func (c *TokenCache) NeedsRefresh() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	return time.Since(c.lastRefresh) > c.ttl
}
