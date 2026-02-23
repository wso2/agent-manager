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

package catalog

import (
	"strings"

	"github.com/google/uuid"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// catalogNamespace is a fixed namespace UUID used to derive deterministic evaluator IDs from identifiers.
// Using the DNS namespace UUID as a stable, well-known base.
var catalogNamespace = uuid.MustParse("6ba7b810-9dad-11d1-80b4-00c04fd430c8")

// Entry is a builtin evaluator catalog entry
type Entry struct {
	Identifier   string
	DisplayName  string
	Description  string
	Version      string
	Provider     string
	ClassName    string
	Tags         []string
	ConfigSchema []models.EvaluatorConfigParam
}

// ID returns a deterministic UUID derived from the evaluator identifier.
// The same identifier always produces the same UUID.
func (e *Entry) ID() uuid.UUID {
	return uuid.NewSHA1(catalogNamespace, []byte(e.Identifier))
}

// entryIndex is a lazily-built map for O(1) identifier lookups.
var entryIndex map[string]*Entry

func buildIndex() {
	entryIndex = make(map[string]*Entry, len(entries))
	for _, e := range entries {
		entryIndex[e.Identifier] = e
	}
}

// Get returns a builtin evaluator by identifier, or nil if not found.
func Get(identifier string) *Entry {
	if entryIndex == nil {
		buildIndex()
	}
	return entryIndex[identifier]
}

// List returns all builtin evaluators matching the given filters.
// All filters are AND-ed together; empty/nil values match everything.
func List(tags []string, provider, search string) []*Entry {
	var result []*Entry
	for _, e := range entries {
		if !matchesTags(e, tags) || !matchesProvider(e, provider) || !matchesSearch(e, search) {
			continue
		}
		result = append(result, e)
	}
	return result
}

// All returns all builtin evaluator entries.
func All() []*Entry {
	return entries
}

func matchesTags(e *Entry, tags []string) bool {
	for _, t := range tags {
		found := false
		for _, et := range e.Tags {
			if et == t {
				found = true
				break
			}
		}
		if !found {
			return false
		}
	}
	return true
}

func matchesProvider(e *Entry, provider string) bool {
	return provider == "" || e.Provider == provider
}

func matchesSearch(e *Entry, search string) bool {
	if search == "" {
		return true
	}
	s := strings.ToLower(search)
	return strings.Contains(strings.ToLower(e.DisplayName), s) ||
		strings.Contains(strings.ToLower(e.Description), s)
}

// floatPtr returns a pointer to a float64 value.
// Used by the generated builtin_evaluators.go for Min/Max fields.
func floatPtr(v float64) *float64 {
	return &v
}
