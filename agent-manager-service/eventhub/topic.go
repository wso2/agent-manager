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

package eventhub

import (
	"errors"
	"sync"
	"time"
)

var (
	ErrGatewayNotFound      = errors.New("gateway not found")
	ErrGatewayAlreadyExists = errors.New("gateway already exists")
	ErrSubscriberNotFound   = errors.New("subscriber not found")
)

type gateway struct {
	id                string
	subscribers       []chan Event
	knownVersion      string
	lastPolledTime    time.Time // zero means no event has been delivered yet
	lastPolledEventID string    // event_id of the last delivered event; used as tie-breaker
	queuedLoggedAt    int64     // unix nano of last "events queued, no subscribers" log; 0 = not yet logged
}

type gatewayRegistry struct {
	mu       sync.RWMutex
	gateways map[string]*gateway
}

func newGatewayRegistry() *gatewayRegistry {
	return &gatewayRegistry{gateways: make(map[string]*gateway)}
}

func (r *gatewayRegistry) register(gatewayID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.gateways[gatewayID]; exists {
		return ErrGatewayAlreadyExists
	}
	r.gateways[gatewayID] = &gateway{id: gatewayID, subscribers: make([]chan Event, 0)}
	return nil
}

func (r *gatewayRegistry) addSubscriber(gatewayID string, ch chan Event) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	gw, exists := r.gateways[gatewayID]
	if !exists {
		return ErrGatewayNotFound
	}
	gw.subscribers = append(gw.subscribers, ch)
	return nil
}

func (r *gatewayRegistry) removeSubscriber(gatewayID string, subscriber <-chan Event) (chan Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	gw, exists := r.gateways[gatewayID]
	if !exists {
		return nil, ErrGatewayNotFound
	}
	for i, ch := range gw.subscribers {
		if (<-chan Event)(ch) != subscriber {
			continue
		}
		last := len(gw.subscribers) - 1
		gw.subscribers[i] = gw.subscribers[last]
		gw.subscribers[last] = nil
		gw.subscribers = gw.subscribers[:last]
		return ch, nil
	}
	return nil, ErrSubscriberNotFound
}

func (r *gatewayRegistry) removeAllSubscribers(gatewayID string) ([]chan Event, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	gw, exists := r.gateways[gatewayID]
	if !exists {
		return nil, ErrGatewayNotFound
	}
	subscribers := gw.subscribers
	gw.subscribers = nil
	return subscribers, nil
}

// forEach calls fn for each gateway while holding the read lock.
// fn must not retain the *gateway pointer after it returns.
func (r *gatewayRegistry) forEach(fn func(*gateway)) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, gw := range r.gateways {
		fn(gw)
	}
}

// get returns the gateway pointer for the given ID, or nil if not found.
// The caller MUST hold r.mu (read or write) before calling and for the
// duration of any access to fields on the returned pointer.
func (r *gatewayRegistry) get(gatewayID string) *gateway {
	return r.gateways[gatewayID]
}
