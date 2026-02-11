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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	apiplatformclient "github.com/wso2/ai-agent-management-platform/agent-manager-service/clients/apiplatformsvc/client"
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
		Properties:        &req.AdapterConfig,
	}

	// Create gateway in API Platform
	gateway, err := c.apiPlatformClient.CreateGateway(ctx, clientReq)
	if err != nil {
		log.Error("RegisterGateway: failed to create gateway in API Platform", "error", err)
		handleGatewayErrors(w, err, "Failed to register gateway")
		return
	}

	// Store gateway metadata in local database
	dbGateway := &models.Gateway{
		UUID:             uuid.MustParse(gateway.ID),
		OrganizationName: orgName,
		Name:             gateway.Name,
		DisplayName:      gateway.DisplayName,
		GatewayType:      string(req.GatewayType),
		VHost:            gateway.Vhost,
		IsCritical:       gateway.IsCritical,
		Status:           convertAPIPlatformStatusToGatewayStatus(gateway.IsActive),
		AdapterConfig:    gateway.Properties,
		CreatedAt:        gateway.CreatedAt,
		UpdatedAt:        gateway.UpdatedAt,
	}

	if req.Region != nil {
		dbGateway.Region = *req.Region
	}

	if err := c.db.Create(dbGateway).Error; err != nil {
		log.Error("RegisterGateway: failed to store gateway in database", "error", err)
		// Try to rollback - delete from API Platform
		if delErr := c.apiPlatformClient.DeleteGateway(ctx, gateway.ID); delErr != nil {
			log.Error("RegisterGateway: failed to rollback gateway creation", "error", delErr)
		}
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to store gateway metadata")
		return
	}

	// Assign to environments if provided
	if len(req.EnvironmentIds) > 0 {
		for _, envID := range req.EnvironmentIds {
			if err := c.assignGatewayToEnvironmentInternal(ctx, gateway.ID, envID); err != nil {
				log.Warn("RegisterGateway: failed to assign gateway to environment", "envID", envID, "error", err)
				// Continue with other environments
			}
		}

		// Reload with environments
		if err := c.db.Preload("Environments").First(dbGateway, "uuid = ?", dbGateway.UUID).Error; err != nil {
			log.Warn("RegisterGateway: failed to reload gateway with environments", "error", err)
		}
	}

	response := convertDBGatewayToSpecResponse(dbGateway, orgName)
	utils.WriteSuccessResponse(w, http.StatusCreated, response)
}

