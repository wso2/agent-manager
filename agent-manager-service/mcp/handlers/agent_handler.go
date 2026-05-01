package handlers

import (
	"context"
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
)

type AgentHandler struct {
	agentSvc services.AgentManagerService
	tokenSvc services.AgentTokenManagerService
}

func NewAgentHandler(agentSvc services.AgentManagerService, tokenSvc services.AgentTokenManagerService) *AgentHandler {
	return &AgentHandler{agentSvc: agentSvc, tokenSvc: tokenSvc}
}

func (h *AgentHandler) ListAgents(ctx context.Context, orgName string, projName string, limit int32, offset int32) ([]*models.AgentResponse, int32, error) {
	return h.agentSvc.ListAgents(ctx, orgName, projName, limit, offset)
}

func (h *AgentHandler) GenerateName(ctx context.Context, orgName string, payload spec.ResourceNameRequest) (string, error) {
	return h.agentSvc.GenerateName(ctx, orgName, payload)
}

func (h *AgentHandler) CreateAgent(ctx context.Context, orgName string, projectName string, req *spec.CreateAgentRequest) error {
	return h.agentSvc.CreateAgent(ctx, orgName, projectName, req)
}

func (h *AgentHandler) GetAgent(ctx context.Context, orgName string, projectName string, agentName string) (*models.AgentResponse, error) {
	return h.agentSvc.GetAgent(ctx, orgName, projectName, agentName)
}

func (h *AgentHandler) GenerateToken(ctx context.Context, orgName string, projectName string, agentName string, environment string, expiresIn string) (*spec.TokenResponse, error) {
	if h.tokenSvc == nil {
		return nil, fmt.Errorf("token service not configured")
	}

	callerClaims := jwtassertion.GetTokenClaims(ctx)
	if callerClaims == nil || callerClaims.OuId == "" {
		return nil, fmt.Errorf("organization identity missing from caller token")
	}

	req := services.GenerateTokenRequest{
		OrgName:     orgName,
		ProjectName: projectName,
		AgentName:   agentName,
		Environment: environment,
		ExpiresIn:   expiresIn,
		OrgId:       callerClaims.OuId,
	}
	return h.tokenSvc.GenerateToken(ctx, req)
}
