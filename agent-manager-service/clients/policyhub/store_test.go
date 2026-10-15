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
	"errors"
	"testing"
	"time"

	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// fakeRedis is an in-memory RedisClient recording the key and expiry of each write.
// getErr/setErr make the next calls fail the way a go-redis command would.
type fakeRedis struct {
	values     map[string]string
	lastKey    string
	lastExpiry time.Duration
	getErr     error
	setErr     error
}

func newFakeRedis() *fakeRedis {
	return &fakeRedis{values: map[string]string{}}
}

func (f *fakeRedis) Get(_ context.Context, key string) *redis.StringCmd {
	if f.getErr != nil {
		return redis.NewStringResult("", f.getErr)
	}
	v, ok := f.values[key]
	if !ok {
		return redis.NewStringResult("", redis.Nil)
	}
	return redis.NewStringResult(v, nil)
}

func (f *fakeRedis) Set(_ context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd {
	f.lastKey, f.lastExpiry = key, expiration
	if f.setErr != nil {
		return redis.NewStatusResult("", f.setErr)
	}
	switch v := value.(type) {
	case []byte:
		f.values[key] = string(v)
	case string:
		f.values[key] = v
	default:
		return redis.NewStatusResult("", errors.New("fakeRedis: unsupported value type"))
	}
	return redis.NewStatusResult("OK", nil)
}

func TestRedisStore_RoundTripsSnapshot(t *testing.T) {
	rdb := newFakeRedis()
	store := NewRedisStore(rdb)
	want := Snapshot{
		Policies: map[string]Policy{
			"mcp-ratelimit": {Name: "mcp-ratelimit", Version: "1.0", DisplayName: "MCP Rate Limit", Categories: []string{"MCP", "Security"}, IsLatest: true},
		},
		FetchedAt: time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC),
	}

	require.NoError(t, store.Save(context.Background(), want))
	got, found, err := store.Load(context.Background())

	require.NoError(t, err)
	require.True(t, found)
	assert.Equal(t, want, got)
	assert.Equal(t, redisStoreKey, rdb.lastKey, "every replica must share one key")
	assert.Equal(t, time.Duration(0), rdb.lastExpiry, "the stored catalog must not expire: it is the outage fallback")
}

func TestRedisStore_LoadMissingKeyIsNotFound(t *testing.T) {
	store := NewRedisStore(newFakeRedis())

	_, found, err := store.Load(context.Background())

	require.NoError(t, err, "an empty store is not an error")
	assert.False(t, found)
}

func TestRedisStore_LoadErrorIsReportedNotTreatedAsMissing(t *testing.T) {
	connErr := errors.New("connection refused")
	rdb := newFakeRedis()
	rdb.getErr = connErr
	store := NewRedisStore(rdb)

	_, found, err := store.Load(context.Background())

	require.ErrorIs(t, err, connErr)
	assert.False(t, found)
}

func TestRedisStore_LoadCorruptValueIsAnError(t *testing.T) {
	rdb := newFakeRedis()
	rdb.values[redisStoreKey] = "{not json"
	store := NewRedisStore(rdb)

	_, found, err := store.Load(context.Background())

	require.Error(t, err)
	assert.False(t, found)
}

func TestRedisStore_SaveErrorIsWrapped(t *testing.T) {
	writeErr := errors.New("READONLY replica")
	rdb := newFakeRedis()
	rdb.setErr = writeErr
	store := NewRedisStore(rdb)

	err := store.Save(context.Background(), Snapshot{Policies: map[string]Policy{"cors": {Name: "cors"}}})

	require.ErrorIs(t, err, writeErr)
}
