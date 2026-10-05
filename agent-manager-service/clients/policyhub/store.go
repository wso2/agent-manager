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
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// redisStoreKey is the single key every replica reads and writes: the hub catalog is
// global, so one shared copy is what lets a restarted or new replica serve enriched
// listings while the hub is unreachable.
const redisStoreKey = "amp:policy-hub-catalog:v1"

// Snapshot is one fetched hub catalog and when it was fetched.
type Snapshot struct {
	Policies  map[string]Policy `json:"policies"`
	FetchedAt time.Time         `json:"fetchedAt"`
}

// Store persists the last good hub catalog beyond this process, so it survives restarts
// and is shared across replicas. It is a cache: a failed Load or Save never fails a
// listing, it only means the hub is consulted (or the in-process copy used) instead.
type Store interface {
	// Load returns the persisted snapshot. found is false, with a nil error, when
	// nothing has been stored yet.
	Load(ctx context.Context) (snapshot Snapshot, found bool, err error)
	// Save replaces the persisted snapshot.
	Save(ctx context.Context, snapshot Snapshot) error
}

// RedisClient is the subset of a go-redis client RedisStore uses; *redis.Client
// satisfies it. Narrow so tests can stand in for Redis without a server.
type RedisClient interface {
	Get(ctx context.Context, key string) *redis.StringCmd
	Set(ctx context.Context, key string, value interface{}, expiration time.Duration) *redis.StatusCmd
}

// RedisStore is the Redis-backed Store.
type RedisStore struct {
	client RedisClient
}

// NewRedisStore builds a Store on an existing Redis client. It does not connect or
// ping; the first Load or Save surfaces connectivity problems.
func NewRedisStore(client RedisClient) *RedisStore {
	return &RedisStore{client: client}
}

// Load implements Store.
func (s *RedisStore) Load(ctx context.Context) (Snapshot, bool, error) {
	data, err := s.client.Get(ctx, redisStoreKey).Bytes()
	if errors.Is(err, redis.Nil) {
		return Snapshot{}, false, nil
	}
	if err != nil {
		return Snapshot{}, false, fmt.Errorf("failed to read policy hub catalog from redis: %w", err)
	}
	var snapshot Snapshot
	if err := json.Unmarshal(data, &snapshot); err != nil {
		return Snapshot{}, false, fmt.Errorf("failed to unmarshal stored policy hub catalog: %w", err)
	}
	return snapshot, true, nil
}

// Save implements Store. The key has no expiry: an old catalog is still the best
// available source of names and categories while the hub is down, and every
// successful refresh replaces it.
func (s *RedisStore) Save(ctx context.Context, snapshot Snapshot) error {
	data, err := json.Marshal(snapshot)
	if err != nil {
		return fmt.Errorf("failed to marshal policy hub catalog: %w", err)
	}
	if err := s.client.Set(ctx, redisStoreKey, data, 0).Err(); err != nil {
		return fmt.Errorf("failed to write policy hub catalog to redis: %w", err)
	}
	return nil
}
