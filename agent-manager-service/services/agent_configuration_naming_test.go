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

package services

import (
	"strings"
	"testing"
)

func TestScopedProxyIdentifier(t *testing.T) {
	tests := []struct {
		name        string
		projectName string
		agentName   string
		configName  string
		envName     string
	}{
		{
			name:        "short names",
			projectName: "default",
			agentName:   "agent-1",
			configName:  "openai",
			envName:     "default",
		},
		{
			name:        "long config name truncated to 10 chars",
			projectName: "default-project",
			agentName:   "my-lonnnnnnnnnngname-ugly-anaaaaaame-agent",
			configName:  "my-looooooooonbgngekshn-fdhhjkddf",
			envName:     "development",
		},
		{
			name:        "config name exactly 10 chars",
			projectName: "proj",
			agentName:   "agent",
			configName:  "1234567890",
			envName:     "dev",
		},
		{
			name:        "config name with special characters",
			projectName: "proj",
			agentName:   "agent",
			configName:  "My_Config Name!",
			envName:     "staging",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := scopedProxyIdentifier(tt.projectName, tt.agentName, tt.configName, tt.envName)

			// Must contain a hyphen separating prefix from hash.
			if !strings.Contains(id, "-") {
				t.Errorf("expected identifier to contain hyphen, got %q", id)
			}

			// Hash suffix is 16 hex chars, prefix is at most 10, separator is 1 → max 27 chars.
			if len(id) > proxyNamePrefixMaxLen+1+16 {
				t.Errorf("identifier too long: %q (len=%d, max=%d)", id, len(id), proxyNamePrefixMaxLen+1+16)
			}

			// Must be a valid K8s name segment (lowercase, alphanumeric + hyphens, no leading/trailing hyphen).
			if id != strings.ToLower(id) {
				t.Errorf("identifier not lowercase: %q", id)
			}
			if strings.HasPrefix(id, "-") || strings.HasSuffix(id, "-") {
				t.Errorf("identifier has leading/trailing hyphen: %q", id)
			}
			if strings.Contains(id, "--") {
				t.Errorf("identifier has consecutive hyphens: %q", id)
			}
		})
	}
}

func TestScopedProxyIdentifierUniqueness(t *testing.T) {
	// Same config name in different agents must produce different identifiers.
	id1 := scopedProxyIdentifier("projectA", "agent-1", "openai", "default")
	id2 := scopedProxyIdentifier("projectA", "agent-2", "openai", "default")
	if id1 == id2 {
		t.Errorf("expected different identifiers for different agents, both got %q", id1)
	}

	// Same config name in different projects must produce different identifiers.
	id3 := scopedProxyIdentifier("projectA", "agent-1", "openai", "default")
	id4 := scopedProxyIdentifier("projectB", "agent-1", "openai", "default")
	if id3 == id4 {
		t.Errorf("expected different identifiers for different projects, both got %q", id3)
	}
}

func TestScopedProxyIdentifierDeterministic(t *testing.T) {
	id1 := scopedProxyIdentifier("proj", "agent", "openai", "dev")
	id2 := scopedProxyIdentifier("proj", "agent", "openai", "dev")
	if id1 != id2 {
		t.Errorf("expected deterministic output, got %q and %q", id1, id2)
	}
}
