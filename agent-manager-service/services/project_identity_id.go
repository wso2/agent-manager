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
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
)

// ProjectIdentityIDResolver names a project the way the identity platform keeps it. A deployment
// whose identity control plane keeps its own project for each Agent Manager project, such as WSO2
// Cloud, gives it the ID of the OpenChoreo project, so an AgentID created for the project lands in the
// project the control plane already has instead of one the provider would have to invent.
//
// An empty ID with no error means no ID is known, and the request carries the project name as before.
type ProjectIdentityIDResolver interface {
	ProjectIdentityID(ctx context.Context, organizationID, projectName string) (string, error)
}

// ProjectIdentityIDResolverSetter is an optional hook so tests and deployments can override the
// resolver DBBackedAgentThunderProvisioning builds.
type ProjectIdentityIDResolverSetter interface {
	SetProjectIdentityIDResolver(resolver ProjectIdentityIDResolver)
}

type openChoreoProjectIdentityIDResolver struct {
	oc client.OpenChoreoClient
}

// NewOpenChoreoProjectIdentityIDResolver reads a project's ID (its OpenChoreo UID) from OpenChoreo.
func NewOpenChoreoProjectIdentityIDResolver(oc client.OpenChoreoClient) ProjectIdentityIDResolver {
	return &openChoreoProjectIdentityIDResolver{oc: oc}
}

func (r *openChoreoProjectIdentityIDResolver) ProjectIdentityID(ctx context.Context, organizationID, projectName string) (string, error) {
	project, err := r.oc.GetProject(ctx, organizationID, projectName)
	if err != nil {
		return "", fmt.Errorf("get project: %w", err)
	}
	if project == nil {
		return "", nil
	}
	return strings.TrimSpace(project.UUID), nil
}

// projectIdentityID is the ID the identity platform knows the project by: the resolver's, or the
// project name when none is known (LEGACY_DIRECT, no resolver, or OpenChoreo has no ID for it).
// A resolver that cannot answer is an error, never the name: the name would make the provider
// create a project of its own, and the AgentID would be in the wrong one.
func (s *agentThunderProvisioningService) projectIdentityID(ctx context.Context, organizationID, projectName string) (string, error) {
	if s.projectIDResolver == nil {
		return projectName, nil
	}
	id, err := s.projectIDResolver.ProjectIdentityID(ctx, organizationID, projectName)
	if err != nil {
		return "", fmt.Errorf("read the project's ID from OpenChoreo: %w", err)
	}
	if id == "" {
		return projectName, nil
	}
	return id, nil
}

// SetProjectIdentityIDResolver replaces the resolver. Call it before the service is used.
func (s *agentThunderProvisioningService) SetProjectIdentityIDResolver(resolver ProjectIdentityIDResolver) {
	s.projectIDResolver = resolver
}

var _ ProjectIdentityIDResolverSetter = (*agentThunderProvisioningService)(nil)
