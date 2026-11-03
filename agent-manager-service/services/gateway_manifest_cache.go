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
	"context"
	"sync"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// GatewayManifestCacheBackend holds the latest policy manifest each gateway pushed,
// keyed by gateway UUID.
//
// A gateway pushes its whole manifest each time its control-plane WebSocket
// (re)connects. Manifests are large and only ever read to answer "which policies does
// this gateway advertise?", so they are not persisted. The cache keeps one entry per
// gateway, with no history and no TTL: an entry lasts until the gateway's next connect
// replaces it, the gateway is deleted, or (Redis) the key is lost. Entries are per
// gateway, not shared, because gateways can run different policy bundles: a self-hosted
// gateway runs whatever its operator installed, and its push must describe only itself,
// never the gateways of other organizations.
//
// Two implementations exist:
//   - InMemoryGatewayManifestCache: process-local, the default. A restarted (or newly
//     scaled-up) replica starts empty and refills as each gateway reconnects to it.
//   - RedisGatewayManifestCache: shared across replicas. Required in HA deployments —
//     an in-process cache is inconsistent there, since each replica only ever sees the
//     pushes routed to it.
//
// Manifests are advisory (they gate policy pickers, not traffic), so a cold window on
// either backend is tolerable.
type GatewayManifestCacheBackend interface {
	// Set replaces the gateway's cached manifest with the one it just pushed.
	Set(ctx context.Context, gatewayID string, manifest map[string]interface{}) error
	// Get returns the gateway's cached manifest, and whether it has pushed one since
	// startup (in-memory backend) or since its key was last deleted (Redis backend).
	Get(ctx context.Context, gatewayID string) (map[string]interface{}, bool, error)
	// Delete drops the gateway's cached manifest, e.g. when the gateway is deleted.
	Delete(ctx context.Context, gatewayID string) error
}

// InMemoryGatewayManifestCache is the process-local GatewayManifestCacheBackend. Safe
// as the default for a single replica; under HA, each replica gets its own independent
// copy, which readers on other replicas cannot see — use RedisGatewayManifestCache there.
type InMemoryGatewayManifestCache struct {
	mu        sync.RWMutex
	manifests map[string]map[string]interface{}
}

// NewInMemoryGatewayManifestCache creates an empty in-process manifest cache.
func NewInMemoryGatewayManifestCache() *InMemoryGatewayManifestCache {
	return &InMemoryGatewayManifestCache{
		mu:        sync.RWMutex{},
		manifests: map[string]map[string]interface{}{},
	}
}

// Set implements GatewayManifestCacheBackend.
func (c *InMemoryGatewayManifestCache) Set(_ context.Context, gatewayID string, manifest map[string]interface{}) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.manifests[gatewayID] = manifest
	return nil
}

// Get implements GatewayManifestCacheBackend.
func (c *InMemoryGatewayManifestCache) Get(_ context.Context, gatewayID string) (map[string]interface{}, bool, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	manifest, ok := c.manifests[gatewayID]
	return manifest, ok, nil
}

// Delete implements GatewayManifestCacheBackend.
func (c *InMemoryGatewayManifestCache) Delete(_ context.Context, gatewayID string) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.manifests, gatewayID)
	return nil
}

// gatewayManifestCache is the process-wide manifest cache backend. It is a package
// var — rather than constructor-injected into every caller — because the writer (the
// gateway manifest push endpoint, via PlatformGatewayService) and the readers (the MCP
// and LLM policy pickers, package-level functions in this package) are constructed
// independently and neither currently threads a shared dependency between them.
// Overwritten at process startup by wiring.ProvideGatewayManifestCacheBackend once
// config selects between "memory" and "redis"; defaults to the in-memory backend so
// tests and any code path that runs before wiring still get a working cache.
var gatewayManifestCache GatewayManifestCacheBackend = NewInMemoryGatewayManifestCache()

// SetGatewayManifestCacheBackend swaps the process-wide manifest cache backend. Called
// once at startup by wiring, based on config.GatewayManifestCache.Backend.
func SetGatewayManifestCacheBackend(backend GatewayManifestCacheBackend) {
	gatewayManifestCache = backend
}

// gatewayManifest returns the manifest to evaluate for a gateway, and whether the
// gateway's policies are known at all: its own cached push, otherwise the manifest still
// on its row (rows written before manifests moved out of the database). A cache read
// failure (e.g. Redis unreachable) degrades to the same row fallback rather than erroring
// the whole policy listing — manifests are advisory, so a stale read beats a hard failure.
//
// known is false when there is neither a cached push nor a non-empty row: the gateway
// has not reported since it registered (or since the cache lost its entry). Callers treat
// that as "not reported yet", not as "supports no policies", and leave the gateway out of
// policy checks until it reports.
func gatewayManifest(gateway *models.Gateway) (manifest map[string]interface{}, known bool) {
	if gateway == nil {
		return nil, false
	}
	cached, ok, err := gatewayManifestCache.Get(context.Background(), gateway.UUID.String())
	if err == nil && ok {
		return cached, true
	}
	return gateway.Manifest, len(gateway.Manifest) > 0
}
