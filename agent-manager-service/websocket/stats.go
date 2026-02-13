/*
 *  Copyright (c) 2026, WSO2 LLC. (http://www.wso2.org) All Rights Reserved.
 *
 *  Licensed under the Apache License, Version 2.0 (the "License");
 *  you may not use this file except in compliance with the License.
 *  You may obtain a copy of the License at
 *
 *  http://www.apache.org/licenses/LICENSE-2.0
 *
 *  Unless required by applicable law or agreed to in writing, software
 *  distributed under the License is distributed on an "AS IS" BASIS,
 *  WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 *  See the License for the specific language governing permissions and
 *  limitations under the License.
 *
 */

package websocket

import (
	"sync/atomic"
	"time"
)

// Stats tracks delivery statistics for a connection
type Stats struct {
	// TotalEventsSent tracks the total number of events sent over this connection
	TotalEventsSent atomic.Uint64

	// FailedDeliveries tracks the number of failed delivery attempts
	FailedDeliveries atomic.Uint64

	// LastFailureTime records when the most recent delivery failure occurred
	LastFailureTime atomic.Value // stores time.Time

	// LastFailureReason stores the reason for the most recent failure
	LastFailureReason atomic.Value // stores string
}

// NewStats creates a new Stats instance
func NewStats() *Stats {
	return &Stats{}
}

// IncrementTotalSent atomically increments the total events sent counter
func (s *Stats) IncrementTotalSent() {
	s.TotalEventsSent.Add(1)
}

// IncrementFailed atomically increments the failed deliveries counter and records the failure details
func (s *Stats) IncrementFailed(reason string) {
	s.FailedDeliveries.Add(1)
	s.LastFailureTime.Store(time.Now())
	s.LastFailureReason.Store(reason)
}

// GetTotalEventsSent returns the total number of events sent
func (s *Stats) GetTotalEventsSent() uint64 {
	return s.TotalEventsSent.Load()
}

// GetFailedDeliveries returns the number of failed deliveries
func (s *Stats) GetFailedDeliveries() uint64 {
	return s.FailedDeliveries.Load()
}

// GetSuccessRate calculates the delivery success rate as a percentage (0-100)
func (s *Stats) GetSuccessRate() float64 {
	total := s.GetTotalEventsSent()
	if total == 0 {
		return 100.0 // No events sent yet, consider it 100% success
	}
	failed := s.GetFailedDeliveries()
	successful := total - failed
	return (float64(successful) / float64(total)) * 100.0
}

// GetLastFailure returns the time and reason of the most recent failure
func (s *Stats) GetLastFailure() (time.Time, string) {
	timeVal := s.LastFailureTime.Load()
	reasonVal := s.LastFailureReason.Load()

	var failureTime time.Time
	var failureReason string

	if timeVal != nil {
		failureTime = timeVal.(time.Time)
	}
	if reasonVal != nil {
		failureReason = reasonVal.(string)
	}

	return failureTime, failureReason
}
