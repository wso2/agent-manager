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

package models

import (
	"errors"
	"fmt"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Services address agent-owned rows by OU and by project and agent names, and tables store the agent's id. These
// lookups let a model's hooks translate between the two, so callers keep setting and reading names.

// ErrAgentRefNotFound is an agent named by a row that doesn't exist.
var ErrAgentRefNotFound = errors.New("agent not found")

// fresh is a clean session on tx's connection, for a lookup inside a hook.
func fresh(tx *gorm.DB) *gorm.DB {
	return tx.Session(&gorm.Session{NewDB: true, SkipHooks: true})
}

// AgentIDQuery selects the id of the agent named agentName in project projectName of ouID, for a WHERE clause:
// Where("agent_id = (?)", AgentIDQuery(db, ...)).
func AgentIDQuery(db *gorm.DB, ouID, projectName, agentName string) *gorm.DB {
	return fresh(db).Table("agents a").Select("a.id").
		Joins("JOIN projects p ON p.org_id = a.org_id AND p.id = a.project_id").
		Where("a.org_id = ? AND a.name = ? AND p.name = ?", ouID, agentName, projectName)
}

// AgentIDByNames is the id of an agent named by OU, project and agent name.
func AgentIDByNames(db *gorm.DB, ouID, projectName, agentName string) (uuid.UUID, error) {
	var ids []uuid.UUID
	if err := AgentIDQuery(db, ouID, projectName, agentName).Pluck("a.id", &ids).Error; err != nil {
		return uuid.Nil, err
	}
	if len(ids) == 0 {
		return uuid.Nil, fmt.Errorf("%w: %s/%s", ErrAgentRefNotFound, projectName, agentName)
	}
	return ids[0], nil
}

// AgentNamesByID are the project and agent names of an agent.
func AgentNamesByID(db *gorm.DB, agentID uuid.UUID) (projectName, agentName string, err error) {
	var row struct {
		ProjectName string
		AgentName   string
	}
	err = fresh(db).Table("agents a").Select("p.name AS project_name, a.name AS agent_name").
		Joins("JOIN projects p ON p.org_id = a.org_id AND p.id = a.project_id").
		Where("a.id = ?", agentID).Take(&row).Error
	return row.ProjectName, row.AgentName, err
}

// orgIDOf reads the org_id of a row, for children that copy their parent's organization.
func orgIDOf(db *gorm.DB, table string, id uuid.UUID) (string, error) {
	var row struct{ OrgID string }
	if err := fresh(db).Table(table).Select("org_id").Where("id = ?", id).Take(&row).Error; err != nil {
		return "", fmt.Errorf("reading %s %s: %w", table, id, err)
	}
	return row.OrgID, nil
}

// EnvironmentIDQuery selects the id of the environment named envName in ouID, for a WHERE clause.
func EnvironmentIDQuery(db *gorm.DB, ouID, envName string) *gorm.DB {
	return fresh(db).Table("environments").Select("id").Where("org_id = ? AND name = ?", ouID, envName)
}

// EnvironmentNameByID is the name of an environment.
func EnvironmentNameByID(db *gorm.DB, envID uuid.UUID) (string, error) {
	var row struct{ Name string }
	err := fresh(db).Table("environments").Select("name").Where("id = ?", envID).Take(&row).Error
	return row.Name, err
}

// agentRef is how a per-environment row's hooks move between the stored agent id and the names services use.
type agentRef struct {
	orgID, projectName, agentName *string
	agentID                       **uuid.UUID
	environmentID                 uuid.UUID
	environmentName               *string
}

// resolve sets the stored agent id from the names when it is unset.
func (r agentRef) resolve(tx *gorm.DB) error {
	if *r.agentID != nil && **r.agentID != uuid.Nil {
		return nil
	}
	id, err := AgentIDByNames(tx, *r.orgID, *r.projectName, *r.agentName)
	if err != nil {
		return err
	}
	*r.agentID = &id
	return nil
}

// fill sets the names from the stored ids.
func (r agentRef) fill(tx *gorm.DB) error {
	if *r.agentID != nil && *r.agentName == "" {
		project, agent, err := AgentNamesByID(tx, **r.agentID)
		if err != nil {
			return err
		}
		*r.projectName, *r.agentName = project, agent
	}
	if r.environmentName != nil && *r.environmentName == "" && r.environmentID != uuid.Nil {
		name, err := EnvironmentNameByID(tx, r.environmentID)
		if err != nil {
			return err
		}
		*r.environmentName = name
	}
	return nil
}

// resolveEnvironment sets *envID from the environment's name when it is unset.
func resolveEnvironment(tx *gorm.DB, orgID, envName string, envID *uuid.UUID) error {
	if *envID != uuid.Nil {
		return nil
	}
	var ids []uuid.UUID
	if err := EnvironmentIDQuery(tx, orgID, envName).Pluck("id", &ids).Error; err != nil {
		return err
	}
	if len(ids) == 0 {
		return fmt.Errorf("environment %q not found", envName)
	}
	*envID = ids[0]
	return nil
}