func (c *gatewayController) GetGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	// Get from database with environments
	var dbGateway models.Gateway
	if err := c.db.Preload("Environments").First(&dbGateway, "uuid = ? AND organization_name = ?", gatewayID, orgName).Error; err != nil {
		log.Error("GetGateway: gateway not found in database", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway")
		return
	}

	// Optionally sync with API Platform to get latest status
	if c.apiPlatformClient != nil {
		if platformGateway, err := c.apiPlatformClient.GetGateway(ctx, gatewayID); err == nil {
			// Update status from API Platform
			dbGateway.Status = convertAPIPlatformStatusToGatewayStatus(platformGateway.IsActive)
			dbGateway.UpdatedAt = platformGateway.UpdatedAt
			// Save updated status
			c.db.Model(&dbGateway).Updates(map[string]interface{}{
				"status":     dbGateway.Status,
				"updated_at": dbGateway.UpdatedAt,
			})
		}
	}

	response := convertDBGatewayToSpecResponse(&dbGateway, orgName)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) ListGateways(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)

	// Parse pagination parameters
	limit := getIntQueryParam(r, "limit", defaultLimit)
	offset := getIntQueryParam(r, "offset", defaultOffset)

	// Build query
	query := c.db.Model(&models.Gateway{}).Where("organization_name = ?", orgName)

	// Apply filters
	if gatewayType := r.URL.Query().Get("type"); gatewayType != "" {
		query = query.Where("gateway_type = ?", gatewayType)
	}
	if status := r.URL.Query().Get("status"); status != "" {
		query = query.Where("status = ?", status)
	}

	// Get total count
	var total int64
	if err := query.Count(&total).Error; err != nil {
		log.Error("ListGateways: failed to count gateways", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list gateways")
		return
	}

	// Get gateways with pagination
	var dbGateways []models.Gateway
	if err := query.Preload("Environments").Limit(limit).Offset(offset).Order("created_at DESC").Find(&dbGateways).Error; err != nil {
		log.Error("ListGateways: failed to fetch gateways", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to list gateways")
		return
	}

	// Convert to spec responses
	specGateways := make([]spec.GatewayResponse, len(dbGateways))
	for i, gw := range dbGateways {
		specGateways[i] = convertDBGatewayToSpecResponse(&gw, orgName)
	}

	response := spec.GatewayListResponse{
		Gateways: specGateways,
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
	gatewayID := r.PathValue("gatewayID")

	var req spec.UpdateGatewayRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		log.Error("UpdateGateway: failed to decode request", "error", err)
		utils.WriteErrorResponse(w, http.StatusBadRequest, "Invalid request body")
		return
	}

	// Check if gateway exists in database
	var dbGateway models.Gateway
	if err := c.db.First(&dbGateway, "uuid = ? AND organization_name = ?", gatewayID, orgName).Error; err != nil {
		log.Error("UpdateGateway: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to update gateway")
		return
	}

	// Convert spec request to API Platform client request
	clientReq := apiplatformclient.UpdateGatewayRequest{
		DisplayName: req.DisplayName,
		IsCritical:  req.IsCritical,
		Properties:  &req.AdapterConfig,
	}

	// Update in API Platform
	if c.apiPlatformClient != nil {
		platformGateway, err := c.apiPlatformClient.UpdateGateway(ctx, gatewayID, clientReq)
		if err != nil {
			log.Error("UpdateGateway: failed to update gateway in API Platform", "error", err)
			handleGatewayErrors(w, err, "Failed to update gateway")
			return
		}

		// Update from API Platform response
		updates := map[string]interface{}{
			"updated_at": platformGateway.UpdatedAt,
		}
		if req.DisplayName != nil {
			updates["display_name"] = platformGateway.DisplayName
		}
		if req.IsCritical != nil {
			updates["is_critical"] = platformGateway.IsCritical
		}
		if len(req.AdapterConfig) > 0 {
			updates["adapter_config"] = platformGateway.Properties
		}

		if err := c.db.Model(&dbGateway).Updates(updates).Error; err != nil {
			log.Error("UpdateGateway: failed to update gateway in database", "error", err)
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to update gateway metadata")
			return
		}
	}

	// Reload gateway
	if err := c.db.Preload("Environments").First(&dbGateway, "uuid = ?", gatewayID).Error; err != nil {
		log.Error("UpdateGateway: failed to reload gateway", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to retrieve updated gateway")
		return
	}

	response := convertDBGatewayToSpecResponse(&dbGateway, orgName)
	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

func (c *gatewayController) DeleteGateway(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	// Check if gateway exists
	var dbGateway models.Gateway
	if err := c.db.First(&dbGateway, "uuid = ? AND organization_name = ?", gatewayID, orgName).Error; err != nil {
		log.Error("DeleteGateway: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to delete gateway")
		return
	}

	// Delete from API Platform first
	if c.apiPlatformClient != nil {
		if err := c.apiPlatformClient.DeleteGateway(ctx, gatewayID); err != nil {
			log.Error("DeleteGateway: failed to delete gateway from API Platform", "error", err)
			handleGatewayErrors(w, err, "Failed to delete gateway")
			return
		}
	}

	// Delete environment mappings
	if err := c.db.Where("gateway_uuid = ?", gatewayID).Delete(&models.GatewayEnvironmentMapping{}).Error; err != nil {
		log.Warn("DeleteGateway: failed to delete gateway-environment mappings", "error", err)
	}

	// Delete from database (soft delete)
	if err := c.db.Delete(&dbGateway).Error; err != nil {
		log.Error("DeleteGateway: failed to delete gateway from database", "error", err)
		utils.WriteErrorResponse(w, http.StatusInternalServerError, "Failed to delete gateway")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusNoContent, struct{}{})
}

func (c *gatewayController) AssignGatewayToEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")
	envID := r.PathValue("envID")

	// Verify gateway exists
	var dbGateway models.Gateway
	if err := c.db.First(&dbGateway, "uuid = ? AND organization_name = ?", gatewayID, orgName).Error; err != nil {
		log.Error("AssignGatewayToEnvironment: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to assign gateway")
		return
	}

	if err := c.assignGatewayToEnvironmentInternal(ctx, gatewayID, envID); err != nil {
		log.Error("AssignGatewayToEnvironment: failed to assign", "error", err)
		handleGatewayErrors(w, err, "Failed to assign gateway to environment")
		return
	}

	utils.WriteSuccessResponse(w, http.StatusCreated, map[string]string{"message": "Gateway assigned successfully"})
}

func (c *gatewayController) RemoveGatewayFromEnvironment(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")
	envID := r.PathValue("envID")

	// Verify gateway exists
	var dbGateway models.Gateway
	if err := c.db.First(&dbGateway, "uuid = ? AND organization_name = ?", gatewayID, orgName).Error; err != nil {
		log.Error("RemoveGatewayFromEnvironment: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to remove gateway")
		return
	}

	// Delete the mapping
	result := c.db.Where("gateway_uuid = ? AND environment_uuid = ?", gatewayID, envID).
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
	log := logger.GetLogger(ctx)
	orgName := r.PathValue(utils.PathParamOrgName)
	gatewayID := r.PathValue("gatewayID")

	// Get gateway with environments
	var dbGateway models.Gateway
	if err := c.db.Preload("Environments").First(&dbGateway, "uuid = ? AND organization_name = ?", gatewayID, orgName).Error; err != nil {
		log.Error("GetGatewayEnvironments: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway environments")
		return
	}

	// Convert to spec responses
	specEnvs := make([]spec.GatewayEnvironmentResponse, len(dbGateway.Environments))
	for i, env := range dbGateway.Environments {
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
	gatewayID := r.PathValue("gatewayID")

	// Verify gateway exists
	var dbGateway models.Gateway
	if err := c.db.First(&dbGateway, "uuid = ? AND organization_name = ?", gatewayID, orgName).Error; err != nil {
		log.Error("CheckGatewayHealth: gateway not found", "error", err)
		handleGatewayErrors(w, err, "Failed to check gateway health")
		return
	}

	// For now, return basic health based on gateway status
	// In future, this could check actual gateway connectivity
	status := "healthy"
	if dbGateway.Status != string(models.GatewayStatusActive) {
		status = "unhealthy"
	}

	response := spec.HealthStatusResponse{
		GatewayId: gatewayID,
		Status:    status,
		CheckedAt: time.Now(),
	}

	utils.WriteSuccessResponse(w, http.StatusOK, response)
}

// Internal helper methods

func (c *gatewayController) assignGatewayToEnvironmentInternal(ctx context.Context, gatewayID, envID string) error {
	log := logger.GetLogger(ctx)

	// Verify environment exists
	var env models.Environment
	if err := c.db.First(&env, "uuid = ?", envID).Error; err != nil {
		return fmt.Errorf("environment not found: %w", err)
	}

	// Check if mapping already exists
	var existing models.GatewayEnvironmentMapping
	err := c.db.Where("gateway_uuid = ? AND environment_uuid = ?", gatewayID, envID).
		First(&existing).Error

	if err == nil {
		log.Warn("Gateway already assigned to environment", "gatewayID", gatewayID, "envID", envID)
		return nil // Already assigned, treat as success
	}

	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check existing mapping: %w", err)
	}

	// Create mapping
	mapping := &models.GatewayEnvironmentMapping{
		GatewayUUID:     uuid.MustParse(gatewayID),
		EnvironmentUUID: uuid.MustParse(envID),
		CreatedAt:       time.Now(),
	}

	if err := c.db.Create(mapping).Error; err != nil {
		return fmt.Errorf("failed to create gateway-environment mapping: %w", err)
	}

	return nil
}

// Helper conversion functions

func convertSpecGatewayTypeToFunctionalityType(gatewayType spec.GatewayType) apiplatformclient.FunctionalityType {
	// Map spec.GatewayType to API Platform FunctionalityType
	// For now, default to Regular
	return apiplatformclient.FunctionalityTypeRegular
}

func convertAPIPlatformStatusToGatewayStatus(isActive bool) string {
	if isActive {
		return string(models.GatewayStatusActive)
	}
	return string(models.GatewayStatusInactive)
}

func convertDBGatewayToSpecResponse(gw *models.Gateway, orgName string) spec.GatewayResponse {
	response := spec.GatewayResponse{
		Uuid:             gw.UUID.String(),
		OrganizationName: orgName,
		Name:             gw.Name,
		DisplayName:      gw.DisplayName,
		GatewayType:      spec.GatewayType(gw.GatewayType),
		Vhost:            gw.VHost,
		IsCritical:       gw.IsCritical,
		Status:           spec.GatewayStatus(gw.Status),
		CreatedAt:        gw.CreatedAt,
		UpdatedAt:        gw.UpdatedAt,
	}

	if gw.Region != "" {
		response.Region = &gw.Region
	}
	if gw.ControlPlaneURL != "" {
		response.ControlPlaneUrl = &gw.ControlPlaneURL
	}
	if len(gw.AdapterConfig) > 0 {
		response.AdapterConfig = gw.AdapterConfig
	}

	// Convert environments
	if len(gw.Environments) > 0 {
		envs := make([]spec.GatewayEnvironmentResponse, len(gw.Environments))
		for i, env := range gw.Environments {
			envs[i] = convertDBEnvironmentToSpecResponse(&env)
		}
		response.Environments = envs
	}

	return response
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

// getIntQueryParam is already defined in environment_controller.go, removed duplicate
