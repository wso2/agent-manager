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
	"context"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
)

// AgentIdentityListQuery asks for the identity resources one agent has in an
// environment that receives its identity by promotion.
type AgentIdentityListQuery struct {
	OUID         string
	OrgNamespace string
	ProjectName  string
	Environment  string
	// ThunderAgentID is the agent application's id in that environment.
	ThunderAgentID string
}

// MCPResourceServerListing names one MCP resource server of a listing.
type MCPResourceServerListing struct {
	Handle string
	Name   string
}

// AgentIdentityListing is what the control plane holds for an agent in an
// environment: the roles, groups and MCP resource servers that belong to it.
// The agent application itself is reported from the agent's stored identity
// binding, which records its id and client ID.
type AgentIdentityListing struct {
	Roles              []PromotedIdentityResource
	Groups             []PromotedIdentityResource
	MCPResourceServers []MCPResourceServerListing
}

// AgentIdentityEnvironmentReader is an optional capability of an
// IdentityPromotionManager. It lists an agent's identity resources in an
// environment that receives its identity by promotion, through the control
// plane. The data plane of such an environment is never read directly: without
// this capability the listing is unavailable.
type AgentIdentityEnvironmentReader interface {
	ListAgentIdentityResources(ctx context.Context, query AgentIdentityListQuery) (AgentIdentityListing, error)
}

// promotedEnvironmentReader returns the manager's reader, or the typed
// capability error when the deployment cannot list through the control plane.
func (s *agentThunderProvisioningService) promotedEnvironmentReader(envName string) (AgentIdentityEnvironmentReader, error) {
	manager, _, _ := s.promotionDependencies()
	reader, ok := manager.(AgentIdentityEnvironmentReader)
	if !ok {
		return nil, fmt.Errorf("%w: listing identity resources through the control plane is not available for environment %s",
			thundersvc.ErrIdentityCapabilityUnavailable, envName)
	}
	return reader, nil
}

// listPromotedEnvironment reads an environment that receives its identity by
// promotion through the control plane reader.
func (s *agentThunderProvisioningService) listPromotedEnvironment(ctx context.Context, ouID, projectName, envName,
	thunderAgentID string,
) (AgentIdentityListing, error) {
	reader, err := s.promotedEnvironmentReader(envName)
	if err != nil {
		return AgentIdentityListing{}, err
	}
	listing, err := reader.ListAgentIdentityResources(ctx, AgentIdentityListQuery{
		OUID: ouID, OrgNamespace: ThunderOrgNamespace(), ProjectName: projectName, Environment: envName,
		ThunderAgentID: thunderAgentID,
	})
	if err != nil {
		return AgentIdentityListing{}, fmt.Errorf("list agent identity resources in %s: %w", envName, err)
	}
	return listing, nil
}
