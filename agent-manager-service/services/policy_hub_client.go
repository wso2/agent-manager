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
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"
)

const policyHubPageSize = 50

// HubPolicy is one policy entry from the public WSO2 policy hub.
type HubPolicy struct {
	Name        string   `json:"name"`
	Version     string   `json:"version"`
	DisplayName string   `json:"displayName"`
	Description string   `json:"description"`
	Categories  []string `json:"categories"`
	IsLatest    bool     `json:"isLatest"`
}

// PolicyHubClient reads the public policy hub catalog.
type PolicyHubClient interface {
	// Snapshot returns every hub policy (all categories). ok=false means no data at
	// all: hub disabled, or never fetched successfully. Stale data returns ok=true.
	Snapshot(ctx context.Context) (policies []HubPolicy, ok bool)
}

type policyHubClient struct {
	baseURL string
	ttl     time.Duration
	timeout time.Duration
	hc      *http.Client

	mu        sync.Mutex
	policies  []HubPolicy
	fetchedAt time.Time
	hasData   bool
}

// NewPolicyHubClient builds a cached hub client. An empty baseURL disables the hub.
func NewPolicyHubClient(baseURL string, ttl, timeout time.Duration, hc *http.Client) PolicyHubClient {
	if hc == nil {
		hc = &http.Client{}
	}
	return &policyHubClient{baseURL: strings.TrimSpace(baseURL), ttl: ttl, timeout: timeout, hc: hc}
}

func (c *policyHubClient) Snapshot(ctx context.Context) ([]HubPolicy, bool) {
	if c.baseURL == "" {
		return nil, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.hasData && time.Since(c.fetchedAt) < c.ttl {
		return c.policies, true
	}
	fetchCtx, cancel := context.WithTimeout(ctx, c.timeout)
	defer cancel()
	fresh, err := c.fetchAll(fetchCtx)
	if err != nil {
		// Serve stale data if we have any; otherwise report no data.
		return c.policies, c.hasData
	}
	c.policies, c.fetchedAt, c.hasData = fresh, time.Now(), true
	return c.policies, true
}

type hubPageResponse struct {
	Data       []HubPolicy `json:"data"`
	Pagination struct {
		Total int `json:"total"`
	} `json:"pagination"`
}

func (c *policyHubClient) fetchAll(ctx context.Context) ([]HubPolicy, error) {
	all := []HubPolicy{}
	for offset := 0; ; {
		page, err := c.fetchPage(ctx, offset)
		if err != nil {
			return nil, err
		}
		all = append(all, page.Data...)
		offset += len(page.Data)
		// An empty page guards against looping forever on an inconsistent total.
		if len(page.Data) == 0 || offset >= page.Pagination.Total {
			return all, nil
		}
	}
}

func (c *policyHubClient) fetchPage(ctx context.Context, offset int) (*hubPageResponse, error) {
	u, err := url.Parse(c.baseURL)
	if err != nil {
		return nil, fmt.Errorf("invalid policy hub URL: %w", err)
	}
	q := u.Query()
	q.Set("offset", strconv.Itoa(offset))
	q.Set("limit", strconv.Itoa(policyHubPageSize))
	u.RawQuery = q.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), http.NoBody)
	if err != nil {
		return nil, err
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("policy hub returned status %d", resp.StatusCode)
	}
	var page hubPageResponse
	if err := json.NewDecoder(resp.Body).Decode(&page); err != nil {
		return nil, fmt.Errorf("decode policy hub response: %w", err)
	}
	return &page, nil
}
