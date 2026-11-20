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

// PolicyCatalogKind selects which policy picker a catalog is built for.
type PolicyCatalogKind string

const (
	// PolicyCatalogKindLLM is the LLM guardrail picker catalog.
	PolicyCatalogKindLLM PolicyCatalogKind = "llm"
	// PolicyCatalogKindMCP is the MCP proxy policy picker catalog.
	PolicyCatalogKindMCP PolicyCatalogKind = "mcp"
)

// PolicyAvailabilityScope says which gateways availability was checked against.
type PolicyAvailabilityScope string

const (
	// PolicyAvailabilityScopeDeployed means the gateways the resource is deployed to.
	PolicyAvailabilityScopeDeployed PolicyAvailabilityScope = "deployed"
	// PolicyAvailabilityScopeOrgManaged means the org's active managed gateways.
	PolicyAvailabilityScopeOrgManaged PolicyAvailabilityScope = "org_managed"
)

// PolicyCatalogSource is where a catalog entry's definition comes from.
type PolicyCatalogSource string

const (
	// PolicyCatalogSourceHub is a policy published in the policy hub.
	PolicyCatalogSourceHub PolicyCatalogSource = "hub"
	// PolicyCatalogSourceCustom is an organization-managed policy reported by a gateway.
	PolicyCatalogSourceCustom PolicyCatalogSource = "custom"
)

// PolicyCatalogStatus says whether a catalog entry is installed on the checked gateways.
type PolicyCatalogStatus string

const (
	// PolicyCatalogStatusInstalled means every reporting gateway has the policy.
	PolicyCatalogStatusInstalled PolicyCatalogStatus = "installed"
	// PolicyCatalogStatusNotInstalled means the policy is absent from the checked gateways.
	PolicyCatalogStatusNotInstalled PolicyCatalogStatus = "not_installed"
)

// PolicyCatalogGatewayRef identifies a gateway in a catalog entry.
type PolicyCatalogGatewayRef struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PolicyCatalogItem is one policy in the merged catalog.
type PolicyCatalogItem struct {
	Name             string                    `json:"name"`
	Version          string                    `json:"version"`
	DisplayName      string                    `json:"displayName,omitempty"`
	Description      string                    `json:"description,omitempty"`
	Source           PolicyCatalogSource       `json:"source"`
	Status           PolicyCatalogStatus       `json:"status"`
	MissingOn        []PolicyCatalogGatewayRef `json:"missingOn,omitempty"`
	Categories       []string                  `json:"categories"`
	Parameters       map[string]interface{}    `json:"parameters,omitempty"`
	SystemParameters map[string]interface{}    `json:"systemParameters,omitempty"`
}

// PolicyCatalogResponse is the merged policy catalog for one kind.
type PolicyCatalogResponse struct {
	Count             int32                   `json:"count"`
	List              []PolicyCatalogItem     `json:"list"`
	HubAvailable      bool                    `json:"hubAvailable"`
	AvailabilityScope PolicyAvailabilityScope `json:"availabilityScope"`
}
