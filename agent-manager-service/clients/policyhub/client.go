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

// Package policyhub is a cached, read-only client for the policy hub's public
// catalog API. The hub is an enrichment source only: gateways decide which policies
// exist, and the hub contributes display names, descriptions and categories.
package policyhub

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/singleflight"

	"github.com/wso2/agent-manager/agent-manager-service/clients/requests"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
)

const (
	// pageSize is the per-request page size; the hub reports pagination.total, and
	// ListPolicies keeps paging until it has every policy.
	pageSize = 100
	// maxPages bounds the paging loop so a hub that misreports its total cannot
	// keep a refresh running forever.
	maxPages = 50
	// fetchTimeout bounds one full catalog refresh (all pages).
	fetchTimeout = 15 * time.Second
	// failureBackoff is how long a failed refresh suppresses the next attempt, so a
	// hub outage costs one timeout per window instead of one per listing request.
	failureBackoff = 30 * time.Second
)

// Policy is one policy as the hub publishes it. Version is the hub's coarse
// major.minor version, which does not match a gateway's build version, so callers
// should join on Name only.
type Policy struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	DisplayName string   `json:"displayName"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	IsLatest    bool     `json:"isLatest"`
}

// Client lists the hub's policy catalog.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg clientmocks -out ../clientmocks/policy_hub_client_fake.go . Client:PolicyHubClientMock
type Client interface {
	// ListPolicies returns the hub catalog keyed by policy name. The returned map is
	// shared and must not be modified. A disabled client returns an empty map and a
	// nil error. When the hub is unreachable it serves the last good catalog, from this
	// process or from the Store; an error is returned only when neither has one.
	ListPolicies(ctx context.Context) (map[string]Policy, error)
}

type listResponse struct {
	Data       []Policy `json:"data"`
	Pagination struct {
		Offset int `json:"offset"`
		Limit  int `json:"limit"`
		Total  int `json:"total"`
	} `json:"pagination"`
}

type cachingClient struct {
	httpClient  requests.HttpClient
	policiesURL string
	ttl         time.Duration
	store       Store // optional; nil keeps the catalog in this process only
	now         func() time.Time

	sf         singleflight.Group
	mu         sync.RWMutex
	policies   map[string]Policy
	fetchedAt  time.Time
	lastErr    error
	retryAfter time.Time
}

// NewClient builds a Client for the hub at baseURL (e.g. ".../policy-hub-public/v1.0").
// An empty baseURL returns a disabled client whose ListPolicies always returns an
// empty catalog, so enrichment is optional for air-gapped deployments. store, when
// non-nil, persists the last good catalog so it survives restarts and is shared
// across replicas; nil keeps it in this process only.
func NewClient(httpClient requests.HttpClient, baseURL string, ttl time.Duration, store Store) Client {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return disabledClient{}
	}
	return &cachingClient{
		httpClient:  httpClient,
		policiesURL: baseURL + "/policies",
		ttl:         ttl,
		store:       store,
		now:         time.Now,
	}
}

type disabledClient struct{}

func (disabledClient) ListPolicies(context.Context) (map[string]Policy, error) {
	return map[string]Policy{}, nil
}

func (c *cachingClient) ListPolicies(ctx context.Context) (map[string]Policy, error) {
	c.mu.RLock()
	cached, fetchedAt, lastErr, retryAfter := c.policies, c.fetchedAt, c.lastErr, c.retryAfter
	c.mu.RUnlock()
	if cached != nil && c.now().Sub(fetchedAt) < c.ttl {
		return cached, nil
	}

	// The in-process copy is missing or stale. The store may hold a newer one: written
	// by another replica, or by this process before a restart.
	if snapshot, ok := c.loadStored(ctx); ok && snapshot.FetchedAt.After(fetchedAt) {
		c.adopt(snapshot)
		cached, fetchedAt = snapshot.Policies, snapshot.FetchedAt
		if c.now().Sub(fetchedAt) < c.ttl {
			return cached, nil
		}
	}

	now := c.now()
	if now.Before(retryAfter) {
		if cached != nil {
			return cached, nil
		}
		return nil, fmt.Errorf("failed to fetch policy hub catalog: %w", lastErr)
	}

	// One refresh at a time, shared by every concurrent caller. The fetch is detached
	// from the first caller's context so its cancellation does not fail the others;
	// fetchTimeout bounds it instead.
	v, err, _ := c.sf.Do("policies", func() (any, error) {
		fetchCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), fetchTimeout)
		defer cancel()
		policies, err := c.fetchAll(fetchCtx)
		c.mu.Lock()
		if err != nil {
			c.lastErr, c.retryAfter = err, c.now().Add(failureBackoff)
			c.mu.Unlock()
			return nil, err
		}
		snapshot := Snapshot{Policies: policies, FetchedAt: c.now()}
		c.policies, c.fetchedAt = snapshot.Policies, snapshot.FetchedAt
		c.lastErr, c.retryAfter = nil, time.Time{}
		c.mu.Unlock()

		c.saveStored(fetchCtx, snapshot)
		return policies, nil
	})
	if err != nil {
		if cached != nil {
			logger.GetLogger(ctx).Warn("policyhub: refresh failed, serving stale catalog",
				slog.String("url", c.policiesURL),
				slog.Time("fetchedAt", fetchedAt),
				slog.String("error", err.Error()))
			return cached, nil
		}
		return nil, fmt.Errorf("failed to fetch policy hub catalog: %w", err)
	}
	return v.(map[string]Policy), nil
}

// loadStored reads the persisted snapshot. A store error is logged and treated as a
// miss: the store is a cache, so it must never fail a listing.
func (c *cachingClient) loadStored(ctx context.Context) (Snapshot, bool) {
	if c.store == nil {
		return Snapshot{}, false
	}
	snapshot, found, err := c.store.Load(ctx)
	if err != nil {
		logger.GetLogger(ctx).Warn("policyhub: failed to load stored catalog",
			slog.String("url", c.policiesURL),
			slog.String("error", err.Error()))
		return Snapshot{}, false
	}
	if !found || len(snapshot.Policies) == 0 {
		return Snapshot{}, false
	}
	return snapshot, true
}

// adopt replaces the in-process copy with snapshot unless a newer one landed meanwhile.
func (c *cachingClient) adopt(snapshot Snapshot) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if snapshot.FetchedAt.After(c.fetchedAt) {
		c.policies, c.fetchedAt = snapshot.Policies, snapshot.FetchedAt
	}
}

// saveStored persists a freshly fetched snapshot. A failure is logged only: the
// listing already has the fresh catalog in process.
func (c *cachingClient) saveStored(ctx context.Context, snapshot Snapshot) {
	if c.store == nil {
		return
	}
	if err := c.store.Save(ctx, snapshot); err != nil {
		logger.GetLogger(ctx).Warn("policyhub: failed to persist catalog",
			slog.String("url", c.policiesURL),
			slog.String("error", err.Error()))
	}
}

// fetchAll pages through the whole catalog. A name listed more than once keeps the
// entry the hub marks isLatest.
func (c *cachingClient) fetchAll(ctx context.Context) (map[string]Policy, error) {
	policies := map[string]Policy{}
	offset := 0
	for range maxPages {
		req := &requests.HttpRequest{
			Name:   "policyhub.ListPolicies",
			URL:    c.policiesURL,
			Method: http.MethodGet,
		}
		req.SetQuery("limit", strconv.Itoa(pageSize))
		req.SetQuery("offset", strconv.Itoa(offset))

		var resp listResponse
		if err := requests.SendRequest(ctx, c.httpClient, req).ScanResponse(&resp, http.StatusOK); err != nil {
			return nil, fmt.Errorf("list policies at offset %d: %w", offset, err)
		}
		for _, p := range resp.Data {
			if p.Name == "" {
				continue
			}
			if existing, ok := policies[p.Name]; ok && existing.IsLatest && !p.IsLatest {
				continue
			}
			policies[p.Name] = p
		}
		offset += len(resp.Data)
		if len(resp.Data) == 0 || offset >= resp.Pagination.Total {
			logger.GetLogger(ctx).Debug("policyhub: catalog fetched",
				slog.String("url", c.policiesURL),
				slog.Int("count", len(policies)))
			return policies, nil
		}
	}
	return nil, fmt.Errorf("policy hub catalog exceeded %d pages", maxPages)
}
