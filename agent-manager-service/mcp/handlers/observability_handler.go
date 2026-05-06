package handlers

import (
	"context"
	// "fmt"
	// "time"

	// traceobserversvc "github.com/wso2/agent-manager/agent-manager-service/clients/traceobserversvc"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

// For runtime logs and metrics
type ObservabilityHandler struct {
	agentSvc services.AgentManagerService
	// traceClient traceobserversvc.TraceObserverClient

}

func NewObservabilityHandler(agentSvc services.AgentManagerService) *ObservabilityHandler {
	return &ObservabilityHandler{agentSvc: agentSvc}
}

// func NewObservabilityHandler(agentSvc services.AgentManagerService, traceClient traceobserversvc.TraceObserverClient) *ObservabilityHandler {
// 	return &ObservabilityHandler{agentSvc: agentSvc, traceClient: traceClient}
// }

func (h *ObservabilityHandler) GetRuntimeLogs(ctx context.Context, orgName string, projectName string, agentName string, payload spec.LogFilterRequest) (*models.LogsResponse, error) {
	return h.agentSvc.GetAgentRuntimeLogs(ctx, orgName, projectName, agentName, payload)
}

func (h *ObservabilityHandler) GetMetrics(ctx context.Context, orgName string, projectName string, agentName string, payload spec.MetricsFilterRequest) (*spec.MetricsResponse, error) {
	return h.agentSvc.GetAgentMetrics(ctx, orgName, projectName, agentName, payload)
}