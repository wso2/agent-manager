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

import "sync"

const (
	// mcpServerInfoMaxInFlight caps fetch-server-info probes running at once across
	// the service; each one can hold a goroutine and an upstream connection for up
	// to mcpServerInfoDeadline.
	mcpServerInfoMaxInFlight = 32
	// mcpServerInfoMaxInFlightPerOrg keeps one organization from taking every slot.
	mcpServerInfoMaxInFlightPerOrg = 4
)

// concurrencyLimiter bounds in-flight work both overall and per key. It never
// blocks: a caller over either limit is refused at once, so a refused request
// holds nothing while it waits.
type concurrencyLimiter struct {
	mu        sync.Mutex
	maxTotal  int
	maxPerKey int
	total     int
	perKey    map[string]int
}

func newConcurrencyLimiter(maxTotal, maxPerKey int) *concurrencyLimiter {
	return &concurrencyLimiter{maxTotal: maxTotal, maxPerKey: maxPerKey, perKey: map[string]int{}}
}

// tryAcquire takes a slot for key and returns its release func, or false when a
// limit is reached. The release func is safe to call more than once.
func (l *concurrencyLimiter) tryAcquire(key string) (func(), bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.total >= l.maxTotal || l.perKey[key] >= l.maxPerKey {
		return nil, false
	}
	l.total++
	l.perKey[key]++
	var once sync.Once
	return func() {
		once.Do(func() {
			l.mu.Lock()
			defer l.mu.Unlock()
			l.total--
			if l.perKey[key]--; l.perKey[key] <= 0 {
				delete(l.perKey, key)
			}
		})
	}, true
}

// mcpServerInfoLimiter is shared by every MCPProxyService so the limit holds per pod.
var mcpServerInfoLimiter = newConcurrencyLimiter(mcpServerInfoMaxInFlight, mcpServerInfoMaxInFlightPerOrg)
