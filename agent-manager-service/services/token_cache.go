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
	"sort"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

type gatewayCacheEntry struct {
	gateway  *models.PlatformGateway
	cachedAt time.Time
}

// TokenCache caches gateway metadata and prefixes without an active token.
// Every successful verification still needs an active token record.
type TokenCache struct {
	mu         sync.RWMutex
	gateways   map[uuid.UUID]gatewayCacheEntry
	misses     map[string]time.Time
	gatewayTTL time.Duration
	missTTL    time.Duration
}

const negativeCacheTTL = 30 * time.Second

// maxMissEntries bounds the miss cache. Any caller can submit an arbitrary
// UUID-shaped prefix with no authentication, so without a cap a sustained
// stream of unique fake prefixes would grow c.misses without bound. Once the
// cap is hit, RecordMiss evicts the oldest entries first.
const maxMissEntries = 10000

// NewTokenCache creates a token cache with the given gateway metadata TTL.
func NewTokenCache(gatewayTTL time.Duration) *TokenCache {
	return &TokenCache{
		gateways:   make(map[uuid.UUID]gatewayCacheEntry),
		misses:     make(map[string]time.Time),
		gatewayTTL: gatewayTTL,
		missTTL:    negativeCacheTTL,
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

// GetGateway retrieves cached gateway metadata for an already verified token.
func (c *TokenCache) GetGateway(gatewayUUID uuid.UUID) (*models.PlatformGateway, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.gateways[gatewayUUID]
	if !exists || time.Since(entry.cachedAt) > c.gatewayTTL {
		return nil, false
	}
	return entry.gateway, true
}

// SetGateway caches gateway metadata without changing the recorded token misses.
func (c *TokenCache) SetGateway(gateway *models.PlatformGateway) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.gateways[gateway.UUID] = gatewayCacheEntry{gateway: gateway, cachedAt: time.Now()}
}

// InvalidateGateway evicts cached gateway metadata.
func (c *TokenCache) InvalidateGateway(gatewayUUID uuid.UUID) {
	c.mu.Lock()
	defer c.mu.Unlock()

	delete(c.gateways, gatewayUUID)
}
