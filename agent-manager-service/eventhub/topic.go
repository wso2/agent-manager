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
)

var (
	ErrGatewayNotFound      = errors.New("gateway not found")
	ErrGatewayAlreadyExists = errors.New("gateway already exists")
	ErrSubscriberNotFound   = errors.New("subscriber not found")
)

type gateway struct {
	id           string
	subscribers  []chan Event
	knownVersion string
	lastPolled   int64
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

func (r *gatewayRegistry) getAll() []*gateway {
	r.mu.RLock()
	defer r.mu.RUnlock()
	gateways := make([]*gateway, 0, len(r.gateways))
	for _, gw := range r.gateways {
		gateways = append(gateways, gw)
	}
	return gateways
}
