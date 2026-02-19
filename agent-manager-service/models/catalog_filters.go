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

package models

import (
	"fmt"
)

// CatalogListFilters contains filter criteria for catalog queries
type CatalogListFilters struct {
	// OrganizationUUID is the organization identifier (required)
	OrganizationUUID string

	// Kind filters by catalog resource type (optional)
	// Valid values: "LlmProvider", "agent", "mcp"
	Kind string

	// Name filters by provider/agent name (optional)
	// Supports partial matching
	Name string

	// EnvironmentUUID filters LLM providers by deployment environment (optional)
	// Only applies when Kind == "LlmProvider"
	EnvironmentUUID string

	// Pagination
	Limit  int
	Offset int
}

// Validate validates the filter parameters
func (f *CatalogListFilters) Validate() error {
	if f.OrganizationUUID == "" {
		return fmt.Errorf("organization UUID is required")
	}

	if f.Limit < 0 {
		return fmt.Errorf("limit must be non-negative")
	}

	if f.Offset < 0 {
		return fmt.Errorf("offset must be non-negative")
	}

	// Validate name length to prevent DoS attacks via excessive pattern matching
	if len(f.Name) > 255 {
		return fmt.Errorf("name filter exceeds maximum length of 255 characters")
	}

	// Validate kind if provided
	if f.Kind != "" {
		validKinds := map[string]bool{
			CatalogKindLLMProvider: true,
			CatalogKindAgent:       true,
			CatalogKindMCP:         true,
		}
		if !validKinds[f.Kind] {
			return fmt.Errorf("invalid kind: %s", f.Kind)
		}
	}

	return nil
}

// HasEnvironmentFilter returns true if environment filtering is requested
func (f *CatalogListFilters) HasEnvironmentFilter() bool {
	return f.EnvironmentUUID != ""
}

// HasNameFilter returns true if name filtering is requested
func (f *CatalogListFilters) HasNameFilter() bool {
	return f.Name != ""
}
