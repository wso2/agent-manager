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

// ProviderPlacement reports, per provider, the gateways it is placed on (status heading for DEPLOYED) with each one's status.
// It reads one status row per gateway, and skips a gateway that no longer exists.
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
		rows, err := s.deploymentRepo.StatusRecords(ctx, ouID, providerUUID)
		if err != nil {
			return nil, fmt.Errorf("failed to read the placement of %s: %w", providerUUID, err)
		}
		summaries := make([]gwmodels.DeploymentSummary, 0, len(rows))
		for _, row := range rows {
			g, ok := byUUID[row.GatewayUUID]
			if !ok || row.StatusDesired != gwmodels.DeploymentStatusDeployed {
				continue
			}
			updatedAt := row.UpdatedAt
			summaries = append(summaries, gwmodels.DeploymentSummary{
				GatewayID:   row.GatewayUUID,
				GatewayName: g.Name,
				Status:      gwmodels.UserStatusOf(row).Status,
				DeployedAt:  &updatedAt,
				VHost:       g.Vhost,
			})
		}
		out[providerUUID] = summaries
	}
	return out, nil
}
