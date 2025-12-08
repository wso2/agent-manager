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

package repositories

import (
	"context"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

type AgentRepository interface {
	ListAgents(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID) ([]*models.Agent, error)
	GetAgentByName(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string) (*models.Agent, error)
	CreateAgent(ctx context.Context, agent *models.Agent) error
	SoftDeleteAgentByName(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string) error
	HardDeleteAgentByName(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string) error
	UpdateAgentTimestamp(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string) error
}

type agentRepository struct{}

func NewAgentRepository() AgentRepository {
	return &agentRepository{}
}

func (r *agentRepository) ListAgents(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID) ([]*models.Agent, error) {
	var agents []*models.Agent
	if err := db.DB(ctx).Where("org_id = ? AND project_id = ?", orgId, projectId).Find(&agents).Error; err != nil {
		return nil, fmt.Errorf("agentRepository.ListAgents: %w", err)
	}

	return agents, nil
}

func (r *agentRepository) GetAgentByName(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string) (*models.Agent, error) {
	var agent models.Agent
	if err := db.DB(ctx).Where("org_id = ? AND project_id = ? AND name = ?", orgId, projectId, agentName).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("agentRepository.GetAgentByName: %w", err)
	}
	return &agent, nil
}

func (r *agentRepository) CreateAgent(ctx context.Context, agent *models.Agent) error {
	if err := db.DB(ctx).Create(agent).Error; err != nil {
		return fmt.Errorf("agentRepository.CreateAgent: %w", err)
	}
	return nil
}

func (r *agentRepository) SoftDeleteAgentByName(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string) error {
	if err := db.DB(ctx).Where("org_id = ? AND project_id = ? AND name = ?", orgId, projectId, agentName).Delete(&models.Agent{}).Error; err != nil {
		return fmt.Errorf("agentRepository.DeleteAgentByName: %w", err)
	}
	return nil
}

func (r *agentRepository) HardDeleteAgentByName(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string) error {
	if err := db.DB(ctx).Unscoped().Where("org_id = ? AND project_id = ? AND name = ?", orgId, projectId, agentName).Delete(&models.Agent{}).Error; err != nil {
		return fmt.Errorf("agentRepository.HardDeleteAgentByName: %w", err)
	}
	return nil
}

func (r *agentRepository) UpdateAgentTimestamp(ctx context.Context, orgId uuid.UUID, projectId uuid.UUID, agentName string) error {
	if err := db.DB(ctx).Model(&models.Agent{}).
		Where("org_id = ? AND project_id = ? AND name = ?", orgId, projectId, agentName).
		Update("updated_at", gorm.Expr("NOW()")).Error; err != nil {
		return fmt.Errorf("agentRepository.UpdateAgentTimestamp: %w", err)
	}
	return nil
}
