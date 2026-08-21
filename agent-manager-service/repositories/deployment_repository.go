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

package repositories

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// DeploymentRepository defines the interface for deployment data operations
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/deployment_repository_mock.go . DeploymentRepository:DeploymentRepositoryMock
type DeploymentRepository interface {
	// Deployment artifact methods (immutable deployments)
	CreateWithLimitEnforcement(deployment *models.Deployment, hardLimit int) error
	GetWithContent(deploymentID, artifactUUID, orgUUID string) (*models.Deployment, error)
	GetWithState(deploymentID, artifactUUID, orgUUID string) (*models.Deployment, error)
	// GetWithStateCtx is GetWithState with context propagation, for call paths
	// (e.g. UndeployLLMProxyDeployment) that need cancellation to reach the
	// query. GetWithState itself is left as-is to avoid forcing ctx onto its
	// other existing caller.
	GetWithStateCtx(ctx context.Context, deploymentID, artifactUUID, orgUUID string) (*models.Deployment, error)
	GetDeploymentsWithState(artifactUUID, orgUUID string, gatewayID *string, status *string, maxPerAPIGW int) ([]*models.Deployment, error)
	Delete(deploymentID, artifactUUID, orgUUID string) error
	GetCurrentByGateway(artifactUUID, gatewayID, orgUUID string) (*models.Deployment, error)

	// Deployment status methods (mutable state tracking)
	SetCurrent(artifactUUID, orgUUID, gatewayID, deploymentID string, status models.DeploymentStatus) (updatedAt time.Time, err error)
	// SetCurrentCtx is SetCurrent with context propagation — see GetWithStateCtx.
	SetCurrentCtx(ctx context.Context, artifactUUID, orgUUID, gatewayID, deploymentID string, status models.DeploymentStatus) (updatedAt time.Time, err error)
	GetStatus(artifactUUID, orgUUID, gatewayID string) (deploymentID string, status models.DeploymentStatus, updatedAt *time.Time, err error)
	DeleteStatus(artifactUUID, orgUUID, gatewayID string) error

	// Deployment ack methods
	UpdateStatusByDeploymentID(deploymentID, gatewayUUID string, status models.DeploymentStatus) (time.Time, error)

	// Gateway mapping methods (derived from deployment status)
	GetDeployedGatewaysByProvider(artifactUUID uuid.UUID, orgUUID string) ([]string, error)
	GetTrackedGatewaysByProvider(artifactUUID uuid.UUID, orgUUID string) ([]string, error)
	GetDeployedProvidersByGateway(gatewayUUID uuid.UUID, orgUUID string) ([]string, error)
	IsProviderDeployedToGateway(artifactUUID uuid.UUID, gatewayUUID uuid.UUID, orgUUID string) (bool, error)
	GetByArtifactAndGateway(artifactUUID, gatewayUUID, orgUUID string) (*models.Deployment, error)
}

// DeploymentRepo implements DeploymentRepository using GORM
type DeploymentRepo struct {
	db *gorm.DB
}

// NewDeploymentRepo creates a new deployment repository
func NewDeploymentRepo(db *gorm.DB) DeploymentRepository {
	return &DeploymentRepo{
		db: db,
	}
}

