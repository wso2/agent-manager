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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The services address rows by the caller's Thunder OU and by project, agent and environment names. Tables key them
// by id (schema rule 6). An organization's id is its Thunder OU id, so an OU id is used as org_id directly; the
// lookups below translate project, agent and environment names, so repositories keep the interfaces the services
// call while storing only ids.

// ensureOrg returns the organization for an OU, creating it, its default environment and its default deployment
// pipeline (which holds that environment) on first use.
func ensureOrg(ctx context.Context, db *gorm.DB, ouID string) (*models.Organization, error) {
	if ouID == "" {
		return nil, utils.ErrOrganizationNotFound
	}
	orgID, err := uuid.Parse(ouID)
	if err != nil {
		return nil, fmt.Errorf("%w: OU id %q is not a UUID", utils.ErrOrganizationNotFound, ouID)
	}
	db = db.WithContext(ctx)
	var org models.Organization
	err = db.Where("id = ?", orgID).First(&org).Error
	if err == nil {
		return &org, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, err
	}

	handle := ouID
	if resolved, ok := orgctx.GetResolvedOrg(ctx); ok && resolved.OUID == ouID && resolved.OuHandle != "" {
		handle = resolved.OuHandle
	}
	now := dbx.Now()
	org = models.Organization{
		ID: orgID, Handle: handle, DisplayName: handle, ThunderOUID: &ouID,
		CreatedAt: now, UpdatedAt: now,
	}
	env := models.Environment{
		ID: newID(), OrgID: org.ID, Name: models.DefaultEnvironmentName, DisplayName: "Default",
		CreatedAt: now, UpdatedAt: now,
	}
	pipeline := models.DeploymentPipeline{
		ID: newID(), OrgID: org.ID, Name: models.DefaultPipelineName, DisplayName: "Default",
		CreatedAt: now, UpdatedAt: now,
	}
	stage := models.DeploymentPipelineEnvironment{
		PipelineID: pipeline.ID, EnvironmentID: env.ID, OrgID: org.ID, Position: 0, CreatedAt: now,
	}
	txErr := db.Transaction(func(tx *gorm.DB) error {
		for _, row := range []any{&org, &env, &pipeline, &stage} {
			if err := tx.Create(row).Error; err != nil {
				return err
			}
		}
		return nil
	})
	if txErr != nil {
		// A concurrent request may have created it first.
		var existing models.Organization
		if err := db.Where("id = ?", orgID).First(&existing).Error; err == nil {
			return &existing, nil
		}
		return nil, fmt.Errorf("failed to create organization for OU %q: %w", ouID, txErr)
	}
	return &org, nil
}

// orgIDFor is the id of the caller's organization.
func orgIDFor(ctx context.Context, db *gorm.DB, ouID string) (uuid.UUID, error) {
	org, err := ensureOrg(ctx, db, ouID)
	if err != nil {
		return uuid.Nil, err
	}
	return org.ID, nil
}

// projectIDFor is the id of a project in an organization.
func projectIDFor(ctx context.Context, db *gorm.DB, orgID uuid.UUID, projectName string) (uuid.UUID, error) {
	var p models.Project
	err := db.WithContext(ctx).Select("id").Where("org_id = ? AND name = ?", orgID, projectName).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, utils.ErrProjectNotFound
	}
	return p.ID, err
}

// agentScope identifies an agent and the environment a per-environment row belongs to.
type agentScope struct {
	OrgID         uuid.UUID
	ProjectID     uuid.UUID
	AgentID       uuid.UUID
	EnvironmentID uuid.UUID
}

// agentScopeFor resolves an agent by names. Agent names are unique per org, so the project only has to match.
func agentScopeFor(ctx context.Context, db *gorm.DB, ouID, projectName, agentName string) (agentScope, error) {
	orgID, err := orgIDFor(ctx, db, ouID)
	if err != nil {
		return agentScope{}, err
	}
	var row struct {
		ID        uuid.UUID
		ProjectID uuid.UUID
	}
	err = db.WithContext(ctx).Table("agents a").Select("a.id, a.project_id").
		Joins("JOIN projects p ON p.org_id = a.org_id AND p.id = a.project_id").
		Where("a.org_id = ? AND a.name = ? AND p.name = ?", orgID, agentName, projectName).
		Take(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return agentScope{}, utils.ErrAgentNotFound
	}
	if err != nil {
		return agentScope{}, err
	}
	envID, err := defaultEnvironmentID(ctx, db, orgID)
	if err != nil {
		return agentScope{}, err
	}
	return agentScope{OrgID: orgID, ProjectID: row.ProjectID, AgentID: row.ID, EnvironmentID: envID}, nil
}

// defaultEnvironmentID is the org's only environment: Lite never addresses another (environment names are ignored).
func defaultEnvironmentID(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (uuid.UUID, error) {
	var env models.Environment
	err := db.WithContext(ctx).Select("id").Where("org_id = ? AND name = ?", orgID, models.DefaultEnvironmentName).First(&env).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return uuid.Nil, utils.ErrEnvironmentNotFound
	}
	return env.ID, err
}

// agentNames is what services know an agent by.
type agentNames struct {
	OUID        string
	ProjectName string
	AgentName   string
}

// agentNamesByID reads the names of agents by id, for filling the name fields of rows read by id.
func agentNamesByID(ctx context.Context, db *gorm.DB, ids []uuid.UUID) (map[uuid.UUID]agentNames, error) {
	out := make(map[uuid.UUID]agentNames, len(ids))
	for _, chunk := range dbx.Chunks(ids) {
		var rows []struct {
			ID          uuid.UUID
			AgentName   string
			ProjectName string
			OUID        *string
		}
		err := db.WithContext(ctx).Table("agents a").
			Select("a.id, a.name AS agent_name, p.name AS project_name, o.thunder_ou_id AS ouid").
			Joins("JOIN projects p ON p.org_id = a.org_id AND p.id = a.project_id").
			Joins("JOIN organizations o ON o.id = a.org_id").
			Where("a.id IN ?", chunk).Scan(&rows).Error
		if err != nil {
			return nil, err
		}
		for _, r := range rows {
			out[r.ID] = agentNames{OUID: deref(r.OUID), ProjectName: r.ProjectName, AgentName: r.AgentName}
		}
	}
	return out, nil
}

// ouIDByOrgID is the Thunder OU of an organization.
func ouIDByOrgID(ctx context.Context, db *gorm.DB, orgID uuid.UUID) (string, error) {
	var org models.Organization
	if err := db.WithContext(ctx).Select("thunder_ou_id").Where("id = ?", orgID).First(&org).Error; err != nil {
		return "", err
	}
	return deref(org.ThunderOUID), nil
}

// lockAgent locks an agent's row for the rest of tx, serialising work on that agent (schema rule 10).
func lockAgent(ctx context.Context, tx *gorm.DB, ouID, projectName, agentName string) (agentScope, error) {
	scope, err := agentScopeFor(ctx, tx, ouID, projectName, agentName)
	if err != nil {
		return agentScope{}, err
	}
	return scope, dbx.LockRow(tx.WithContext(ctx), "agents", "id", "id = ?", scope.AgentID)
}
