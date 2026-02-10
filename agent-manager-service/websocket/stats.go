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

package websocket

import (
	"sync"
	"sync/atomic"
	"time"
)

// DeliveryStats tracks delivery statistics for a WebSocket connection
type DeliveryStats struct {
	TotalEventsSent   int64
	FailedDeliveries  int64
	LastFailureTime   time.Time
	LastFailureReason string
	mu                sync.RWMutex
}

// IncrementTotalSent atomically increments the total events sent counter
func (s *DeliveryStats) IncrementTotalSent() {
	atomic.AddInt64(&s.TotalEventsSent, 1)
}

// IncrementFailed atomically increments the failed deliveries counter and records failure details
func (s *DeliveryStats) IncrementFailed(reason string) {
	atomic.AddInt64(&s.FailedDeliveries, 1)
	s.mu.Lock()
	s.LastFailureTime = time.Now()
	s.LastFailureReason = reason
	s.mu.Unlock()
}

// GetTotalSent returns the total number of events sent
func (s *DeliveryStats) GetTotalSent() int64 {
	return atomic.LoadInt64(&s.TotalEventsSent)
}

// GetFailedDeliveries returns the total number of failed deliveries
func (s *DeliveryStats) GetFailedDeliveries() int64 {
	return atomic.LoadInt64(&s.FailedDeliveries)
}

// GetLastFailure returns the last failure time and reason
func (s *DeliveryStats) GetLastFailure() (time.Time, string) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.LastFailureTime, s.LastFailureReason
}