// CreateWithLimitEnforcement atomically creates a deployment with hard limit enforcement
func (r *DeploymentRepo) CreateWithLimitEnforcement(deployment *models.Deployment, hardLimit int) error {
	return r.db.Transaction(func(tx *gorm.DB) error {
		// Generate UUID for deployment if not already set
		if deployment.DeploymentID == uuid.Nil {
			deployment.DeploymentID = uuid.New()
		}
		deployment.CreatedAt = time.Now()

		// Status must be provided and should be DEPLOYED for new deployments
		if deployment.Status == nil {
			deployed := models.DeploymentStatusDeployed
			deployment.Status = &deployed
		}

		updatedAt := time.Now()
		deployment.UpdatedAt = &updatedAt

		// Count total deployments for this artifact+Gateway
		var count int64
		if err := tx.Model(&models.Deployment{}).
			Where("artifact_uuid = ? AND gateway_uuid = ? AND ou_id = ?",
				deployment.ArtifactUUID, deployment.GatewayUUID, deployment.OUID).
			Count(&count).Error; err != nil {
			return err
		}

		// If at/over hard limit, delete oldest 5 ARCHIVED deployments
		if count >= int64(hardLimit) {
			// Get oldest 5 ARCHIVED deployment IDs
			var idsToDelete []uuid.UUID
			if err := tx.Table("deployments d").
				Select("d.deployment_id").
				Joins("LEFT JOIN deployment_status s ON d.deployment_id = s.deployment_id "+
					"AND d.artifact_uuid = s.artifact_uuid "+
					"AND d.ou_id = s.ou_id "+
					"AND d.gateway_uuid = s.gateway_uuid").
				Where("d.artifact_uuid = ? AND d.gateway_uuid = ? AND d.ou_id = ? AND s.deployment_id IS NULL",
					deployment.ArtifactUUID, deployment.GatewayUUID, deployment.OUID).
				Order("d.created_at ASC").
				Limit(5).
				Pluck("d.deployment_id", &idsToDelete).Error; err != nil {
				return err
			}

			// Delete one-by-one to use row-level locks
			for _, id := range idsToDelete {
				if err := tx.Where("deployment_id = ?", id).Delete(&models.Deployment{}).Error; err != nil {
					return err
				}
			}
		}

		// Insert new deployment
		if err := tx.Create(deployment).Error; err != nil {
			return err
		}

		// Insert or update deployment status (UPSERT)
		deploymentStatus := &models.DeploymentStatusRecord{
			ArtifactUUID: deployment.ArtifactUUID,
			OUID:         deployment.OUID,
			GatewayUUID:  deployment.GatewayUUID,
			DeploymentID: deployment.DeploymentID,
			Status:       *deployment.Status,
			UpdatedAt:    *deployment.UpdatedAt,
		}

		return tx.Clauses(clause.OnConflict{
			Columns:   []clause.Column{{Name: "artifact_uuid"}, {Name: "ou_id"}, {Name: "gateway_uuid"}},
			DoUpdates: clause.AssignmentColumns([]string{"deployment_id", "status", "updated_at"}),
		}).Create(deploymentStatus).Error
	})
}

// GetWithContent retrieves a deployment including its content
func (r *DeploymentRepo) GetWithContent(deploymentID, artifactUUID, orgUUID string) (*models.Deployment, error) {
	var deployment models.Deployment
	err := r.db.Where("deployment_id = ? AND artifact_uuid = ? AND ou_id = ?",
		deploymentID, artifactUUID, orgUUID).
		First(&deployment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("deployment not found")
		}
		return nil, err
	}
	return &deployment, nil
}

// Delete deletes a deployment record
func (r *DeploymentRepo) Delete(deploymentID, artifactUUID, orgUUID string) error {
	result := r.db.Where("deployment_id = ? AND artifact_uuid = ? AND ou_id = ?",
		deploymentID, artifactUUID, orgUUID).
		Delete(&models.Deployment{})

	if result.Error != nil {
		return result.Error
	}

	if result.RowsAffected == 0 {
		return errors.New("deployment not found")
	}

	return nil
}

// GetCurrentByGateway retrieves the currently DEPLOYED deployment for an artifact on a gateway
func (r *DeploymentRepo) GetCurrentByGateway(artifactUUID, gatewayID, orgUUID string) (*models.Deployment, error) {
	var deployment models.Deployment
	result := r.db.Table("deployments d").
		Select("d.deployment_id, d.name, d.artifact_uuid, d.ou_id, d.gateway_uuid, "+
			"d.base_deployment_id, d.content, d.metadata, d.created_at, "+
			"s.status, s.updated_at AS status_updated_at").
		Joins("INNER JOIN deployment_status s ON d.deployment_id = s.deployment_id "+
			"AND d.artifact_uuid = s.artifact_uuid "+
			"AND d.ou_id = s.ou_id "+
			"AND d.gateway_uuid = s.gateway_uuid").
		Where("d.artifact_uuid = ? AND d.gateway_uuid = ? AND d.ou_id = ? AND s.status = ?",
			artifactUUID, gatewayID, orgUUID, string(models.DeploymentStatusDeployed)).
		Order("d.created_at DESC").
		Limit(1).
		Scan(&deployment)
	if result.Error != nil {
		return nil, result.Error
	}
	if result.RowsAffected == 0 {
		//nolint:nilnil // A nil deployment with nil error means no active deployment; callers handle deployment == nil.
		return nil, nil
	}
	return &deployment, nil
}

