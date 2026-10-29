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
package handlers

import (
	"context"

	"github.com/wso2/agent-manager/agent-manager-service/services"
)

type GatewayHandler struct {
	gatewaySvc *services.PlatformGatewayService
}

func NewGatewayHandler(gatewaySvc *services.PlatformGatewayService) *GatewayHandler {
	return &GatewayHandler{gatewaySvc: gatewaySvc}
}

func (h *GatewayHandler) ListGateways(_ context.Context, ouID string, environmentID string, limit int, offset int) (*services.GatewayListResponse, error) {
	filters := &services.GatewayListFilters{}
	if environmentID != "" {
		filters.EnvironmentID = &environmentID
	}
	return h.gatewaySvc.ListGateways(&ouID, filters, limit, offset)
}
