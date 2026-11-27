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
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
)

// A fresh prefix has no recorded miss yet, so a first-time lookup must always
// reach the DB rather than being short-circuited as a false "known miss".
func TestTokenCache_IsKnownMiss_UnknownPrefixIsNotAMiss(t *testing.T) {
	c := NewTokenCache(5 * time.Minute)

	assert.False(t, c.IsKnownMiss("never-looked-up"))
}

// After RecordMiss, a repeat lookup for the exact same prefix must be
// recognized as a known miss so the caller can skip the DB entirely — this is
// the mechanism that stops a burst of repeated fake keys from each triggering
// a real query.
func TestTokenCache_RecordMiss_ThenIsKnownMiss(t *testing.T) {
	c := NewTokenCache(5 * time.Minute)

	c.RecordMiss("fake-prefix")

	assert.True(t, c.IsKnownMiss("fake-prefix"))
}

// The negative cache entry must expire on its own short TTL — independent of
// the positive-entry ttl — so a token that starts existing shortly after being
// probed as absent (rotation, delayed provisioning) is not shut out for the
// lifetime of the long positive TTL.
func TestTokenCache_RecordMiss_ExpiresIndependentlyOfPositiveTTL(t *testing.T) {
	c := NewTokenCache(5 * time.Minute)
	c.missTTL = time.Millisecond

	c.RecordMiss("fake-prefix")
	time.Sleep(5 * time.Millisecond)

	assert.False(t, c.IsKnownMiss("fake-prefix"), "a miss entry must not outlive its own (short) TTL")
}

func TestTokenCache_SetGatewayPreservesTokenMiss(t *testing.T) {
	cache := NewTokenCache(5 * time.Minute)
	gateway := gatewayFixture("edge")
	cache.RecordMiss("inactive-prefix")

	cache.SetGateway(gateway)

	assert.True(t, cache.IsKnownMiss("inactive-prefix"))
	cached, found := cache.GetGateway(gateway.UUID)
	assert.True(t, found)
	assert.Same(t, gateway, cached)
}

// Any caller can submit an arbitrary UUID-shaped prefix with no
// authentication, so a sustained stream of unique fake prefixes must not grow
// the miss cache without bound — RecordMiss must evict old entries once the
// cap is hit.
func TestTokenCache_RecordMiss_BoundedByMaxMissEntries(t *testing.T) {
	c := NewTokenCache(5 * time.Minute)

	for i := 0; i < maxMissEntries+500; i++ {
		c.RecordMiss(uuid.New().String())
	}

	assert.LessOrEqual(t, len(c.misses), maxMissEntries, "miss cache must never grow past its configured cap")
}
