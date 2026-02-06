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

package gateway

import (
	"fmt"
	"log/slog"
)

// AdapterConstructor is a function that creates an adapter instance
type AdapterConstructor func(config AdapterConfig, logger *slog.Logger) (IGatewayAdapter, error)

// AdapterFactory creates gateway adapters based on configuration
type AdapterFactory struct {
	adapters map[string]AdapterConstructor
	logger   *slog.Logger
}

// NewAdapterFactory creates a new factory instance
func NewAdapterFactory(logger *slog.Logger) *AdapterFactory {
	return &AdapterFactory{
		adapters: make(map[string]AdapterConstructor),
		logger:   logger,
	}
}

// Register adds an adapter constructor to the factory
func (f *AdapterFactory) Register(adapterType string, constructor AdapterConstructor) {
	f.adapters[adapterType] = constructor
}

// CreateAdapter creates an adapter instance based on configuration
func (f *AdapterFactory) CreateAdapter(config AdapterConfig) (IGatewayAdapter, error) {
	constructor, exists := f.adapters[config.Type]
	if !exists {
		return nil, fmt.Errorf("unsupported adapter type: %s", config.Type)
	}

	return constructor(config, f.logger)
}

// ListSupportedTypes returns all registered adapter types
func (f *AdapterFactory) ListSupportedTypes() []string {
	types := make([]string, 0, len(f.adapters))
	for t := range f.adapters {
		types = append(types, t)
	}
	return types
}
