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

package controllers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/repositories"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/services"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/spec"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

const (
	// Default limit for pagination
	defaultLimit = 100

	// Default offset for pagination
	defaultOffset = 0
)

// GatewayController defines interface for gateway HTTP handlers
type GatewayController interface {
	RegisterGateway(w http.ResponseWriter, r *http.Request)
	GetGateway(w http.ResponseWriter, r *http.Request)
	ListGateways(w http.ResponseWriter, r *http.Request)
	UpdateGateway(w http.ResponseWriter, r *http.Request)
	DeleteGateway(w http.ResponseWriter, r *http.Request)
	AssignGatewayToEnvironment(w http.ResponseWriter, r *http.Request)
	RemoveGatewayFromEnvironment(w http.ResponseWriter, r *http.Request)
	GetGatewayEnvironments(w http.ResponseWriter, r *http.Request)
	CheckGatewayHealth(w http.ResponseWriter, r *http.Request)
	RotateGatewayToken(w http.ResponseWriter, r *http.Request)
	RevokeGatewayToken(w http.ResponseWriter, r *http.Request)
	GetGatewayStatus(w http.ResponseWriter, r *http.Request)
	GetGatewayArtifacts(w http.ResponseWriter, r *http.Request)
}

type gatewayController struct {
	gatewayService *services.PlatformGatewayService
	orgRepo        repositories.OrganizationRepository
	db             *gorm.DB
}

// NewGatewayController creates a new gateway controller
func NewGatewayController(
	gatewayService *services.PlatformGatewayService,
	orgRepo repositories.OrganizationRepository,
	db *gorm.DB,
) GatewayController {
	return &gatewayController{
		gatewayService: gatewayService,
		orgRepo:        orgRepo,
		db:             db,
	}
}

// resolveOrgUUID resolves organization handle to UUID
func (c *gatewayController) resolveOrgUUID(ctx context.Context, orgName string) (string, error) {
	org, err := c.orgRepo.GetOrganizationByHandle(orgName)
	if err != nil {
		return "", utils.ErrOrganizationNotFound
	}
	return org.UUID.String(), nil
}

