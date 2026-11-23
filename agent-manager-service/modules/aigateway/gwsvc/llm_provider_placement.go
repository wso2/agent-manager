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

package gwsvc

import (
	"context"
	"fmt"

	"github.com/google/uuid"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// ProviderPlacement reports where providers are placed, one summary per gateway holding each.
// It takes a slice so a list page costs two queries rather than two per row, and skips a
// deployment whose gateway is gone - gatewayName is required and a blank one tells a reader nothing.
func (s *LLMProviderService) ProviderPlacement(
	ctx context.Context, ouID string, providers []uuid.UUID,
) (map[uuid.UUID][]gwmodels.DeploymentSummary, error) {
	out := make(map[uuid.UUID][]gwmodels.DeploymentSummary, len(providers))
	if len(providers) == 0 {
		return out, nil
	}

	gateways, err := s.gatewayRepo.GetByOrganizationID(ctx, ouID)
	if err != nil {
		return nil, fmt.Errorf("failed to read the organization's gateways: %w", err)
	}
	byUUID := make(map[uuid.UUID]*gwmodels.Gateway, len(gateways))
	for _, g := range gateways {
		byUUID[g.UUID] = g
	}

	for _, providerUUID := range providers {
		deployments, err := s.deploymentRepo.GetDeploymentsWithState(providerUUID.String(), ouID, nil, nil, 0)
		if err != nil {
			return nil, fmt.Errorf("failed to read deployments of %s: %w", providerUUID, err)
		}
		summaries := make([]gwmodels.DeploymentSummary, 0, len(deployments))
		for _, d := range deployments {
			g, ok := byUUID[d.GatewayUUID]
			if !ok {
				continue
			}
			summary := gwmodels.DeploymentSummary{
				GatewayID:   d.GatewayUUID,
				GatewayName: g.Name,
				DeployedAt:  d.UpdatedAt,
				VHost:       g.Vhost,
			}
			// The status a USER is shown, not the raw row: a deployment with a
			// retry owed reads as DEPLOYING, which is what the v1 catalog
			// already reports for the same row.
			summary.Status = gwmodels.UserStatusOf(gwmodels.DeploymentStatusRecord{
				Status:        deref(d.Status),
				StatusDesired: deref(d.StatusDesired),
				StatusReason:  deref(d.StatusReason),
				NextAttemptAt: d.NextAttemptAt,
				RenderReason:  deref(d.RenderReason),
			}).Status
			summaries = append(summaries, summary)
		}
		out[providerUUID] = summaries
	}
	return out, nil
}

func deref[T any](p *T) T {
	var zero T
	if p == nil {
		return zero
	}
	return *p
}
