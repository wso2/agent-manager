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
	"errors"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/models"
)

// AgentConfigRepository defines the interface for agent configuration operations
type AgentConfigRepository interface {
	// Upsert creates or updates an agent config for a specific environment
	Upsert(config *models.AgentConfig) error

	// Get retrieves agent config for a specific agent and environment
	Get(orgName, agentName, environmentName string) (*models.AgentConfig, error)

	// DeleteAllByAgent removes all configs for an agent (used when agent is deleted)
	DeleteAllByAgent(orgName, projectName, agentName string) error
}

// AgentConfigRepo implements AgentConfigRepository using GORM
type AgentConfigRepo struct {
	db *gorm.DB
}

// NewAgentConfigRepo creates a new agent config repository
func NewAgentConfigRepo(db *gorm.DB) AgentConfigRepository {
	return &AgentConfigRepo{db: db}
}

// Upsert creates or updates an agent config for a specific environment
func (r *AgentConfigRepo) Upsert(config *models.AgentConfig) error {
	// Use Select("*") to force GORM to include all fields including boolean false values
	// Without this, GORM skips "zero value" fields like false booleans during Create
	return r.db.Select("*").Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "org_name"}, {Name: "agent_name"}, {Name: "environment_name"}},
		DoUpdates: clause.Assignments(map[string]interface{}{
			"enable_auto_instrumentation": config.EnableAutoInstrumentation,
			"updated_at":                  clause.Expr{SQL: "NOW()"},
		}),
	}).Create(config).Error
}

// Get retrieves agent config for a specific agent and environment
func (r *AgentConfigRepo) Get(orgName, agentName, environmentName string) (*models.AgentConfig, error) {
	var config models.AgentConfig
	err := r.db.Where("org_name = ? AND agent_name = ? AND environment_name = ?",
		orgName, agentName, environmentName).First(&config).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, nil // Return nil for not found (allows fallback to default)
		}
		return nil, err
	}
	return &config, nil
}

// DeleteAllByAgent removes all configs for an agent (used when agent is deleted)
func (r *AgentConfigRepo) DeleteAllByAgent(orgName, projectName, agentName string) error {
	return r.db.Where("org_name = ? AND project_name = ? AND agent_name = ?",
		orgName, projectName, agentName).Delete(&models.AgentConfig{}).Error
}
