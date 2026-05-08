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
	"fmt"

	traceobserversvc "github.com/wso2/agent-manager/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// For runtime logs and metrics
type ObservabilityHandler struct {
	agentSvc    services.AgentManagerService
	traceClient traceobserversvc.TraceObserverSvcClient
}

func NewObservabilityHandler(agentSvc services.AgentManagerService, traceClient traceobserversvc.TraceObserverSvcClient) *ObservabilityHandler {
	return &ObservabilityHandler{agentSvc: agentSvc, traceClient: traceClient}
}

func (h *ObservabilityHandler) GetRuntimeLogs(ctx context.Context, orgName string, projectName string, agentName string, payload spec.LogFilterRequest) (*models.LogsResponse, error) {
	return h.agentSvc.GetAgentRuntimeLogs(ctx, orgName, projectName, agentName, payload)
}

func (h *ObservabilityHandler) GetMetrics(ctx context.Context, orgName string, projectName string, agentName string, payload spec.MetricsFilterRequest) (*spec.MetricsResponse, error) {
	return h.agentSvc.GetAgentMetrics(ctx, orgName, projectName, agentName, payload)
}

func (h *ObservabilityHandler) ListTraces(ctx context.Context, orgName string, projectName string, agentName string, environment string, startTime string, endTime string, sortOrder string, limit int) (map[string]any, error) {
	if h.traceClient == nil {
		return nil, fmt.Errorf("trace observer client is not configured")
	}

	params := traceobserversvc.TraceListParams{
		Organization: orgName,
		Project:      projectName,
		Component:    agentName,
		Environment:  environment,
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
		SortOrder:    sortOrder,
	}

	return h.traceClient.ListTraces(ctx, params)
}

func (h *ObservabilityHandler) ExportTraces(ctx context.Context, orgName string, projectName string, agentName string, environment string, startTime string, endTime string, sortOrder string, limit int) (map[string]any, error) {
	if h.traceClient == nil {
		return nil, fmt.Errorf("trace observer client is not configured")
	}

	params := traceobserversvc.TraceListParams{
		Organization: orgName,
		Project:      projectName,
		Component:    agentName,
		Environment:  environment,
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
		SortOrder:    sortOrder,
	}

	return h.traceClient.ExportTraces(ctx, params)
}

func (h *ObservabilityHandler) GetTraceDetails(ctx context.Context, orgName string, projectName string, agentName string, traceID string, environment string, startTime string, endTime string, limit int) (map[string]any, error) {
	if h.traceClient == nil {
		return nil, fmt.Errorf("trace observer client is not configured")
	}

	params := traceobserversvc.TraceDetailsParams{
		TraceID:      traceID,
		Organization: orgName,
		Project:      projectName,
		Component:    agentName,
		Environment:  environment,
		StartTime:    startTime,
		EndTime:      endTime,
		Limit:        limit,
	}

	return h.traceClient.GetTrace(ctx, params)
}

func (h *ObservabilityHandler) GetSpanDetails(ctx context.Context, orgName string, projectName string, agentName string, traceID string, spanID string, environment string) (map[string]any, error) {
	if h.traceClient == nil {
		return nil, fmt.Errorf("trace observer client is not configured")
	}

	params := traceobserversvc.SpanDetailsParams{
		TraceID:      traceID,
		SpanID:       spanID,
		Organization: orgName,
		Project:      projectName,
		Component:    agentName,
		Environment:  environment,
	}

	return h.traceClient.GetSpan(ctx, params)
}
