package handlers

import (
	"context"
	"fmt"
	"time"

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

func (h *ObservabilityHandler) ListTraces(ctx context.Context, orgName string, projectName string, agentName string, environment string, startTime string, endTime string, sortOrder string, limit int, offset int) (map[string]any, error) {
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
		Offset:       offset,
		SortOrder:    sortOrder,
	}

	return h.traceClient.ListTraces(ctx, params)
}

func (h *ObservabilityHandler) ExportTraces(ctx context.Context, orgName string, projectName string, agentName string, environment string, startTime string, endTime string, sortOrder string, limit int, offset int) (map[string]any, error) {
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
		Offset:       offset,
		SortOrder:    sortOrder,
	}

	return h.traceClient.ExportTraces(ctx, params)
}

//need to chnge
func (h *ObservabilityHandler) GetTraceDetails(ctx context.Context, orgName string, projectName string, agentName string, traceID string, environment string) (map[string]any, error) {
	if h.traceClient == nil {
		return nil, fmt.Errorf("trace observer client is not configured")
	}

	now := time.Now().UTC()
	params := traceobserversvc.TraceDetailsParams{
		TraceID:      traceID,
		Organization: orgName,
		Project:      projectName,
		Component:    agentName,
		Environment:  environment,
		StartTime:    now.AddDate(0, 0, -7).Format(time.RFC3339),
		EndTime:      now.Format(time.RFC3339),
		Limit:        1000, // fetch all spans for the trace
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