// SetCurrent inserts or updates the deployment status record
func (r *DeploymentRepo) SetCurrent(artifactUUID, orgUUID, gatewayID, deploymentID string, status models.DeploymentStatus) (time.Time, error) {
	return r.SetCurrentCtx(context.Background(), artifactUUID, orgUUID, gatewayID, deploymentID, status)
}

// SetCurrentCtx is SetCurrent with context propagation — see the interface doc comment.
func (r *DeploymentRepo) SetCurrentCtx(ctx context.Context, artifactUUID, orgUUID, gatewayID, deploymentID string, status models.DeploymentStatus) (time.Time, error) {
	updatedAt := time.Now()

	artifactUUID_uuid, err := uuid.Parse(artifactUUID)
	if err != nil {
		return time.Time{}, err
	}
	gatewayUUID_uuid, err := uuid.Parse(gatewayID)
	if err != nil {
		return time.Time{}, err
	}
	deploymentUUID_uuid, err := uuid.Parse(deploymentID)
	if err != nil {
		return time.Time{}, err
	}

	deploymentStatus := &models.DeploymentStatusRecord{
		ArtifactUUID: artifactUUID_uuid,
		OUID:         orgUUID,
		GatewayUUID:  gatewayUUID_uuid,
		DeploymentID: deploymentUUID_uuid,
		Status:       status,
		UpdatedAt:    updatedAt,
	}

	err = r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "artifact_uuid"}, {Name: "ou_id"}, {Name: "gateway_uuid"}},
		DoUpdates: clause.AssignmentColumns([]string{"deployment_id", "status", "updated_at"}),
	}).Create(deploymentStatus).Error

	return updatedAt, err
}

// UpdateStatusByDeploymentID updates the status of a deployment_status record identified by deployment_id and gateway_uuid.
// This is used to process deployment ack messages from the gateway.
func (r *DeploymentRepo) UpdateStatusByDeploymentID(deploymentID, gatewayUUID string, status models.DeploymentStatus) (time.Time, error) {
	updatedAt := time.Now()
	result := r.db.Table("deployment_status").
		Where("deployment_id = ? AND gateway_uuid = ?", deploymentID, gatewayUUID).
		Updates(map[string]interface{}{
			"status":     status,
			"updated_at": updatedAt,
		})
	if result.Error != nil {
		return time.Time{}, result.Error
	}
	if result.RowsAffected == 0 {
		//nolint:forbidigo // reached only from the gateway ack handler, which has no request context
		slog.Warn("UpdateStatusByDeploymentID: no matching deployment_status record found", "deployment_id", deploymentID)
		return time.Time{}, fmt.Errorf("deployment_status not found for deploymentID %s", deploymentID)
	}
	return updatedAt, nil
}

// GetStatus retrieves the current deployment status for an artifact on a gateway
func (r *DeploymentRepo) GetStatus(artifactUUID, orgUUID, gatewayID string) (string, models.DeploymentStatus, *time.Time, error) {
	var result struct {
		DeploymentID string
		Status       string
		UpdatedAt    time.Time
	}

	err := r.db.Table("deployment_status").
		Select("deployment_id, status, updated_at").
		Where("artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?",
			artifactUUID, orgUUID, gatewayID).
		Scan(&result).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return "", "", nil, err
		}
		return "", "", nil, err
	}

	return result.DeploymentID, models.DeploymentStatus(result.Status), &result.UpdatedAt, nil
}

