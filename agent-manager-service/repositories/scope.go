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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// The services address rows by the caller's Thunder OU and by project, agent and environment names. Tables key them
// by id (schema rule 6). An organization's id is its Thunder OU id, so an OU id is used as org_id directly; the
// lookups below translate project, agent and environment names, so repositories keep the interfaces the services
// call while storing only ids.

// EnsureOrg makes the organization of an OU ready to hold rows and returns its id. An organization is the token's;
// the database holds no row for it, only what belongs to it, first of all its default environment, which this
// creates on first use.
func EnsureOrg(ctx context.Context, db *gorm.DB, ouID string) (uuid.UUID, error) {
	if ouID == "" {
		return uuid.Nil, utils.ErrOrganizationNotFound
	}
	orgID, err := uuid.Parse(ouID)
	if err != nil {
		return uuid.Nil, fmt.Errorf("%w: OU id %q is not a UUID", utils.ErrOrganizationNotFound, ouID)
	}
	db = db.WithContext(ctx)
	exists, err := hasDefaultEnvironment(db, orgID)
	if err != nil || exists {
		return orgID, err
	}

	now := dbx.Now()
	env := models.Environment{
		ID: NewID(), OrgID: orgID, Name: models.DefaultEnvironmentName, DisplayName: "Default",
		CreatedAt: now, UpdatedAt: now,
	}
	// The default environment is the only one an organization gets, and it
	// comes with the organization, possibly on a read: it cannot be refused,
	// so environment hooks get only the apply phase, in a savepoint. A module
	// that fails there is logged and its writes undone; the environment is
	// created all the same.
	op := coreext.Begin(ctx, modkit.ResourceEnvironment)
	envReq := modkit.HookRequest{Action: modkit.ActionCreate, OrgID: orgID.String()}
	txErr := db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&env).Error; err != nil {
			return err
		}
		object := environmentObject(&env)
		hookErr := tx.Transaction(func(sp *gorm.DB) error {
			if err := op.Run(ctx, sp, modkit.Apply, withObject(envReq, env.ID.String(), object), nil); err != nil {
				return err
			}
			return emit(ctx, op, sp, modkit.EventEnvironmentCreated, orgID.String(), env.Name, object)
		})
		if hookErr != nil {
			slog.ErrorContext(ctx, "module hooks failed on a new organization's default environment; created without them",
				"orgId", orgID.String(), "error", hookErr)
		}
		return nil
	})
	if txErr != nil {
		// A concurrent request may have created it first.
		if exists, err := hasDefaultEnvironment(db, orgID); err == nil && exists {
			return orgID, nil
		}
		return uuid.Nil, fmt.Errorf("failed to create the default environment of organization %q: %w", ouID, txErr)
	}
	return orgID, nil
}

// hasDefaultEnvironment reports whether the organization has its default environment, which is what makes it exist.
func hasDefaultEnvironment(db *gorm.DB, orgID uuid.UUID) (bool, error) {
	var n int64
	err := db.Model(&models.Environment{}).Where("org_id = ? AND name = ?", orgID, models.DefaultEnvironmentName).
		Count(&n).Error
	return n > 0, err
}

// orgHandle is an organization's handle: the request's when it is for this organization; outside a request, the
// configured default organization's; otherwise the id, as nothing else names it.
func orgHandle(ctx context.Context, orgID string) string {
	if resolved, ok := orgctx.GetResolvedOrg(ctx); ok && sameOrg(resolved.OUID, orgID) && resolved.OuHandle != "" {
		return resolved.OuHandle
	}
	if def := config.GetConfig().DefaultOrg; sameOrg(def.OUID, orgID) && def.Handle != "" {
		return def.Handle
	}
	return orgID
}

// sameOrg reports whether two organization ids name the same UUID, whatever their case.
func sameOrg(a, b string) bool {
	ua, errA := uuid.Parse(a)
	ub, errB := uuid.Parse(b)
	return errA == nil && errB == nil && ua == ub
}

// environmentObject is an environment as module hooks and events see it.
func environmentObject(e *models.Environment) map[string]any {
	return map[string]any{
		"id": e.ID.String(), "orgId": e.OrgID.String(), "name": e.Name,
		"displayName": e.DisplayName, "isProduction": e.IsProduction,
	}
}

// orgIDFor is the id of the caller's organization.
func orgIDFor(ctx context.Context, db *gorm.DB, ouID string) (uuid.UUID, error) {
	return EnsureOrg(ctx, db, ouID)
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

// LockAgent locks an agent's row for the rest of tx, serialising work on that agent (schema rule 10).
func LockAgent(ctx context.Context, tx *gorm.DB, ouID, projectName, agentName string) (agentScope, error) {
	scope, err := agentScopeFor(ctx, tx, ouID, projectName, agentName)
	if err != nil {
		return agentScope{}, err
	}
	return scope, dbx.LockRow(tx.WithContext(ctx), "agents", "id", "id = ?", scope.AgentID)
}

// AgentCond matches the rows of the named agent.
func AgentCond(db *gorm.DB, ouID, projectName, agentName string) (string, []any) {
	return "org_id = ? AND agent_id IN (?)", []any{ouID, models.AgentIDQuery(db, ouID, projectName, agentName)}
}

// AgentEnvCond matches the named agent's rows in the named environment.
func AgentEnvCond(db *gorm.DB, ouID, projectName, agentName, environmentName string) (string, []any) {
	cond, args := AgentCond(db, ouID, projectName, agentName)
	return cond + " AND environment_id IN (?)", append(args, models.EnvironmentIDQuery(db, ouID, environmentName))
}
