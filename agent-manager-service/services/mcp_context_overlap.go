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

	"github.com/google/uuid"
	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
)

// mcpGatewayContext is the context the gateway serves the proxy at. A project proxy's context is
// relative to its project, so the gateway serves it at /<project><context>; an org proxy's context is
// served as is. projectName falls back to the proxy's own project when empty.
func mcpGatewayContext(proxy *models.MCPProxy, projectName string) string {
	context := ""
	if proxy.Configuration.Context != nil {
		context = strings.TrimSpace(*proxy.Configuration.Context)
	}
	if proxy.ProjectUUID == nil {
		return context
	}
	if projectName == "" && proxy.ProjectName != nil {
		projectName = *proxy.ProjectName
	}
	return strings.TrimRight("/"+strings.Trim(projectName, "/")+"/"+strings.TrimPrefix(context, "/"), "/")
}

// contextsOverlap reports whether two gateway contexts collide: they are equal, or one is a path-segment
// prefix of the other (/a and /a/b do, /a and /ab don't). The root context contains every other.
func contextsOverlap(a, b string) bool {
	a, b = normalizeGatewayContext(a), normalizeGatewayContext(b)
	if a == "/" || b == "/" || a == b {
		return true
	}
	return strings.HasPrefix(b, a+"/") || strings.HasPrefix(a, b+"/")
}

func normalizeGatewayContext(c string) string {
	c = strings.TrimRight(strings.TrimSpace(c), "/")
	if c == "" {
		return "/"
	}
	if !strings.HasPrefix(c, "/") {
		c = "/" + c
	}
	return c
}

// deployedArtifactContext is the spec.context of a gateway artifact's deployment content, the field every kind
// AMS deploys (Mcp, LlmProvider, LlmProxy, RestApi, Agent) carries; "" when it has none.
func deployedArtifactContext(content []byte) string {
	var artifact struct {
		Spec struct {
			Context string `yaml:"context"`
		} `yaml:"spec"`
	}
	if err := yaml.Unmarshal(content, &artifact); err != nil {
		return ""
	}
	return strings.TrimSpace(artifact.Spec.Context)
}

// gatewayContextClash returns the context of an artifact on the gateway, of any kind other than self, that
// overlaps gatewayContext, or "" when none does. It reads what the gateway is told to hold, the deployments its
// sync lists, so an artifact that is placed but has never been sent (PENDING) is not seen.
func gatewayContextClash(
	ctx context.Context, deployments repositories.DeploymentRepository, ouID string, gateway, self uuid.UUID,
	gatewayContext string,
) (string, error) {
	rows, err := deployments.ListForGatewaySync(ctx, ouID, gateway.String())
	if err != nil {
		return "", fmt.Errorf("failed to list the gateway's artifacts: %w", err)
	}
	ids := make([]uuid.UUID, 0, len(rows))
	for _, row := range rows {
		if row.ArtifactUUID == self || row.State != models.GatewaySyncStateDeployed {
			continue
		}
		id, err := uuid.Parse(row.DeploymentID)
		if err != nil {
			continue
		}
		ids = append(ids, id)
	}
	if len(ids) == 0 {
		return "", nil
	}
	contents, err := deployments.ListContentForGateway(ctx, ouID, gateway.String(), ids)
	if err != nil {
		return "", fmt.Errorf("failed to read the gateway's artifacts: %w", err)
	}
	for _, content := range contents {
		if content.ArtifactUUID == self {
			continue
		}
		if other := deployedArtifactContext(content.Content); other != "" && contextsOverlap(other, gatewayContext) {
			return other, nil
		}
	}
	return "", nil
}
