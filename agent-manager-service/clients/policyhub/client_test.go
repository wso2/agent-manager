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

package policyhub

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeHub serves a fixed catalog with real offset/limit paging and counts requests.
// Setting failing makes every request return 503.
type fakeHub struct {
	policies []Policy
	requests atomic.Int32
	failing  atomic.Bool
}

func (h *fakeHub) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.requests.Add(1)
	if r.URL.Path != "/v1.0/policies" {
		http.NotFound(w, r)
		return
	}
	if h.failing.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	offset, _ := strconv.Atoi(r.URL.Query().Get("offset"))
	limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
	end := min(offset+limit, len(h.policies))
	page := []Policy{}
	if offset < len(h.policies) {
		page = h.policies[offset:end]
	}
	resp := map[string]interface{}{
		"count":      len(page),
		"data":       page,
		"pagination": map[string]int{"offset": offset, "limit": limit, "total": len(h.policies)},
	}
	_ = json.NewEncoder(w).Encode(resp)
}

// fakeClock is a settable time source for the TTL and backoff windows.
type fakeClock struct {
	mu  sync.Mutex
	now time.Time
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.now = c.now.Add(d)
}

func newTestClient(t *testing.T, hub *fakeHub, ttl time.Duration) (*cachingClient, *fakeClock) {
	t.Helper()
	return newTestClientWithStore(t, hub, ttl, nil)
}

func newTestClientWithStore(t *testing.T, hub *fakeHub, ttl time.Duration, store Store) (*cachingClient, *fakeClock) {
	t.Helper()
	srv := httptest.NewServer(hub)
	t.Cleanup(srv.Close)
	clock := &fakeClock{now: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)}
	c, ok := NewClient(srv.Client(), srv.URL+"/v1.0/", ttl, store).(*cachingClient)
	require.True(t, ok, "a non-empty base URL must build the caching client")
	c.now = clock.Now
	return c, clock
}

func TestListPolicies_PagesThroughWholeCatalog(t *testing.T) {
	hub := &fakeHub{}
	for i := range pageSize + 5 {
		hub.policies = append(hub.policies, Policy{Name: "policy-" + strconv.Itoa(i), IsLatest: true})
	}
	c, _ := newTestClient(t, hub, time.Minute)

	got, err := c.ListPolicies(context.Background())

	require.NoError(t, err)
	assert.Len(t, got, pageSize+5)
	assert.Equal(t, int32(2), hub.requests.Load(), "105 policies at 100 per page is two requests")
}

func TestListPolicies_PrefersLatestWhenNameRepeats(t *testing.T) {
	hub := &fakeHub{policies: []Policy{
		{Name: "cors", Version: "1.1", IsLatest: true, DisplayName: "CORS latest"},
		{Name: "cors", Version: "1.0", IsLatest: false, DisplayName: "CORS old"},
	}}
	c, _ := newTestClient(t, hub, time.Minute)

	got, err := c.ListPolicies(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "CORS latest", got["cors"].DisplayName)
}

func TestListPolicies_ServesFromCacheWithinTTL(t *testing.T) {
	hub := &fakeHub{policies: []Policy{{Name: "cors", IsLatest: true}}}
	c, clock := newTestClient(t, hub, 10*time.Minute)

	_, err := c.ListPolicies(context.Background())
	require.NoError(t, err)
	clock.Advance(9 * time.Minute)
	_, err = c.ListPolicies(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(1), hub.requests.Load(), "a call inside the TTL must not hit the hub")

	clock.Advance(2 * time.Minute)
	_, err = c.ListPolicies(context.Background())
	require.NoError(t, err)
	assert.Equal(t, int32(2), hub.requests.Load(), "a call past the TTL must refresh")
}

func TestListPolicies_ServesStaleCatalogWhenRefreshFails(t *testing.T) {
	hub := &fakeHub{policies: []Policy{{Name: "cors", IsLatest: true}}}
	c, clock := newTestClient(t, hub, time.Minute)

	_, err := c.ListPolicies(context.Background())
	require.NoError(t, err)

	hub.failing.Store(true)
	clock.Advance(2 * time.Minute)
	got, err := c.ListPolicies(context.Background())

	require.NoError(t, err, "a failed refresh with a cached catalog must not surface an error")
	assert.Contains(t, got, "cors")
}

func TestListPolicies_ColdFailureReturnsErrorAndBacksOff(t *testing.T) {
	hub := &fakeHub{}
	hub.failing.Store(true)
	c, clock := newTestClient(t, hub, time.Minute)

	_, err := c.ListPolicies(context.Background())
	require.Error(t, err, "no catalog was ever fetched, so the failure must surface")
	require.Equal(t, int32(1), hub.requests.Load())

	clock.Advance(failureBackoff / 2)
	_, err = c.ListPolicies(context.Background())
	require.Error(t, err)
	assert.Equal(t, int32(1), hub.requests.Load(), "calls inside the backoff window must not retry the hub")

	hub.failing.Store(false)
	hub.policies = []Policy{{Name: "cors", IsLatest: true}}
	clock.Advance(failureBackoff)
	got, err := c.ListPolicies(context.Background())
	require.NoError(t, err, "past the backoff window the hub is retried")
	assert.Contains(t, got, "cors")
}

