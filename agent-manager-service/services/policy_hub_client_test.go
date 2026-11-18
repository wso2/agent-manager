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

func hubPage(w http.ResponseWriter, names []string, offset, limit, total int) {
	body := `{"count":` + strconv.Itoa(len(names)) + `,"data":[`
	for i, n := range names {
		if i > 0 {
			body += ","
		}
		body += fmt.Sprintf(`{"name":%q,"version":"1.0","displayName":"D-%s","description":"d","categories":["Security","AI"],"isLatest":true}`, n, n)
	}
	body += fmt.Sprintf(`],"pagination":{"offset":%d,"limit":%d,"total":%d}}`, offset, limit, total)
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(body))
}

func TestPolicyHubClient_FetchesAllPages(t *testing.T) {
	var sawAuth atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "" {
			sawAuth.Store(true)
		}
		switch r.URL.Query().Get("offset") {
		case "0":
			hubPage(w, []string{"a", "b"}, 0, 2, 3)
		case "2":
			hubPage(w, []string{"c"}, 2, 2, 3)
		default:
			w.WriteHeader(http.StatusBadRequest)
		}
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, time.Second, srv.Client())
	policies, ok := c.Snapshot(context.Background())

	require.True(t, ok)
	require.Len(t, policies, 3)
	assert.Equal(t, "a", policies[0].Name)
	assert.Equal(t, "D-c", policies[2].DisplayName)
	assert.Equal(t, []string{"Security", "AI"}, policies[0].Categories)
	assert.False(t, sawAuth.Load())
}

func TestPolicyHubClient_CachesWithinTTL(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		hubPage(w, []string{"a"}, 0, 50, 1)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, time.Second, srv.Client())
	_, ok1 := c.Snapshot(context.Background())
	_, ok2 := c.Snapshot(context.Background())

	assert.True(t, ok1)
	assert.True(t, ok2)
	assert.Equal(t, int32(1), hits.Load())
}

func TestPolicyHubClient_ServesStaleOnError(t *testing.T) {
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if fail.Load() {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hubPage(w, []string{"a"}, 0, 50, 1)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Nanosecond, time.Second, srv.Client())
	first, ok := c.Snapshot(context.Background())
	require.True(t, ok)
	require.Len(t, first, 1)

	fail.Store(true)
	second, ok := c.Snapshot(context.Background())

	assert.True(t, ok)
	assert.Equal(t, first, second)
}

func TestPolicyHubClient_ColdFailureNotOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, time.Second, srv.Client())
	policies, ok := c.Snapshot(context.Background())

	assert.False(t, ok)
	assert.Empty(t, policies)
}

func TestPolicyHubClient_TimeoutBounded(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(time.Second):
		case <-r.Context().Done():
		}
		hubPage(w, []string{"a"}, 0, 50, 1)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, 50*time.Millisecond, srv.Client())
	start := time.Now()
	policies, ok := c.Snapshot(context.Background())

	assert.Less(t, time.Since(start), 500*time.Millisecond)
	assert.False(t, ok)
	assert.Empty(t, policies)
}

func TestPolicyHubClient_EmptyBaseURLDisabled(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		hits.Add(1)
	}))
	defer srv.Close()

	c := NewPolicyHubClient("", time.Minute, time.Second, srv.Client())
	policies, ok := c.Snapshot(context.Background())

	assert.False(t, ok)
	assert.Empty(t, policies)
	assert.Zero(t, hits.Load())
}

func TestPolicyHubClient_FailureBackoffSkipsRefetch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, time.Second, srv.Client())
	_, ok1 := c.Snapshot(context.Background())
	_, ok2 := c.Snapshot(context.Background())

	assert.False(t, ok1)
	assert.False(t, ok2)
	assert.Equal(t, int32(1), hits.Load())
}

func TestPolicyHubClient_RetriesAfterBackoffElapses(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, time.Second, srv.Client()).(*policyHubClient)
	c.backoff = time.Nanosecond
	c.Snapshot(context.Background())
	c.Snapshot(context.Background())

	assert.Equal(t, int32(2), hits.Load())
}

func TestPolicyHubClient_ConcurrentColdCallersShareOneFetch(t *testing.T) {
	var hits atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hits.Add(1)
		time.Sleep(100 * time.Millisecond)
		hubPage(w, []string{"a"}, 0, 50, 1)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, 2*time.Second, srv.Client())
	const n = 8
	var wg sync.WaitGroup
	oks := make([]bool, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			_, oks[i] = c.Snapshot(context.Background())
		}(i)
	}
	wg.Wait()

	assert.Equal(t, int32(1), hits.Load())
	for _, ok := range oks {
		assert.True(t, ok)
	}
}

// The caller that starts the shared cold fetch may give up; the fetch must still finish for the callers waiting on
// it, populate the cache and not start the failure backoff.
func TestPolicyHubClient_StarterCancelDoesNotFailSharedFetch(t *testing.T) {
	var hits atomic.Int32
	started := make(chan struct{})
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if hits.Add(1) == 1 {
			close(started)
		}
		<-release
		hubPage(w, []string{"a"}, 0, 50, 1)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, 5*time.Second, srv.Client()).(*policyHubClient)

	starterCtx, cancelStarter := context.WithCancel(context.Background())
	starterOK := make(chan bool, 1)
	go func() {
		_, ok := c.Snapshot(starterCtx)
		starterOK <- ok
	}()
	<-started

	waiterOK := make(chan bool, 1)
	go func() {
		_, ok := c.Snapshot(context.Background())
		waiterOK <- ok
	}()
	require.Eventually(t, func() bool {
		c.mu.Lock()
		defer c.mu.Unlock()
		return c.inflight != nil
	}, time.Second, time.Millisecond)
	time.Sleep(50 * time.Millisecond) // let the waiter reach the inflight wait

	cancelStarter()
	close(release)

	assert.True(t, <-waiterOK)
	assert.True(t, <-starterOK)
	got, ok := c.Snapshot(context.Background())
	assert.True(t, ok)
	assert.Len(t, got, 1)
	assert.Equal(t, int32(1), hits.Load())
	c.mu.Lock()
	defer c.mu.Unlock()
	assert.True(t, c.failedAt.IsZero(), "a cancelled starter must not start the backoff")
	assert.Nil(t, c.inflight)
}

func TestPolicyHubClient_SnapshotReturnsCopy(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		hubPage(w, []string{"a"}, 0, 50, 1)
	}))
	defer srv.Close()

	c := NewPolicyHubClient(srv.URL, time.Minute, time.Second, srv.Client())
	first, _ := c.Snapshot(context.Background())
	first[0].Name = "mutated"
	second, _ := c.Snapshot(context.Background())

	assert.Equal(t, "a", second[0].Name)
}
