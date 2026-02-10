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
