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

package services

import (
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
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
	lastRefresh time.Time
	ttl         time.Duration
}

// NewTokenCache creates a new token cache with specified TTL
func NewTokenCache(ttl time.Duration) *TokenCache {
	return &TokenCache{
		tokens: make(map[string]*TokenCacheEntry),
		ttl:    ttl,
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
