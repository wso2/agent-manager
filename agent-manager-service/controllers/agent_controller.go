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

package controllers

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/workloadpolicy"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/thundersvc"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/spec"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

type AgentController interface {
	ListAgents(w http.ResponseWriter, r *http.Request)
	ListOrgAgents(w http.ResponseWriter, r *http.Request)
	GetAgent(w http.ResponseWriter, r *http.Request)
	CreateAgent(w http.ResponseWriter, r *http.Request)
	UpdateAgentBasicInfo(w http.ResponseWriter, r *http.Request)
	DeleteAgent(w http.ResponseWriter, r *http.Request)
	GetAgentDeployments(w http.ResponseWriter, r *http.Request)
	GetAgentConfigurations(w http.ResponseWriter, r *http.Request)
	GenerateName(w http.ResponseWriter, r *http.Request)
	UpdateAgentDeploySettings(w http.ResponseWriter, r *http.Request)
	GetAgentIdentity(w http.ResponseWriter, r *http.Request)
	RegenerateAgentIdentitySecret(w http.ResponseWriter, r *http.Request)
	RevokeAgentIdentitySecret(w http.ResponseWriter, r *http.Request)
	ProvisionAgentIdentity(w http.ResponseWriter, r *http.Request)
	RetryAgentIdentityProvisioning(w http.ResponseWriter, r *http.Request)
	GetAgentRoles(w http.ResponseWriter, r *http.Request)
	GetAgentGroups(w http.ResponseWriter, r *http.Request)
}

type agentController struct {
	agentService  services.AgentManagerService
	eventsCleaner services.EventsCleaner
}

// NewAgentController returns a new AgentController instance.
func NewAgentController(
	agentService services.AgentManagerService,
	eventsCleaner services.EventsCleaner,
) AgentController {
	return &agentController{
		agentService:  agentService,
		eventsCleaner: eventsCleaner,
	}
}

// handleCommonErrors checks for common resource errors and writes appropriate responses.
// If no common error matches, writes an internal server error with the provided fallback message.
// entitlementDeniedMessage returns the message the subscription check gave for
// a refused create, or a generic one when it gave none.
func entitlementDeniedMessage(err error) string {
	var denied *utils.EntitlementDeniedError
	if errors.As(err, &denied) && denied.Message != "" {
		return denied.Message
	}
	return "Your subscription does not allow this. Upgrade your subscription to continue."
}

// writeSubscriptionSuspended answers a request refused because the
// organization's subscription is suspended (workloadpolicy.ErrSuspended). Every
// controller uses it, so the message and code stay the same everywhere.
func writeSubscriptionSuspended(w http.ResponseWriter) {
	utils.WriteErrorResponseWithReason(w, http.StatusForbidden,
		"The Agent Manager subscription is suspended", "Renew the subscription to resume", "SUBSCRIPTION_SUSPENDED")
}

