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

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

type InternalAgentRepository interface {
	GetAgentById(ctx context.Context, agentId uuid.UUID) (*models.InternalAgent, error)
	CreateInternalAgent(ctx context.Context, agent *models.InternalAgent) error
}

type internalAgentRepository struct{}

func NewInternalAgentRepository() InternalAgentRepository {
	return &internalAgentRepository{}
}

func (r *internalAgentRepository) GetAgentById(ctx context.Context, agentId uuid.UUID) (*models.InternalAgent, error) {
	var agent models.InternalAgent
	if err := db.DB(ctx).Where("id = ?", agentId).First(&agent).Error; err != nil {
		return nil, fmt.Errorf("internalAgentRepository.GetAgentById: %w", err)
	}
	return &agent, nil
}

func (r *internalAgentRepository) CreateInternalAgent(ctx context.Context, agent *models.InternalAgent) error {
	if err := db.DB(ctx).Create(agent).Error; err != nil {
		return fmt.Errorf("internalAgentRepository.CreateAgent: %w", err)
	}
	return nil
}
