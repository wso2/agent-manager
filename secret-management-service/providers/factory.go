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

package providers

import (
	"fmt"
)

// ProviderFactory is a function type for creating providers
type ProviderFactory func(cfg Config) (Provider, error)

// registry holds registered provider factories
var registry = make(map[ProviderType]ProviderFactory)

// RegisterProvider registers a provider factory
func RegisterProvider(providerType ProviderType, factory ProviderFactory) {
	registry[providerType] = factory
}

// NewProvider creates the appropriate KV provider based on the provider type
func NewProvider(providerType ProviderType, cfg Config) (Provider, error) {
	factory, ok := registry[providerType]
	if !ok {
		return nil, fmt.Errorf("unsupported KV provider: %s", providerType)
	}
	return factory(cfg)
}

// DetectProvider determines the provider type from address patterns
func DetectProvider(address string) ProviderType {
	// Default to OpenBao
	return ProviderOpenBao
}
