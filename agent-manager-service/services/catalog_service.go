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
	"fmt"
	"log/slog"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
)

// Environment mapping cache with TTL to reduce external API calls
type envMappingCache struct {
	data    map[string]envCacheEntry
	mu      sync.RWMutex
	ttl     time.Duration
	maxSize int
}

type envCacheEntry struct {
	mapping   map[string]string
	expiresAt time.Time
}

// Global cache instance with 5-minute TTL
var globalEnvCache = &envMappingCache{
	data:    make(map[string]envCacheEntry),
	ttl:     5 * time.Minute,
	maxSize: 100, // Limit cache size to prevent memory issues
}

func (c *envMappingCache) get(orgUUID string) (map[string]string, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	entry, exists := c.data[orgUUID]
	if !exists {
		return nil, false
	}

	// Check if entry has expired
	if time.Now().After(entry.expiresAt) {
		return nil, false
	}

	return entry.mapping, true
}

func (c *envMappingCache) set(orgUUID string, mapping map[string]string) {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Simple cache eviction: if at max size, clear oldest entries
	if len(c.data) >= c.maxSize {
		// Clear all expired entries
		now := time.Now()
		for key, entry := range c.data {
			if now.After(entry.expiresAt) {
				delete(c.data, key)
			}
		}
		// If still at max, clear half the cache (simple LRU alternative)
		if len(c.data) >= c.maxSize {
			count := 0
			target := c.maxSize / 2
			for key := range c.data {
				delete(c.data, key)
				count++
				if count >= target {
					break
				}
			}
		}
	}

	c.data[orgUUID] = envCacheEntry{
		mapping:   mapping,
		expiresAt: time.Now().Add(c.ttl),
	}
}

// CatalogService defines the interface for catalog operations
type CatalogService interface {
	ListCatalog(ctx context.Context, orgUUID string, kind string, limit, offset int) ([]models.CatalogEntry, int64, error)
	ListLLMProviders(ctx context.Context, filters *models.CatalogListFilters) ([]models.CatalogLLMProviderEntry, int64, error)
}

type catalogService struct {
	logger           *slog.Logger
	catalogRepo      repositories.CatalogRepository
	artifactRepo     repositories.ArtifactRepository
	openChoreoClient client.OpenChoreoClient
}

// NewCatalogService creates a new catalog service
func NewCatalogService(
	logger *slog.Logger,
	catalogRepo repositories.CatalogRepository,
	artifactRepo repositories.ArtifactRepository,
	openChoreoClient client.OpenChoreoClient,
) CatalogService {
	return &catalogService{
		logger:           logger,
		catalogRepo:      catalogRepo,
		artifactRepo:     artifactRepo,
		openChoreoClient: openChoreoClient,
	}
}

// ListCatalog retrieves catalog entries filtered by kind and organization
func (s *catalogService) ListCatalog(ctx context.Context, orgUUID string, kind string, limit, offset int) ([]models.CatalogEntry, int64, error) {
	s.logger.Info("Listing catalog entries",
		"orgUUID", orgUUID,
		"kind", kind,
		"limit", limit,
		"offset", offset)

	// Validate orgUUID
	if _, err := uuid.Parse(orgUUID); err != nil {
		s.logger.Error("Invalid organization UUID", "orgUUID", orgUUID, "error", err)
		return nil, 0, fmt.Errorf("invalid organization UUID: %w", err)
	}

	var entries []models.CatalogEntry
	var total int64
	var err error

	// Query based on kind filter
	if kind == "" {
		// No kind filter - return all catalog entries
		entries, total, err = s.catalogRepo.ListAll(orgUUID, limit, offset)
	} else {
		// Filter by specific kind
		entries, total, err = s.catalogRepo.ListByKind(orgUUID, kind, limit, offset)
	}

	if err != nil {
		s.logger.Error("Failed to list catalog entries",
			"orgUUID", orgUUID,
			"kind", kind,
			"error", err)
		return nil, 0, fmt.Errorf("failed to list catalog entries: %w", err)
	}

	s.logger.Info("Successfully listed catalog entries",
		"count", len(entries),
		"total", total)

	return entries, total, nil
}