// handleCommonErrors writes the HTTP error response for the sentinel errors the agent services
// return, and a 500 with fallbackMsg for anything else.
func handleCommonErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	if writeIfNotImplemented(w, err) {
		return
	}
	switch {
	case errors.Is(err, utils.ErrWorkloadSecretSyncUnconfirmed):
		utils.WriteErrorResponseWithReason(w, http.StatusServiceUnavailable,
			"Secret values were saved, but the rollout stopped because secret sync could not be confirmed.",
			"", utils.ErrCodeServiceUnavailable)
	case errors.Is(err, workloadpolicy.ErrSuspended):
		writeSubscriptionSuspended(w)
	// A ValidationError raised below the controller (e.g. a stored agent kind
	// version that can't be written as a label) is still a client-side problem —
	// report it as a 400 naming the offending value instead of letting it reach
	// the generic 500 fallback.
	//
	// Keep this FIRST: a ValidationError may also match a classification
	// sentinel (utils.NewInvalidInputError sets ErrInvalidInput), and the
	// sentinel cases below relabel the response with a generic bucket message.
	// Moving this case down silently replaces every such error's own
	// user-facing Message with "Invalid input provided".
	case utils.IsValidationError(err) != nil:
		utils.WriteValidationErrorResponse(w, err)

	case errors.Is(err, utils.ErrEntitlementDenied):
		utils.WriteErrorResponseWithReason(w, http.StatusPaymentRequired,
			entitlementDeniedMessage(err), "", utils.ErrCodeEntitlementDenied)

	// Not found errors
	case errors.Is(err, utils.ErrOrganizationNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Organization not found", err.Error(), utils.ErrCodeOrganizationNotFound)
	case errors.Is(err, utils.ErrProjectNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Project not found", err.Error(), utils.ErrCodeProjectNotFound)
	case errors.Is(err, utils.ErrAgentNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Agent not found", err.Error(), utils.ErrCodeAgentNotFound)
	case errors.Is(err, utils.ErrLLMProviderNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"LLM provider not found", err.Error(), utils.ErrCodeProviderNotFound)
	case errors.Is(err, utils.ErrMCPProxyNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"MCP proxy not found", err.Error(), utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrBuildNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Build not found", err.Error(), utils.ErrCodeBuildNotFound)
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Environment not found", err.Error(), utils.ErrCodeEnvironmentNotFound)
	case errors.Is(err, utils.ErrGitSecretNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Git secret not found", err.Error(), utils.ErrCodeGitSecretNotFound)
	case errors.Is(err, utils.ErrAgentKindNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Agent kind not found", err.Error(), utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrKindVersionNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Agent kind version not found", err.Error(), utils.ErrCodeNotFound)
	case errors.Is(err, utils.ErrSourceAgentNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusNotFound,
			"Source agent not found", err.Error(), utils.ErrCodeNotFound)

	// Conflict errors
	case errors.Is(err, utils.ErrAgentAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent already exists", err.Error(), utils.ErrCodeAgentAlreadyExists)
	case errors.Is(err, utils.ErrOrphanedAgentConfigsExist):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Configurations from a deleted agent with this name still exist", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrProjectAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Project already exists", err.Error(), utils.ErrCodeProjectAlreadyExists)
	case errors.Is(err, utils.ErrProjectHasAssociatedAgents):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Project has associated agents", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrDeploymentPipelineAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"A deployment pipeline with this name already exists. Choose a different name.", "", utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrDeploymentPipelineInUse):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Deployment pipeline is referenced by one or more projects", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrSecretPathConflict):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Secret path conflict", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrGitSecretAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Git secret already exists", err.Error(), utils.ErrCodeGitSecretAlreadyExists)
	case errors.Is(err, utils.ErrAgentKindAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent kind already exists", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrKindVersionAlreadyExists):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent kind version already exists", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrKindImageAlreadyPublished):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Build image already published", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrAgentKindHasInstances):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent kind has active instances", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrAgentIsKindSource):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent is the source of an agent kind", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrResourceBeingDeleted):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"A resource with this name is still being deleted", err.Error(), utils.ErrCodeConflict)
	// Generic conflict catch-all: any unclassified conflict (e.g. a raw "already exists"
	//) is a 409, never a 500. Keep this after the specific conflict cases
	// above so they win; err.Error() carries the detail in the response reason.
	case errors.Is(err, utils.ErrConflict):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Conflict", err.Error(), utils.ErrCodeConflict)

	// Bad request errors
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Invalid input provided", err.Error(), utils.ErrCodeValidation)
	case errors.Is(err, utils.ErrImmutableFieldChange):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Cannot modify immutable field", err.Error(), utils.ErrCodeImmutableField)
	case errors.Is(err, utils.ErrBadRequest):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Bad request", err.Error(), utils.ErrCodeBadRequest)
	case errors.Is(err, utils.ErrDeploymentPipelineNotFound):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Deployment pipeline not found", err.Error(), utils.ErrCodeBadRequest)
	case errors.Is(err, utils.ErrGitSecretInvalidType):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Invalid git secret type", err.Error(), utils.ErrCodeGitSecretInvalidType)
	case errors.Is(err, utils.ErrBuildNotComplete):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Build not complete", err.Error(), utils.ErrCodeBadRequest)
	case errors.Is(err, utils.ErrMissingKindConfigValue):
		utils.WriteErrorResponseWithReason(w, http.StatusBadRequest,
			"Missing required configuration value", err.Error(), utils.ErrCodeValidation)
	case errors.Is(err, utils.ErrDeploymentInProgress):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"A deployment is already in progress", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrComponentNotReconcilable):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent cannot be deployed in its current state", err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrPromotionNeedsRedeploy):
		// The console's promote drawer shows only the message, so it carries the steps too.
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Agent must be deployed again before this promotion: deploy the agent again, promote it up to "+
				"the source environment if that is not the first environment, then retry this promotion",
			err.Error(), utils.ErrCodeConflict)
	case errors.Is(err, utils.ErrEnvironmentHasNoGateway):
		utils.WriteErrorResponseWithReason(w, http.StatusConflict,
			"Environment has no ingress gateway", err.Error(), utils.ErrCodeEnvironmentHasNoGateway)

	// Authorization errors
	case errors.Is(err, utils.ErrUnauthorized):
		utils.WriteErrorResponseWithReason(w, http.StatusUnauthorized,
			"Unauthorized", err.Error(), utils.ErrCodeUnauthorized)
	case errors.Is(err, utils.ErrForbidden):
		utils.WriteErrorResponseWithReason(w, http.StatusForbidden,
			"Forbidden", err.Error(), utils.ErrCodeForbidden)

	// Service unavailable
	case errors.Is(err, utils.ErrServiceUnavailable):
		utils.WriteErrorResponseWithReason(w, http.StatusServiceUnavailable,
			"Service temporarily unavailable", err.Error(), utils.ErrCodeServiceUnavailable)
	case errors.Is(err, audit.ErrRecorderUnavailable):
		// A fail-closed service refused to act unrecorded; nothing was changed.
		utils.WriteErrorResponseWithReason(w, http.StatusServiceUnavailable,
			"The audit trail is unavailable, so the change was not made", "", utils.ErrCodeServiceUnavailable)

	default:
		utils.WriteErrorResponseWithReason(w, http.StatusInternalServerError,
			fallbackMsg, "Internal server error", utils.ErrCodeInternalError)
	}
}

