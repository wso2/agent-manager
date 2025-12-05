// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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
	"log/slog"

	clients "github.com/wso2-enterprise/agent-management-platform/agent-manager-service/clients/openchoreosvc"
)

type BuildCIManagerService interface {
	HandleBuildCallback(ctx context.Context, agentName string, projectName string, orgName string, imageId string)
}

type buildCIManagerService struct {
	OpenChoreoSvcClient clients.OpenChoreoSvcClient
	logger              *slog.Logger
}

func NewBuildCIManager(openChoreoSvcClient clients.OpenChoreoSvcClient, logger *slog.Logger,
) BuildCIManagerService {
	return &buildCIManagerService{
		OpenChoreoSvcClient: openChoreoSvcClient,
		logger:              logger,
	}
}

func (b *buildCIManagerService) HandleBuildCallback(ctx context.Context, agentName string, projectName string, orgName string, imageId string) {
	_, err := b.OpenChoreoSvcClient.GetProject(ctx, projectName, orgName)
	if err != nil {
		b.logger.Error("Project not found", "project", projectName, "organization", orgName)
		return
	}

	component, err := b.OpenChoreoSvcClient.GetAgentComponent(ctx, orgName, projectName, agentName)
	if err != nil {
		b.logger.Error("Failed to get component", "component", agentName, "project", projectName, "organization", orgName, "error", err)
		return
	}
	if err := b.OpenChoreoSvcClient.DeployBuiltImage(ctx, orgName, projectName, component.Name, imageId); err != nil {
		b.logger.Error("Failed to deploy agent component", "component", component.Name, "error", err)
		return
	}
}