// ListLLMProviders retrieves comprehensive LLM provider catalog entries with deployment details
func (s *catalogService) ListLLMProviders(ctx context.Context, filters *models.CatalogListFilters) ([]models.CatalogLLMProviderEntry, int64, error) {
	s.logger.Info("Listing LLM provider catalog entries",
		"orgUUID", filters.OrganizationUUID,
		"environmentUUID", filters.EnvironmentUUID,
		"name", filters.Name,
		"limit", filters.Limit,
		"offset", filters.Offset)

	// Validate filters
	if err := filters.Validate(); err != nil {
		s.logger.Error("Invalid filters", "error", err)
		return nil, 0, fmt.Errorf("invalid filters: %w", err)
	}

	// Parse organization UUID
	orgParsed, err := uuid.Parse(filters.OrganizationUUID)
	if err != nil {
		s.logger.Error("Invalid organization UUID", "orgUUID", filters.OrganizationUUID, "error", err)
		return nil, 0, fmt.Errorf("invalid organization UUID: %w", err)
	}

	// Get LLM providers from repository using optimized single query
	entries, total, err := s.catalogRepo.ListLLMProviders(filters)
	if err != nil {
		s.logger.Error("Failed to list LLM providers from repository",
			"orgUUID", filters.OrganizationUUID,
			"error", err)
		return nil, 0, fmt.Errorf("failed to list LLM providers: %w", err)
	}

	// Only fetch environment mapping if we have entries with deployments
	// This avoids unnecessary external API calls when there are no deployments to enrich
	hasDeployments := false
	for i := range entries {
		if len(entries[i].Deployments) > 0 {
			hasDeployments = true
			break
		}
	}

	var envMap map[string]string
	if hasDeployments {
		// Try to get environment mapping from cache first
		cached, found := globalEnvCache.get(orgParsed.String())
		if found {
			s.logger.Debug("Using cached environment mapping", "orgUUID", orgParsed.String())
			envMap = cached
		} else {
			// Cache miss - fetch from OpenChoreo and cache the result
			s.logger.Debug("Cache miss - fetching environment mapping from OpenChoreo", "orgUUID", orgParsed.String())
			envMap, err = s.buildEnvironmentMapping(ctx, orgParsed)
			if err != nil {
				s.logger.Warn("Failed to build environment mapping, continuing without environment names", "error", err)
				envMap = make(map[string]string) // Continue with empty map
			} else {
				// Cache the successful result
				globalEnvCache.set(orgParsed.String(), envMap)
				s.logger.Debug("Cached environment mapping", "orgUUID", orgParsed.String(), "count", len(envMap))
			}
		}

		// Resolve environment UUIDs to human-readable names
		// Note: All data (configuration, deployments) is already fetched by repository!
		// We just need to replace environment UUIDs with names
		for i := range entries {
			entry := &entries[i]

			// Resolve environment UUIDs to names for each deployment
			for j := range entry.Deployments {
				envUUID := entry.Deployments[j].EnvironmentName
				if envName, ok := envMap[envUUID]; ok {
					entry.Deployments[j].EnvironmentName = envName
				}
				// If not found in map, keep the UUID (or use gateway name as fallback)
			}
		}
	}

	s.logger.Info("Successfully listed LLM provider catalog entries",
		"count", len(entries),
		"total", total)

	return entries, total, nil
}

// buildEnvironmentMapping fetches all environments and builds UUID to name mapping
func (s *catalogService) buildEnvironmentMapping(ctx context.Context, orgUUID uuid.UUID) (map[string]string, error) {
	// Get organization name first
	orgName, err := s.getOrganizationName(ctx, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to get organization name: %w", err)
	}

	// Fetch all environments from OpenChoreo
	environments, err := s.openChoreoClient.ListEnvironments(ctx, orgName)
	if err != nil {
		return nil, fmt.Errorf("failed to list environments: %w", err)
	}

	// Build environment UUID to name mapping
	envMap := make(map[string]string)
	if environments != nil {
		for _, env := range environments {
			envMap[env.UUID] = env.Name
		}
		s.logger.Info("Built environment mapping", "count", len(envMap))
	}

	return envMap, nil
}

// getOrganizationName retrieves organization name from UUID using repository
func (s *catalogService) getOrganizationName(ctx context.Context, orgUUID uuid.UUID) (string, error) {
	orgName, err := s.artifactRepo.GetOrganizationName(orgUUID.String())
	if err != nil {
		return "", fmt.Errorf("failed to get organization: %w", err)
	}

	return orgName, nil
}
