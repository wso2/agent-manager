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

package aigateway

import (
	"context"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwconfig"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
)

// healthGateways caps the gateways listed in the health detail; the counts
// cover all of them.
const healthGateways = 50

// gatewayHealth is one registered gateway in the health detail.
type gatewayHealth struct {
	ID             string `json:"id"`
	OrganizationID string `json:"organizationId"`
	Name           string `json:"name"`
	Connected      bool   `json:"connected"`
	// Since is when the gateway last connected or disconnected.
	Since time.Time `json:"since"`
}

// Health reports every registered gateway's connection from the gateway
// table, which the replica holding a gateway's WebSocket keeps current, so
// any replica (or amp module health) gives the same answer. No gateway
// connected is not degraded: the gateway-controller workload is not
// required.
func (module) Health(ctx context.Context, rt modkit.Runtime) map[string]any {
	db := rt.DB().WithContext(ctx)
	var total, connected int64
	if err := db.Raw(`SELECT count(*) FROM mod_ai_gateway_gateways`).Scan(&total).Error; err != nil {
		return map[string]any{"error": "reading gateways: " + err.Error()}
	}
	if err := db.Raw(`SELECT count(*) FROM mod_ai_gateway_gateways WHERE is_active = ?`, true).Scan(&connected).Error; err != nil {
		return map[string]any{"error": "reading gateways: " + err.Error()}
	}
	gateways := []gatewayHealth{}
	if err := db.Raw(`SELECT id, org_id AS organization_id, name, is_active AS connected, updated_at AS since
		FROM mod_ai_gateway_gateways ORDER BY org_id, name LIMIT ?`, healthGateways).Scan(&gateways).Error; err != nil {
		return map[string]any{"error": "reading gateways: " + err.Error()}
	}
	detail := map[string]any{"gateways": total, "connected": connected, "registered": gateways}
	if total > healthGateways {
		detail["truncated"] = true
	}
	if connected == 0 {
		detail["status"] = "ok, no gateway connected"
	}
	if settings, err := gwconfig.Load(rt.Config); err != nil {
		detail["failureSummary"] = map[string]any{"error": err.Error()}
	} else if summary, err := gwsvc.NewPlatformGatewayService(gwrepo.NewGatewayRepo(rt.DB())).
		GetCrossOrgGatewayFailureSummary(ctx, gwsvc.GatewayFailureSummaryQuery{
			StalenessThreshold:         settings.Failure.Threshold,
			MaxAge:                     settings.Failure.MaxAge,
			FailurePercentageThreshold: settings.Failure.PercentageThreshold,
		}); err != nil {
		detail["failureSummary"] = map[string]any{"error": err.Error()}
	} else {
		detail["failureSummary"] = summary
	}
	return detail
}
