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

package secretmanagersvc

import (
	"fmt"
	"sync"
)

var (
	providers = make(map[string]Provider)
	lock      sync.RWMutex
)

// Register adds a provider to the registry.
// Panics if a provider with the same name is already registered.
// This follows the external-secrets registration pattern.
func Register(name string, provider Provider) {
	lock.Lock()
	defer lock.Unlock()

	if _, exists := providers[name]; exists {
		panic(fmt.Sprintf("provider already registered: %s", name))
	}
	providers[name] = provider
}

// GetProvider retrieves a provider by name from the registry.
// Returns the provider and true if found, nil and false otherwise.
func GetProvider(name string) (Provider, bool) {
	lock.RLock()
	defer lock.RUnlock()

	p, ok := providers[name]
	return p, ok
}

// GetProviders returns a list of all registered provider names.
func GetProviders() []string {
	lock.RLock()
	defer lock.RUnlock()

	names := make([]string, 0, len(providers))
	for name := range providers {
		names = append(names, name)
	}
	return names
}