func TestListPolicies_ConcurrentCallersShareOneFetch(t *testing.T) {
	hub := &fakeHub{policies: []Policy{{Name: "cors", IsLatest: true}}}
	c, _ := newTestClient(t, hub, time.Minute)

	var wg sync.WaitGroup
	for range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := c.ListPolicies(context.Background())
			assert.NoError(t, err)
		}()
	}
	wg.Wait()

	// singleflight collapses overlapping callers; later ones hit the fresh cache.
	assert.Equal(t, int32(1), hub.requests.Load())
}

func TestNewClient_EmptyBaseURLIsDisabled(t *testing.T) {
	c := NewClient(http.DefaultClient, "  ", time.Minute, nil)

	got, err := c.ListPolicies(context.Background())

	require.NoError(t, err)
	assert.NotNil(t, got)
	assert.Empty(t, got)
}

// fakeStore is an in-memory Store whose Load/Save can be made to fail.
type fakeStore struct {
	mu       sync.Mutex
	snapshot *Snapshot
	loadErr  error
	saveErr  error
	saves    int
}

func (s *fakeStore) Load(context.Context) (Snapshot, bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.loadErr != nil {
		return Snapshot{}, false, s.loadErr
	}
	if s.snapshot == nil {
		return Snapshot{}, false, nil
	}
	return *s.snapshot, true, nil
}

func (s *fakeStore) Save(_ context.Context, snapshot Snapshot) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.saves++
	if s.saveErr != nil {
		return s.saveErr
	}
	s.snapshot = &snapshot
	return nil
}

func TestListPolicies_PersistsFetchedCatalogToStore(t *testing.T) {
	hub := &fakeHub{policies: []Policy{{Name: "cors", IsLatest: true}}}
	store := &fakeStore{}
	c, clock := newTestClientWithStore(t, hub, time.Minute, store)

	_, err := c.ListPolicies(context.Background())

	require.NoError(t, err)
	require.NotNil(t, store.snapshot)
	assert.Contains(t, store.snapshot.Policies, "cors")
	assert.Equal(t, clock.Now(), store.snapshot.FetchedAt)
}

func TestListPolicies_ColdStartServesStoredCatalogWhenHubDown(t *testing.T) {
	hub := &fakeHub{}
	hub.failing.Store(true)
	stored := Snapshot{
		Policies:  map[string]Policy{"mcp-ratelimit": {Name: "mcp-ratelimit", DisplayName: "MCP Rate Limit"}},
		FetchedAt: time.Date(2026, 10, 1, 0, 0, 0, 0, time.UTC), // days old: past the TTL
	}
	c, _ := newTestClientWithStore(t, hub, time.Minute, &fakeStore{snapshot: &stored})

	got, err := c.ListPolicies(context.Background())

	require.NoError(t, err, "a restarted process must fall back to the stored catalog, not fail")
	assert.Equal(t, "MCP Rate Limit", got["mcp-ratelimit"].DisplayName)
	assert.Equal(t, int32(1), hub.requests.Load(), "a stale stored copy still triggers a refresh attempt")
}

func TestListPolicies_FreshStoredCatalogSkipsHub(t *testing.T) {
	hub := &fakeHub{policies: []Policy{{Name: "cors", IsLatest: true}}}
	clockStart := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	stored := Snapshot{
		Policies:  map[string]Policy{"set-headers": {Name: "set-headers"}},
		FetchedAt: clockStart.Add(-time.Minute), // written by another replica a minute ago
	}
	c, _ := newTestClientWithStore(t, hub, 10*time.Minute, &fakeStore{snapshot: &stored})

	got, err := c.ListPolicies(context.Background())

	require.NoError(t, err)
	assert.Contains(t, got, "set-headers")
	assert.Equal(t, int32(0), hub.requests.Load(), "a fresh shared copy must not hit the hub")
}

func TestListPolicies_StoreErrorsNeverFailTheListing(t *testing.T) {
	hub := &fakeHub{policies: []Policy{{Name: "cors", IsLatest: true}}}
	store := &fakeStore{loadErr: errors.New("redis down"), saveErr: errors.New("redis down")}
	c, _ := newTestClientWithStore(t, hub, time.Minute, store)

	got, err := c.ListPolicies(context.Background())

	require.NoError(t, err, "a failing store degrades to the hub, it does not surface")
	assert.Contains(t, got, "cors")
	assert.Equal(t, 1, store.saves, "the fresh catalog is still offered to the store")
}

func TestListPolicies_OlderStoredCatalogNeverReplacesNewerInProcessCopy(t *testing.T) {
	hub := &fakeHub{policies: []Policy{{Name: "cors", DisplayName: "CORS fresh", IsLatest: true}}}
	store := &fakeStore{}
	c, clock := newTestClientWithStore(t, hub, time.Minute, store)

	_, err := c.ListPolicies(context.Background())
	require.NoError(t, err)

	// Another writer leaves an older snapshot behind; then the in-process copy expires
	// and the hub goes down.
	store.snapshot = &Snapshot{
		Policies:  map[string]Policy{"cors": {Name: "cors", DisplayName: "CORS old"}},
		FetchedAt: clock.Now().Add(-time.Hour),
	}
	hub.failing.Store(true)
	clock.Advance(2 * time.Minute)

	got, err := c.ListPolicies(context.Background())

	require.NoError(t, err)
	assert.Equal(t, "CORS fresh", got["cors"].DisplayName, "the newer in-process copy wins over an older stored one")
}
