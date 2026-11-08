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

// Package clillmprovider holds amctl llm-provider commands as thin,
// assertion-backed operations over the amctl harness, mirroring
// operations/llmprovider (HTTP).
package clillmprovider

import (
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
)

// LLMProvider is the subset of the CLI's llm-provider envelope data we assert on
// (matches the server's LLMProviderResponse / LLMProviderListItem shapes).
type LLMProvider struct {
	ID       string `json:"id"`
	Name     string `json:"name"`
	Template string `json:"template"`
}

// LLMProviderList is the data shape of `amctl llm-provider list --json`.
type LLMProviderList struct {
	Providers []LLMProvider `json:"providers"`
}

// DeleteResult is the data shape of `amctl llm-provider delete --json`.
type DeleteResult struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

// CreateLLMProvider runs `amctl llm-provider create` with no upstream or API key,
// so the CLI defers to the template, and deploys the provider to the given
// gateways (names or UUIDs). With no gateways the provider is config-only.
func CreateLLMProvider(g Gomega, h *amctl.Harness, org, id, displayName, template string, gateways ...string) LLMProvider {
	args := []string{
		"llm-provider", "create", id,
		"--display-name", displayName,
		"--template", template,
		"--org", org,
		"--json",
	}
	for _, gw := range gateways {
		args = append(args, "--gateways", gw)
	}
	return amctl.DecodeData[LLMProvider](g, h.Run(args...))
}

// ListLLMProviders runs `amctl llm-provider list` and returns the provider list.
func ListLLMProviders(g Gomega, h *amctl.Harness, org string) LLMProviderList {
	return amctl.DecodeData[LLMProviderList](g, h.Run(
		"llm-provider", "list", "--org", org, "--json",
	))
}

// DeleteLLMProvider runs `amctl llm-provider delete --yes` and returns the result.
func DeleteLLMProvider(g Gomega, h *amctl.Harness, org, id string) DeleteResult {
	return amctl.DecodeData[DeleteResult](g, h.Run(
		"llm-provider", "delete", id, "--org", org, "--yes", "--json",
	))
}

// Ids returns the provider ids in the list, for membership assertions.
func (l LLMProviderList) Ids() []string {
	ids := make([]string, 0, len(l.Providers))
	for _, p := range l.Providers {
		ids = append(ids, p.ID)
	}
	return ids
}