// DeleteStatus deletes the status entry for an artifact on a gateway
func (r *DeploymentRepo) DeleteStatus(artifactUUID, orgUUID, gatewayID string) error {
	return r.db.Where("artifact_uuid = ? AND ou_id = ? AND gateway_uuid = ?",
		artifactUUID, orgUUID, gatewayID).
		Delete(&models.DeploymentStatusRecord{}).Error
}

// GetWithState retrieves a deployment with its lifecycle state populated (without content)
func (r *DeploymentRepo) GetWithState(deploymentID, artifactUUID, orgUUID string) (*models.Deployment, error) {
	return r.GetWithStateCtx(context.Background(), deploymentID, artifactUUID, orgUUID)
}

// GetWithStateCtx is GetWithState with context propagation — see the interface doc comment.
func (r *DeploymentRepo) GetWithStateCtx(ctx context.Context, deploymentID, artifactUUID, orgUUID string) (*models.Deployment, error) {
	var deployment models.Deployment
	err := r.db.WithContext(ctx).Table("deployments d").
		Select("d.deployment_id, d.name, d.artifact_uuid, d.ou_id, d.gateway_uuid, "+
			"d.base_deployment_id, d.metadata, d.created_at, "+
			"s.status, s.updated_at AS status_updated_at").
		Joins("LEFT JOIN deployment_status s ON d.deployment_id = s.deployment_id "+
			"AND d.artifact_uuid = s.artifact_uuid "+
			"AND d.ou_id = s.ou_id "+
			"AND d.gateway_uuid = s.gateway_uuid").
		Where("d.deployment_id = ? AND d.artifact_uuid = ? AND d.ou_id = ?",
			deploymentID, artifactUUID, orgUUID).
		Scan(&deployment).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, errors.New("deployment not found")
		}
		return nil, err
	}

	// If status is NULL, it's ARCHIVED
	if deployment.Status == nil {
		archived := models.DeploymentStatusArchived
		deployment.Status = &archived
	}

	return &deployment, nil
}

// GetDeploymentsWithState retrieves deployments with their lifecycle states
func (r *DeploymentRepo) GetDeploymentsWithState(artifactUUID, orgUUID string, gatewayID *string, status *string, maxPerAPIGW int) ([]*models.Deployment, error) {
	// Validation Logic
	if status != nil {
		validStatuses := map[string]bool{
			string(models.DeploymentStatusDeployed):   true,
			string(models.DeploymentStatusUndeployed): true,
			string(models.DeploymentStatusArchived):   true,
		}
		if !validStatuses[*status] {
			return nil, fmt.Errorf("invalid deployment status: %s", *status)
		}
	}

	// Build query with CTE for ranking
	query := `
        WITH AnnotatedDeployments AS (
            SELECT
				d.deployment_id, d.name, d.artifact_uuid, d.ou_id, d.gateway_uuid,
                d.base_deployment_id, d.metadata, d.created_at,
                s.status as current_status,
                s.updated_at as status_updated_at,
                ROW_NUMBER() OVER (
                    PARTITION BY d.gateway_uuid
                    ORDER BY
                        (CASE WHEN s.status IS NOT NULL THEN 0 ELSE 1 END) ASC,
                        d.created_at DESC
                ) as rank_idx
			FROM deployments d
			LEFT JOIN deployment_status s
                ON d.deployment_id = s.deployment_id
                AND d.gateway_uuid = s.gateway_uuid
				AND d.artifact_uuid = s.artifact_uuid
				AND d.ou_id = s.ou_id
			WHERE d.artifact_uuid = ? AND d.ou_id = ?`

	args := []interface{}{artifactUUID, orgUUID}

	if gatewayID != nil {
		query += " AND d.gateway_uuid = ?"
		args = append(args, *gatewayID)
	}

	query += `
        )
        SELECT
			deployment_id, name, artifact_uuid, ou_id, gateway_uuid,
            base_deployment_id, metadata, created_at,
            current_status as status, status_updated_at
        FROM AnnotatedDeployments
        WHERE rank_idx <= ?`

	args = append(args, maxPerAPIGW)

	if status != nil {
		if *status == string(models.DeploymentStatusArchived) {
			query += " AND current_status IS NULL"
		} else {
			query += " AND current_status = ?"
			args = append(args, *status)
		}
	}

	query += " ORDER BY gateway_uuid ASC, rank_idx ASC"

	var deployments []*models.Deployment
	err := r.db.Raw(query, args...).Scan(&deployments).Error

	// Map ARCHIVED status for records with NULL current_status
	for _, d := range deployments {
		if d.Status == nil {
			archived := models.DeploymentStatusArchived
			d.Status = &archived
		}
	}

	return deployments, err
}

