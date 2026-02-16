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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apiplatformclient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/client"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware/logger"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
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
}

type gatewayController struct {
	apiPlatformClient apiplatformclient.APIPlatformClient
	db                *gorm.DB
}

// NewGatewayController creates a new gateway controller
func NewGatewayController(apiPlatformClient apiplatformclient.APIPlatformClient, db *gorm.DB) GatewayController {
	return &gatewayController{
		apiPlatformClient: apiPlatformClient,
		db:                db,
	}
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

	var req spec.CreateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("RegisterGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to API Platform client request
	clientReq := apiplatformclient.CreateGatewayRequest{
		Name:              req.Name,
		DisplayName:       req.DisplayName,
		Vhost:             req.Vhost,
		FunctionalityType: convertSpecGatewayTypeToFunctionalityType(req.GatewayType),
		IsCritical:        req.IsCritical,
	}

	// Create gateway in API Platform
	gateway, err := c.apiPlatformClient.CreateGateway(ctx, clientReq)
	if err != nil {
		log.Error("RegisterGateway: failed to create gateway in API Platform", "error", err)
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
	response := convertAPIPlatformGatewayToSpecResponse(gateway, orgName, environments)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *gatewayController) GetGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// Get gateway from API Platform
	gateway, err := c.apiPlatformClient.GetGateway(ctx, gatewayID)
	if err != nil {
		log.Error("GetGateway: failed to get gateway from API Platform", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway")
		return
	}

	// Get environments from DB
	environments := c.getGatewayEnvironmentsFromDB(ctx, orgName, gatewayID)

	response := convertAPIPlatformGatewayToSpecResponse(gateway, orgName, environments)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) ListGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", defaultLimit)
	offset := getIntQueryParam(r, "offset", defaultOffset)

	// Get gateways from API Platform
	gateways, err := c.apiPlatformClient.ListGateways(ctx)
	if err != nil {
		log.Error("ListGateways: failed to list gateways from API Platform", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list gateways")
		return
	}

	// Convert to spec responses
	specGateways := make([]spec.GatewayResponse, 0)
	for _, gw := range gateways {
		// Get environments from DB for each gateway
		environments := c.getGatewayEnvironmentsFromDB(ctx, orgName, gw.ID)
		specGateways = append(specGateways, convertAPIPlatformGatewayToSpecResponse(gw, orgName, environments))
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

	var req spec.UpdateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Convert spec request to API Platform client request
	clientReq := apiplatformclient.UpdateGatewayRequest{
		DisplayName: req.DisplayName,
		IsCritical:  req.IsCritical,
	}

	// Update in API Platform
	gateway, err := c.apiPlatformClient.UpdateGateway(ctx, gatewayID, clientReq)
	if err != nil {
		log.Error("UpdateGateway: failed to update gateway in API Platform", "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}

	// Get environments from DB
	environments := c.getGatewayEnvironmentsFromDB(ctx, orgName, gatewayID)

	response := convertAPIPlatformGatewayToSpecResponse(gateway, orgName, environments)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// Delete from API Platform
	if err := c.apiPlatformClient.DeleteGateway(ctx, gatewayID); err != nil {
		log.Error("DeleteGateway: failed to delete gateway from API Platform", "error", err)
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

	// Verify gateway exists in API Platform
	if _, err := c.apiPlatformClient.GetGateway(ctx, gatewayID); err != nil {
		log.Error("AssignGatewayToEnvironment: gateway not found in API Platform", "error", err)
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
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// Get gateway from API Platform to check if it exists
	gateway, err := c.apiPlatformClient.GetGateway(ctx, gatewayID)
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

	response := spec.HealthStatusResponse{
		GatewayId: gatewayID,
		Status:    status,
		CheckedAt: time.Now(),
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) RotateGatewayToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))

	// Call API Platform to rotate the token
	tokenResp, err := c.apiPlatformClient.RotateGatewayToken(ctx, gatewayID)
	if err != nil {
		log.Error("RotateGatewayToken: failed to rotate token in API Platform", "error", err)
		handleGatewayErrors(w, err, "Failed to rotate gateway token")
		return
	}

	// Convert to spec response
	response := spec.GatewayTokenResponse{
		GatewayId: tokenResp.GatewayID,
		Token:     tokenResp.Token,
		TokenId:   tokenResp.TokenID,
		CreatedAt: tokenResp.CreatedAt,
		ExpiresAt: tokenResp.ExpiresAt,
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) RevokeGatewayToken(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	gatewayID := strings.TrimSpace(r.PathValue("gatewayID"))
	tokenID := strings.TrimSpace(r.PathValue("tokenID"))

	// Call API Platform to revoke the token
	err := c.apiPlatformClient.RevokeGatewayToken(ctx, gatewayID, tokenID)
	if err != nil {
		log.Error("RevokeGatewayToken: failed to revoke token in API Platform", "error", err)
		handleGatewayErrors(w, err, "Failed to revoke gateway token")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
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
		CreatedAt:       time.Now(),
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

func convertSpecGatewayTypeToFunctionalityType(gatewayType spec.GatewayType) apiplatformclient.FunctionalityType {
	if spec.AI == gatewayType {
		return apiplatformclient.FunctionalityTypeAI
	}
	return apiplatformclient.FunctionalityTypeRegular
}

func convertAPIPlatformGatewayToSpecResponse(gw *apiplatformclient.GatewayResponse, orgName string, environments []models.Environment) spec.GatewayResponse {
	response := spec.GatewayResponse{
		Uuid:             gw.ID,
		OrganizationName: orgName,
		Name:             gw.Name,
		DisplayName:      gw.DisplayName,
		GatewayType:      spec.GatewayType(gw.FunctionalityType),
		Vhost:            gw.Vhost,
		IsCritical:       gw.IsCritical,
		Status:           convertAPIPlatformStatusToGatewayStatus(gw.IsActive),
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

func convertAPIPlatformStatusToGatewayStatus(isActive bool) spec.GatewayStatus {
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