func handleGatewayErrors(w http.ResponseWriter, err error, fallbackMsg string) {
	switch {
	case errors.Is(err, utils.ErrGatewayNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway not found")
	case errors.Is(err, utils.ErrGatewayAlreadyExists):
		utils.WriteErrorResponse(w, http.StatusConflict, "Gateway already exists")
	case errors.Is(err, utils.ErrEnvironmentNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Environment not found")
	case errors.Is(err, utils.ErrInvalidInput):
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid input")
	case errors.Is(err, gorm.ErrRecordNotFound):
		utils.WriteErrorResponse(w, http.StatusNotFound, "Resource not found")
	default:
		utils.WriteErrorResponse(w, http.StatusInternalServerError, fallbackMsg)
	}
}

func (c *gatewayController) RegisterGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("RegisterGateway: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	var req spec.CreateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("RegisterGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Create gateway using local service
	description := "" // Description not in spec, use empty string
	functionalityType := string(req.GatewayType)
	isCritical := false
	if req.IsCritical != nil {
		isCritical = *req.IsCritical
	}
	var properties map[string]interface{}

	gateway, err := c.gatewayService.RegisterGateway(
		orgID,
		req.Name,
		req.DisplayName,
		description,
		req.Vhost,
		isCritical,
		functionalityType,
		properties,
	)
	if err != nil {
		log.Error("RegisterGateway: failed to create gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to register gateway")
		return
	}

	// Assign to environments if provided (using gateway_environment_mappings table)
	if len(req.EnvironmentIds) > 0 {
		for _, envID := range req.EnvironmentIds {
			if err := c.assignGatewayToEnvironmentInDB(ctx, orgName, gateway.ID, envID); err != nil {
				log.Warn("RegisterGateway: failed to assign gateway to environment", "envID", envID, "error", err)
				// Continue with other environments
			}
		}
	}

	// Get environments for response
	environments := c.getGatewayEnvironmentsFromDB(ctx, orgName, gateway.ID)

	// Convert to spec response
	response := convertGatewayToSpecResponse(gateway, orgName, environments)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *gatewayController) GetGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("GetGateway: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Get gateway from local service
	gateway, err := c.gatewayService.GetGateway(gatewayID, orgID)
	if err != nil {
		log.Error("GetGateway: failed to get gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway")
		return
	}

	// Get environments from DB
	environments := c.getGatewayEnvironmentsFromDB(ctx, orgName, gatewayID)

	response := convertGatewayToSpecResponse(gateway, orgName, environments)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) ListGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("ListGateways: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", defaultLimit)
	offset := getIntQueryParam(r, "offset", defaultOffset)

	// Get gateways from local service
	gatewaysResp, err := c.gatewayService.ListGateways(&orgID)
	if err != nil {
		log.Error("ListGateways: failed to list gateways", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list gateways")
		return
	}

	// Convert to spec responses
	specGateways := make([]spec.GatewayResponse, 0)
	for _, gw := range gatewaysResp.List {
		// Get environments from DB for each gateway
		environments := c.getGatewayEnvironmentsFromDB(ctx, orgName, gw.ID)
		specGateways = append(specGateways, convertGatewayToSpecResponse(&gw, orgName, environments))
	}

	// Apply pagination (client-side for now)
	total := len(specGateways)
	start := offset
	end := offset + limit
	if start > len(specGateways) {
		start = len(specGateways)
	}
	if end > len(specGateways) {
		end = len(specGateways)
	}
	paginatedGateways := specGateways[start:end]

	response := spec.GatewayListResponse{
		Gateways: paginatedGateways,
		Total:    int32(total),
		Limit:    int32(limit),
		Offset:   int32(offset),
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) UpdateGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("UpdateGateway: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	var req spec.UpdateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Update using local service
	var properties *map[string]interface{}
	var description *string // Description not in spec
	gateway, err := c.gatewayService.UpdateGateway(gatewayID, orgID, description, req.DisplayName, req.IsCritical, properties)
	if err != nil {
		log.Error("UpdateGateway: failed to update gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}

	// Get environments from DB
	environments := c.getGatewayEnvironmentsFromDB(ctx, orgName, gatewayID)

	response := convertGatewayToSpecResponse(gateway, orgName, environments)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("DeleteGateway: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Delete using local service
	if err := c.gatewayService.DeleteGateway(gatewayID, orgID); err != nil {
		log.Error("DeleteGateway: failed to delete gateway", "error", err)
		handleGatewayErrors(w, err, "Failed to delete gateway")
		return
	}

	// Delete environment mappings from DB
	gwUUID, err := uuid.Parse(gatewayID)
	if err == nil {
		if err := db.DB(ctx).Where("gateway_uuid = ?", gwUUID).Delete(&models.GatewayEnvironmentMapping{}).Error; err != nil {
			log.Warn("DeleteGateway: failed to delete gateway-environment mappings", "error", err)
		}
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *gatewayController) AssignGatewayToEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	envID := strings.TrimSpace(r.PathValue("envID"))

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("AssignGatewayToEnvironment: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Verify gateway exists
	if _, err := c.gatewayService.GetGateway(gatewayID, orgID); err != nil {
		log.Error("AssignGatewayToEnvironment: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to assign gateway")
		return
	}

	// Assign in DB
	if err := c.assignGatewayToEnvironmentInDB(ctx, orgName, gatewayID, envID); err != nil {
		log.Error("AssignGatewayToEnvironment: failed to assign", "error", err)
		handleGatewayErrors(w, err, "Failed to assign gateway to environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, map[string]string{"message": "Gateway assigned successfully"})
}

func (c *gatewayController) RemoveGatewayFromEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	envID := strings.TrimSpace(r.PathValue("envID"))

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		log.Error("RemoveGatewayFromEnvironment: invalid gateway ID", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid gateway ID")
		return
	}

	envUUID, err := uuid.Parse(envID)
	if err != nil {
		log.Error("RemoveGatewayFromEnvironment: invalid environment ID", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid environment ID")
		return
	}

	// Delete the mapping from DB
	result := db.DB(ctx).Where("gateway_uuid = ? AND environment_uuid = ?", gwUUID, envUUID).
		Delete(&models.GatewayEnvironmentMapping{})

	if result.Error != nil {
		log.Error("RemoveGatewayFromEnvironment: failed to delete mapping", "error", result.Error)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to remove gateway from environment")
		return
	}

	if result.RowsAffected == 0 {
		utils.WriteErrorResponse(w, http.StatusNotFound, "Gateway-environment mapping not found")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *gatewayController) GetGatewayEnvironments(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// Get environments from DB
	environments := c.getGatewayEnvironmentsFromDB(ctx, orgName, gatewayID)

	// Convert to spec responses
	specEnvs := make([]spec.GatewayEnvironmentResponse, len(environments))
	for i, env := range environments {
		specEnvs[i] = convertDBEnvironmentToSpecResponse(&env)
	}

	response := spec.GetGatewayEnvironments200Response{
		Environments: specEnvs,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) CheckGatewayHealth(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("CheckGatewayHealth: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Get gateway to check if it exists
	gateway, err := c.gatewayService.GetGateway(gatewayID, orgID)
	if err != nil {
		log.Error("CheckGatewayHealth: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to check gateway health")
		return
	}

	// Return health based on gateway's active status
	status := "healthy"
	if !gateway.IsActive {
		status = "unhealthy"
	}

	response := map[string]interface{}{
		"gatewayId": gatewayID,
		"status":    status,
		"checkedAt": gateway.UpdatedAt,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) RotateGatewayToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("RotateGatewayToken: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Call service to rotate the token
	tokenResp, err := c.gatewayService.RotateToken(gatewayID, orgID)
	if err != nil {
		log.Error("RotateGatewayToken: failed to rotate token", "error", err)
		handleGatewayErrors(w, err, "Failed to rotate gateway token")
		return
	}

	// Convert to spec response
	response := spec.GatewayTokenResponse{
		GatewayId: gatewayID,
		Token:     tokenResp.Token,
		TokenId:   tokenResp.ID,
		CreatedAt: tokenResp.CreatedAt,
		ExpiresAt: nil, // Token doesn't have expiry in current implementation
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) RevokeGatewayToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	tokenID := strings.TrimSpace(r.PathValue("tokenID"))

	// Note: This functionality might need to be added to the service
	log.Warn("RevokeGatewayToken: not implemented in local service", "gatewayID", gatewayID, "tokenID", tokenID)
	utils.WriteErrorResponse(w, http.StatusNotImplemented, "Token revocation not yet implemented")
}

func (c *gatewayController) GetGatewayStatus(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("GetGatewayStatus: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse optional gatewayID query parameter
	gatewayIDParam := r.URL.Query().Get("gatewayId")
	var gatewayIDPtr *string
	if gatewayIDParam != "" {
		gatewayIDPtr = &gatewayIDParam
	}

	statusResp, err := c.gatewayService.GetGatewayStatus(orgID, gatewayIDPtr)
	if err != nil {
		log.Error("GetGatewayStatus: failed to get status", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway status")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, statusResp)
}

func (c *gatewayController) GetGatewayArtifacts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	orgID, err := c.resolveOrgUUID(ctx, orgName)
	if err != nil {
		log.Error("GetGatewayArtifacts: organization not found", "error", err)
		utils.WriteErrorResponse(w, http.StatusUnauthorized, "Organization not found")
		return
	}

	// Parse optional artifactType query parameter
	artifactType := r.URL.Query().Get("type")
	if artifactType == "" {
		artifactType = "all"
	}

	artifactsResp, err := c.gatewayService.GetGatewayArtifacts(gatewayID, orgID, artifactType)
	if err != nil {
		log.Error("GetGatewayArtifacts: failed to get artifacts", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway artifacts")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusOK, artifactsResp)
}

// Internal helper methods

// assignGatewayToEnvironmentInDB creates a mapping in the gateway_environment_mappings table
func (c *gatewayController) assignGatewayToEnvironmentInDB(ctx context.Context, orgName, gatewayID, envID string) error {
	log := logger.GetLogger(ctx)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		return fmt.Errorf("invalid gateway UUID: %w", err)
	}

	envUUID, err := uuid.Parse(envID)
	if err != nil {
		return fmt.Errorf("invalid environment UUID: %w", err)
	}

	// Verify environment exists and belongs to organization
	var env models.Environment
	if err := db.DB(ctx).Where("uuid = ? AND organization_name = ?", envUUID, orgName).First(&env).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return utils.ErrEnvironmentNotFound
		}
		return fmt.Errorf("failed to verify environment: %w", err)
	}

	// Check if mapping already exists
	var existing models.GatewayEnvironmentMapping
	err = db.DB(ctx).Where("gateway_uuid = ? AND environment_uuid = ?", gwUUID, envUUID).
		First(&existing).Error

	if err == nil {
		log.Warn("Gateway already assigned to environment", "gatewayID", gatewayID, "envID", envID)
		return nil // Already assigned, treat as success
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check existing mapping: %w", err)
	}

	// Create mapping
	mapping := &models.GatewayEnvironmentMapping{
		GatewayUUID:     gwUUID,
		EnvironmentUUID: envUUID,
	}

	if err := db.DB(ctx).Create(mapping).Error; err != nil {
		return fmt.Errorf("failed to create gateway-environment mapping: %w", err)
	}

	return nil
}

// getGatewayEnvironmentsFromDB retrieves environments associated with a gateway from the DB
func (c *gatewayController) getGatewayEnvironmentsFromDB(ctx context.Context, orgName, gatewayID string) []models.Environment {
	log := logger.GetLogger(ctx)

	gwUUID, err := uuid.Parse(gatewayID)
	if err != nil {
		log.Warn("getGatewayEnvironmentsFromDB: invalid gateway UUID", "error", err)
		return []models.Environment{}
	}

	var environments []models.Environment
	err = db.DB(ctx).
		Joins("JOIN gateway_environment_mappings ON gateway_environment_mappings.environment_uuid = environments.uuid").
		Where("gateway_environment_mappings.gateway_uuid = ? AND environments.organization_name = ?", gwUUID, orgName).
		Find(&environments).Error
	if err != nil {
		log.Warn("getGatewayEnvironmentsFromDB: failed to get environments", "error", err)
		return []models.Environment{}
	}

	return environments
}

// Helper conversion functions

func convertGatewayToSpecResponse(gw *services.GatewayResponse, orgName string, environments []models.Environment) spec.GatewayResponse {
	response := spec.GatewayResponse{
		Uuid:             gw.ID,
		OrganizationName: orgName,
		Name:             gw.Name,
		DisplayName:      gw.DisplayName,
		GatewayType:      spec.GatewayType(gw.FunctionalityType),
		Vhost:            gw.Vhost,
		IsCritical:       gw.IsCritical,
		Status:           convertStatusToGatewayStatus(gw.IsActive),
		CreatedAt:        gw.CreatedAt,
		UpdatedAt:        gw.UpdatedAt,
	}

	// Convert environments
	if len(environments) > 0 {
		envs := make([]spec.GatewayEnvironmentResponse, len(environments))
		for i, env := range environments {
			envs[i] = convertDBEnvironmentToSpecResponse(&env)
		}
		response.Environments = envs
	}

	return response
}

func convertStatusToGatewayStatus(isActive bool) spec.GatewayStatus {
	if isActive {
		return "ACTIVE"
	}
	return "INACTIVE"
}

func convertDBEnvironmentToSpecResponse(env *models.Environment) spec.GatewayEnvironmentResponse {
	return spec.GatewayEnvironmentResponse{
		Id:               env.UUID.String(),
		OrganizationName: env.OrganizationName,
		Name:             env.Name,
		DisplayName:      env.DisplayName,
		DataplaneRef:     env.DataplaneRef,
		DnsPrefix:        env.DNSPrefix,
		IsProduction:     env.IsProduction,
		CreatedAt:        env.CreatedAt,
		UpdatedAt:        env.UpdatedAt,
	}
}