// GetDeployedGatewaysByProvider returns all gateway UUIDs where this provider is currently deployed
func (r *DeploymentRepo) GetDeployedGatewaysByProvider(artifactUUID uuid.UUID, orgUUID string) ([]string, error) {
	var gatewayUUIDs []string
	err := r.db.Table("deployment_status").
		Where("artifact_uuid = ? AND ou_id = ? AND status = ?",
			artifactUUID, orgUUID, models.DeploymentStatusDeployed).
		Pluck("gateway_uuid", &gatewayUUIDs).Error
	if err != nil {
		return nil, err
	}

	return gatewayUUIDs, nil
}

// GetTrackedGatewaysByProvider returns all gateway UUIDs where this provider has a
// deployment_status row, regardless of whether the current status is DEPLOYED or
// UNDEPLOYED. Used by save-time redeploy flows that must reach every gateway the proxy
// has ever been pushed to, including ones whose last deploy ack failed.
func (r *DeploymentRepo) GetTrackedGatewaysByProvider(artifactUUID uuid.UUID, orgUUID string) ([]string, error) {
	var gatewayUUIDs []string
	err := r.db.Table("deployment_status").
		Where("artifact_uuid = ? AND ou_id = ?", artifactUUID, orgUUID).
		Pluck("gateway_uuid", &gatewayUUIDs).Error
	if err != nil {
		return nil, err
	}

	return gatewayUUIDs, nil
}

// GetDeployedProvidersByGateway returns all provider UUIDs currently deployed to this gateway
func (r *DeploymentRepo) GetDeployedProvidersByGateway(gatewayUUID uuid.UUID, orgUUID string) ([]string, error) {
	var providerUUIDs []string
	err := r.db.Table("deployment_status").
		Where("gateway_uuid = ? AND ou_id = ? AND status = ?",
			gatewayUUID, orgUUID, models.DeploymentStatusDeployed).
		Pluck("artifact_uuid", &providerUUIDs).Error
	if err != nil {
		return nil, err
	}

	return providerUUIDs, nil
}

// IsProviderDeployedToGateway checks if a provider is currently deployed to a gateway
func (r *DeploymentRepo) IsProviderDeployedToGateway(artifactUUID uuid.UUID, gatewayUUID uuid.UUID, orgUUID string) (bool, error) {
	var count int64
	err := r.db.Table("deployment_status").
		Where("artifact_uuid = ? AND gateway_uuid = ? AND ou_id = ? AND status = ?",
			artifactUUID, gatewayUUID, orgUUID, models.DeploymentStatusDeployed).
		Count(&count).Error

	return count > 0, err
}

// GetByArtifactAndGateway retrieves deployment by artifact and gateway
func (r *DeploymentRepo) GetByArtifactAndGateway(artifactUUID, gatewayUUID, orgUUID string) (*models.Deployment, error) {
	var deploymentID string
	var status models.DeploymentStatus
	var updatedAt *time.Time

	// First get the deployment ID and status from deployment_status table
	err := r.db.Table("deployment_status").
		Select("deployment_id, status, updated_at").
		Where("artifact_uuid = ? AND gateway_uuid = ? AND ou_id = ?",
			artifactUUID, gatewayUUID, orgUUID).
		Row().
		Scan(&deploymentID, &status, &updatedAt)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrArtifactNotFound
		}
		return nil, fmt.Errorf("failed to query deployment status: %w", err)
	}

	// Return a minimal deployment object with status information
	deployment := &models.Deployment{
		Status:    &status,
		UpdatedAt: updatedAt,
	}

	return deployment, nil
}