func (c *agentController) GetAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	agent, err := c.agentService.GetAgent(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("GetAgent: failed to get agent", "error", err)
		handleCommonErrors(w, err, "Failed to get agent")
		return
	}

	agentResponse := utils.ConvertToAgentResponse(agent)
	utils.WriteSuccessResponse(w, http.StatusOK, agentResponse)
}

func (c *agentController) ListAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)

	// Parse query parameters
	limitStr := r.URL.Query().Get("limit")
	if limitStr == "" {
		limitStr = strconv.Itoa(utils.DefaultLimit)
	}
	offsetStr := r.URL.Query().Get("offset")
	if offsetStr == "" {
		offsetStr = strconv.Itoa(utils.DefaultOffset)
	}

	// Parse and validate pagination parameters
	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit < utils.MinLimit || limit > utils.MaxLimit {
		log.Error("ListAgents: invalid limit parameter", "limit", limitStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid limit parameter: must be between %d and %d", utils.MinLimit, utils.MaxLimit))
		return
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < utils.MinOffset {
		log.Error("ListAgents: invalid offset parameter", "offset", offsetStr)
		utils.WriteErrorResponse(w, http.StatusBadRequest, fmt.Sprintf("Invalid offset parameter: must be %d or greater", utils.MinOffset))
		return
	}

	labelFilter, err := utils.ParseLabelSelectors(r.URL.Query()["label"])
	if err != nil {
		log.Debug("ListAgents: invalid label parameter", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	agents, total, err := c.agentService.ListAgents(ctx, ouID, projName, labelFilter, int32(limit), int32(offset))
	if err != nil {
		log.Error("ListAgents: failed to list agents", "error", err)
		handleCommonErrors(w, err, "Failed to list agents")
		return
	}

	agentResponses := utils.ConvertToAgentListResponse(agents)
	response := &spec.AgentListResponse{
		Agents: agentResponses,
		Total:  total,
		Limit:  int32(limit),
		Offset: int32(offset),
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *agentController) ListOrgAgents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	ouID := middleware.OUIDFromRequest(r)

	agents, err := c.agentService.ListOrgAgents(ctx, ouID)
	if err != nil {
		log.Error("ListOrgAgents: failed to list agents", "error", err)
		handleCommonErrors(w, err, "Failed to list agents")
		return
	}

	response := &spec.AgentSummaryListResponse{
		Agents: utils.ConvertToAgentSummaryListResponse(agents),
	}
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *agentController) CreateAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)

	// Parse and validate request body
	var payload spec.CreateAgentRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("CreateAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Hosted agents need an agent runtime; refuse them before validating the
	// fields only they carry.
	if payload.Provisioning.Type == string(utils.InternalAgent) {
		utils.WriteErrorResponse(w, http.StatusNotImplemented, "Only external agents can be created in this deployment")
		return
	}
	if err := utils.ValidateAgentCreatePayload(payload); err != nil {
		log.Error("CreateAgent: invalid agent payload", "error", err)
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	growthanalytics.SetDimension(ctx, "creation_method", "external")

	err := c.agentService.CreateAgent(ctx, ouID, projName, &payload)
	if err != nil {
		log.Error("CreateAgent: failed to create agent", "error", err)
		handleCommonErrors(w, err, "Failed to create agent")
		return
	}
	agentType := spec.AgentType{}
	if payload.AgentType != nil {
		agentType = *payload.AgentType
	}
	response := &spec.AgentResponse{
		Name:           payload.Name,
		DisplayName:    payload.DisplayName,
		Description:    utils.StrPointerAsStr(payload.Description, ""),
		ProjectName:    projName,
		Provisioning:   payload.Provisioning,
		AgentType:      agentType,
		Configurations: utils.RedactSecretConfigValues(payload.Configurations),
		CreatedAt:      time.Now(),
	}

	utils.WriteSuccessResponse(w, http.StatusAccepted, response)
}

func (c *agentController) UpdateAgentBasicInfo(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	// Parse and validate request body
	var payload spec.UpdateAgentBasicInfoRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("UpdateAgent: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if err := utils.ValidateAgentBasicInfoUpdatePayload(payload); err != nil {
		utils.WriteValidationErrorResponse(w, err)
		return
	}

	agent, err := c.agentService.UpdateAgentBasicInfo(ctx, ouID, projName, agentName, &payload)
	if err != nil {
		log.Error("UpdateAgent: failed to update agent", "error", err)
		handleCommonErrors(w, err, "Failed to update agent")
		return
	}

	agentResponse := utils.ConvertToAgentResponse(agent)
	utils.WriteSuccessResponse(w, http.StatusOK, agentResponse)
}

func (c *agentController) DeleteAgent(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	err := c.agentService.DeleteAgent(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("DeleteAgent: failed to delete agent", "error", err)
		handleCommonErrors(w, err, "Failed to delete agent")
		return
	}
	if c.eventsCleaner != nil {
		c.eventsCleaner.RemoveForAgent(ctx, ouID, projName, agentName)
	}
	utils.WriteSuccessResponse(w, http.StatusNoContent, "")
}

func (c *agentController) UpdateAgentDeploySettings(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload spec.UpdateAgentDeploySettingsRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("UpdateAgentDeploySettings: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	if payload.EnvironmentName == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environmentName is required")
		return
	}
	events.SetEnvironment(ctx, payload.EnvironmentName)

	if err := c.agentService.UpdateAgentDeploySettings(ctx, ouID, projName, agentName, &payload); err != nil {
		log.Error("UpdateAgentDeploySettings: failed to update deploy settings", "error", err)
		handleCommonErrors(w, err, "Failed to update deploy settings")
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (c *agentController) GenerateName(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	// Parse and validate request body
	var payload spec.ResourceNameRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("GenerateName: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	err := utils.ValidateResourceNameRequest(payload)
	if err != nil {
		log.Error("GenerateName: invalid resource name payload", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid resource name payload")
		return
	}

	candidateName, err := c.agentService.GenerateName(ctx, ouID, payload)
	if err != nil {
		log.Error("GenerateAgentName: failed to generate agent name", "error", err)
		handleCommonErrors(w, err, "Failed to check agent name availability")
		return
	}

	response := &spec.ResourceNameResponse{
		Name:         candidateName,
		DisplayName:  payload.DisplayName,
		ResourceType: payload.ResourceType,
	}
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *agentController) GetAgentDeployments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// Extract path parameters
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	deployments, err := c.agentService.GetAgentDeployments(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("GetAgentDeployments: failed to get deployments", "error", err)
		handleCommonErrors(w, err, "Failed to get deployments")
		return
	}

	deploymentResponses := utils.ConvertToDeploymentDetailsResponse(deployments)
	utils.WriteSuccessResponse(w, http.StatusOK, deploymentResponses)
}

// GetAgentIdentity handles GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities
//
// Returns the agent's AgentID binding for every environment in this project's
// deployment pipeline. A safe, side-effect-free read: it never returns or
// destroys a secret; use RegenerateAgentIdentitySecret to obtain a secret for
// an External agent.
//
// An optional ?environment= query parameter filters the result down to that
// one binding — still returned as an array (0 or 1 elements), so the response
// shape stays consistent whether or not the filter is applied.
func (c *agentController) GetAgentIdentity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	environment := r.URL.Query().Get("environment")

	views, err := c.agentService.GetAgentIdentity(ctx, ouID, projName, agentName)
	if err != nil {
		log.Error("GetAgentIdentity: failed to get agent identity", "orgName", orgName, "agentName", agentName, "error", err)
		handleCommonErrors(w, err, "Failed to get agent identity")
		return
	}

	if views == nil {
		views = []models.AgentIdentityEnvironmentView{}
	}
	if environment != "" {
		filtered := make([]models.AgentIdentityEnvironmentView, 0, 1)
		for _, v := range views {
			if v.EnvironmentName == environment {
				filtered = append(filtered, v)
				break
			}
		}
		views = filtered
	}
	utils.WriteSuccessResponse(w, http.StatusOK, views)
}

// RegenerateAgentIdentitySecret handles
// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities
//
// Rotates the AgentID secret for one environment. The target environment is
// passed in the request body (POST parameters live in the body, not the query
// string — unlike the GET/PUT/DELETE identity endpoints, which take
// ?environment= since they have no body). The new secret is included in the
// response for both Internal and External agents.
func (c *agentController) RegenerateAgentIdentitySecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload models.AgentIdentityActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("RegenerateAgentIdentitySecret: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	envID := payload.Environment
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment is required in the request body")
		return
	}

	log.Info("RegenerateAgentIdentitySecret: starting", "orgName", orgName, "agentName", agentName, "envID", envID)

	attempt, ok := beginAuditOrFail(
		w, r, "RegenerateAgentIdentitySecret", "Failed to regenerate agent identity secret", audit.ActionAgentIdentityRegenerateSecret,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projName),
		audit.Environment(envID),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", envID),
	)
	if !ok {
		return
	}

	resp, err := c.agentService.RegenerateAgentIdentitySecret(ctx, ouID, projName, agentName, envID)
	if err != nil {
		attempt.Complete(ctx, err)
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("RegenerateAgentIdentitySecret: identity not yet provisioned", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		log.Error("RegenerateAgentIdentitySecret: failed to regenerate secret", "orgName", orgName, "agentName", agentName, "envID", envID, "error", err)
		handleCommonErrors(w, err, "Failed to regenerate agent identity secret")
		return
	}

	attempt.Complete(ctx, nil)
	log.Info("RegenerateAgentIdentitySecret: secret regenerated successfully", "orgName", orgName, "agentName", agentName, "envID", envID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// RevokeAgentIdentitySecret handles
// DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities?environment={envID}
//
// Invalidates the AgentID secret for one environment. Never returns a usable
// secret — an explicit regenerate afterward is required to restore access.
func (c *agentController) RevokeAgentIdentitySecret(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.URL.Query().Get("environment")
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	log.Info("RevokeAgentIdentitySecret: starting", "orgName", orgName, "agentName", agentName, "envID", envID)

	attempt, ok := beginAuditOrFail(
		w, r, "RevokeAgentIdentitySecret", "Failed to revoke agent identity secret", audit.ActionAgentIdentityRevokeSecret,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projName),
		audit.Environment(envID),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", envID),
	)
	if !ok {
		return
	}

	resp, err := c.agentService.RevokeAgentIdentitySecret(ctx, ouID, projName, agentName, envID)
	if err != nil {
		attempt.Complete(ctx, err)
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("RevokeAgentIdentitySecret: identity not yet provisioned", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		log.Error("RevokeAgentIdentitySecret: failed to revoke secret", "orgName", orgName, "agentName", agentName, "envID", envID, "error", err)
		handleCommonErrors(w, err, "Failed to revoke agent identity secret")
		return
	}

	attempt.Complete(ctx, nil)
	log.Info("RevokeAgentIdentitySecret: secret revoked successfully", "orgName", orgName, "agentName", agentName, "envID", envID)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}

// ProvisionAgentIdentity handles
// PUT /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities?environment={envID}
//
// Provisions an AgentID for an agent in an environment that doesn't have one
// yet — e.g. one created (or added to this project's pipeline) after the agent
// already existed. Idempotent (PUT semantics):
// if a binding already exists, it is left untouched and the current state is
// returned rather than provisioning again.
func (c *agentController) ProvisionAgentIdentity(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.URL.Query().Get("environment")
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	log.Info("ProvisionAgentIdentity: starting", "orgName", orgName, "agentName", agentName, "envID", envID)

	attempt, ok := beginAuditOrFail(
		w, r, "ProvisionAgentIdentity", "Failed to provision agent identity", audit.ActionAgentIdentityProvision,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projName),
		audit.Environment(envID),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", envID),
	)
	if !ok {
		return
	}

	view, alreadyExisted, err := c.agentService.ProvisionAgentIdentity(ctx, ouID, projName, agentName, envID)
	if err != nil {
		attempt.Complete(ctx, err)
		log.Error("ProvisionAgentIdentity: failed to provision agent identity", "orgName", orgName, "agentName", agentName, "envID", envID, "error", err)
		handleCommonErrors(w, err, "Failed to provision agent identity")
		return
	}

	status := http.StatusAccepted
	if alreadyExisted {
		status = http.StatusOK
	}
	// A no-op provision (the binding already existed) issues no new credential,
	// so it is recorded as a read-level notice rather than a credential change.
	if alreadyExisted {
		attempt.Complete(ctx, nil, audit.SeverityOpt(audit.SeverityNotice), audit.Detail("alreadyExisted", true))
	} else {
		attempt.Complete(ctx, nil)
	}
	log.Info("ProvisionAgentIdentity: completed", "orgName", orgName, "agentName", agentName, "envID", envID, "alreadyExisted", alreadyExisted)
	utils.WriteSuccessResponse(w, status, view)
}

// RetryAgentIdentityProvisioning handles
// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/identities/retry
//
// Resets a FAILED AgentID binding back to PENDING and re-attempts
// provisioning, for both Internal and External agents. The target
// environment is passed in the request body, matching
// RegenerateAgentIdentitySecret's convention for POST identity actions.
func (c *agentController) RetryAgentIdentityProvisioning(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)

	var payload models.AgentIdentityActionRequest
	if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
		log.Error("RetryAgentIdentityProvisioning: failed to decode request body", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}
	envID := payload.Environment
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment is required in the request body")
		return
	}

	log.Info("RetryAgentIdentityProvisioning: starting", "orgName", orgName, "agentName", agentName, "envID", envID)

	attempt, ok := beginAuditOrFail(
		w, r, "RetryAgentIdentityProvisioning", "Failed to retry agent identity provisioning", audit.ActionAgentIdentityRetryProvisioning,
		audit.Org(ouID),
		audit.ResourceNamed(audit.ResourceAgentIdentity, agentName, agentName),
		audit.Project(projName),
		audit.Environment(envID),
		audit.Detail("agentName", agentName),
		audit.Detail("environment", envID),
	)
	if !ok {
		return
	}

	view, err := c.agentService.RetryAgentIdentityProvisioning(ctx, ouID, projName, agentName, envID)
	if err != nil {
		attempt.Complete(ctx, err)
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("RetryAgentIdentityProvisioning: identity not yet provisioned", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		if errors.Is(err, utils.ErrAgentIdentityRetryNotAllowed) {
			log.Warn("RetryAgentIdentityProvisioning: binding not in a failed state", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusConflict, "Agent identity binding is not in a failed state and cannot be retried")
			return
		}
		log.Error("RetryAgentIdentityProvisioning: failed to retry provisioning", "orgName", orgName, "agentName", agentName, "envID", envID, "error", err)
		handleCommonErrors(w, err, "Failed to retry agent identity provisioning")
		return
	}

	attempt.Complete(ctx, nil)
	log.Info("RetryAgentIdentityProvisioning: retry scheduled", "orgName", orgName, "agentName", agentName, "envID", envID)
	utils.WriteSuccessResponse(w, http.StatusAccepted, view)
}

// GetAgentRoles handles
// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/roles?environment={envID}
//
// Returns the Thunder roles assigned to the agent's AgentID in one
// environment. An agent's AgentID (and its role assignments) is per
// environment, so `environment` is required — there is no single answer
// across every environment the agent is deployed to.
func (c *agentController) GetAgentRoles(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.URL.Query().Get("environment")
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	log.Info("GetAgentRoles: starting", "orgName", orgName, "agentName", agentName, "envID", envID)

	roles, err := c.agentService.GetAgentRoles(ctx, ouID, projName, agentName, envID)
	if err != nil {
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("GetAgentRoles: identity not yet provisioned", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		if errors.Is(err, thundersvc.ErrThunderNotProvisioned) {
			// The environment has no identity provider yet, so the agent holds no roles there.
			log.Warn("GetAgentRoles: the environment has no identity provider", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "The identity provider is not set up for this environment yet")
			return
		}
		if errors.Is(err, services.ErrIdentityRoleUnavailable) {
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "Identity management is not configured for this deployment")
			return
		}
		log.Error("GetAgentRoles: failed to get agent roles", "orgName", orgName, "agentName", agentName, "envID", envID, "error", err)
		handleCommonErrors(w, err, "Failed to get agent roles")
		return
	}

	log.Info("GetAgentRoles: completed", "orgName", orgName, "agentName", agentName, "envID", envID)
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"roles": roles})
}

// GetAgentGroups handles
// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/groups?environment={envID}
//
// Returns the Thunder groups the agent's AgentID belongs to in one
// environment. An agent's AgentID (and its group memberships) is per
// environment, so `environment` is required — there is no single answer
// across every environment the agent is deployed to.
func (c *agentController) GetAgentGroups(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	orgName := middleware.OrgHandleFromRequest(r)
	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	envID := r.URL.Query().Get("environment")
	if envID == "" {
		utils.WriteErrorResponse(w, http.StatusBadRequest, "environment query parameter is required")
		return
	}

	log.Info("GetAgentGroups: starting", "orgName", orgName, "agentName", agentName, "envID", envID)

	groups, err := c.agentService.GetAgentGroups(ctx, ouID, projName, agentName, envID)
	if err != nil {
		if errors.Is(err, utils.ErrAgentIdentityNotProvisioned) {
			log.Warn("GetAgentGroups: identity not yet provisioned", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "Agent identity not yet provisioned for this environment")
			return
		}
		if errors.Is(err, thundersvc.ErrThunderNotProvisioned) {
			// The environment has no identity provider yet, so the agent holds no groups there.
			log.Warn("GetAgentGroups: the environment has no identity provider", "orgName", orgName, "agentName", agentName, "envID", envID)
			utils.WriteErrorResponse(w, http.StatusNotFound, "The identity provider is not set up for this environment yet")
			return
		}
		if errors.Is(err, services.ErrIdentityGroupUnavailable) {
			utils.WriteErrorResponse(w, http.StatusServiceUnavailable, "Identity management is not configured for this deployment")
			return
		}
		log.Error("GetAgentGroups: failed to get agent groups", "orgName", orgName, "agentName", agentName, "envID", envID, "error", err)
		handleCommonErrors(w, err, "Failed to get agent groups")
		return
	}

	log.Info("GetAgentGroups: completed", "orgName", orgName, "agentName", agentName, "envID", envID)
	utils.WriteSuccessResponse(w, http.StatusOK, map[string]any{"groups": groups})
}

// GetAgentConfigurations returns the agent's gateway settings (auto-instrumentation,
// CORS, endpoint authentication) for the environment. External agents have no env
// vars or file mounts the platform manages, so those lists are always empty.
func (c *agentController) GetAgentConfigurations(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	ouID := middleware.OUIDFromRequest(r)
	projName := r.PathValue(utils.PathParamProjName)
	agentName := r.PathValue(utils.PathParamAgentName)
	environment := r.URL.Query().Get("environment")
	if environment == "" {
		environment = models.DefaultEnvironmentName
	}

	if _, err := c.agentService.GetAgent(ctx, ouID, projName, agentName); err != nil {
		log.Error("GetAgentConfigurations: failed to get agent", "error", err)
		handleCommonErrors(w, err, "Failed to get configurations")
		return
	}
	envCfg, err := c.agentService.GetAgentEnvConfig(ctx, ouID, projName, agentName, environment)
	if err != nil {
		log.Error("GetAgentConfigurations: failed to read agent config", "error", err)
		handleCommonErrors(w, err, "Failed to get configurations")
		return
	}
	resp := spec.ConfigurationResponse{
		ProjectName: projName,
		AgentName:   agentName,
		Environment: environment,
		Configurations: spec.ConfigurationResponseConfigurations{
			Env:   []spec.ConfigurationItem{},
			Files: []spec.FileMount{},
		},
	}
	utils.PopulateConfigurationResponseFromAgentConfig(&resp, envCfg)
	utils.WriteSuccessResponse(w, http.StatusOK, resp)
}
