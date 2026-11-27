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

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dbx"
)

// coreReadAPI is the read API modules get through Runtime.Core. Every query is
// scoped to the organization the caller names, which must be the one of the
// request, hook or event the call is made for; only EnsureOrg creates one.
type coreReadAPI struct{ db *gorm.DB }

// NewCoreReadAPI returns the core read API for modules.
func NewCoreReadAPI(db *gorm.DB) modkit.CoreAPI { return &coreReadAPI{db: db} }

// parseIDs parses ids that address core rows. A malformed id addresses nothing.
func parseIDs(ids ...string) ([]uuid.UUID, error) {
	out := make([]uuid.UUID, 0, len(ids))
	for _, s := range ids {
		id, err := uuid.Parse(s)
		if err != nil {
			return nil, modkit.ErrNotFound
		}
		out = append(out, id)
	}
	return out, nil
}

// orgRows parses an id that narrows a listing. A malformed id lists nothing.
func orgRows(orgID string) (uuid.UUID, bool) {
	id, err := uuid.Parse(orgID)
	return id, err == nil
}

// foreign reports whether ctx belongs to another organization than orgID.
func foreign(ctx context.Context, orgID string) bool {
	own := modkit.OrgID(ctx)
	return own != "" && own != orgID
}

func notFound(err error) error {
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return modkit.ErrNotFound
	}
	return err
}

// GetOrg is the organization if it exists, that is if it has its default environment. A read: it never creates it.
func (c *coreReadAPI) GetOrg(ctx context.Context, orgID string) (modkit.Org, error) {
	if foreign(ctx, orgID) {
		return modkit.Org{}, modkit.ErrNotFound
	}

	ids, err := parseIDs(orgID)
	if err != nil {
		return modkit.Org{}, err
	}
	exists, err := hasDefaultEnvironment(c.db.WithContext(ctx), ids[0])
	if err != nil {
		return modkit.Org{}, err
	}
	if !exists {
		return modkit.Org{}, modkit.ErrNotFound
	}
	return modkit.Org{ID: ids[0].String(), Handle: orgHandle(ctx, ids[0].String())}, nil
}

func toProject(p *models.Project) modkit.Project {
	return modkit.Project{ID: p.ID.String(), OrgID: p.OrgID.String(), Name: p.Name, DisplayName: p.DisplayName, CreatedAt: p.CreatedAt}
}

func (c *coreReadAPI) ListProjects(ctx context.Context, orgID string) ([]modkit.Project, error) {
	if foreign(ctx, orgID) {
		return []modkit.Project{}, nil
	}

	org, ok := orgRows(orgID)
	if !ok {
		return []modkit.Project{}, nil
	}
	var rows []models.Project
	if err := c.db.WithContext(ctx).Where("org_id = ?", org).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]modkit.Project, 0, len(rows))
	for i := range rows {
		out = append(out, toProject(&rows[i]))
	}
	return out, nil
}

func (c *coreReadAPI) GetProject(ctx context.Context, orgID, projectID string) (modkit.Project, error) {
	if foreign(ctx, orgID) {
		return modkit.Project{}, modkit.ErrNotFound
	}

	ids, err := parseIDs(orgID, projectID)
	if err != nil {
		return modkit.Project{}, err
	}
	var p models.Project
	if err := c.db.WithContext(ctx).Where("org_id = ? AND id = ?", ids[0], ids[1]).First(&p).Error; err != nil {
		return modkit.Project{}, notFound(err)
	}
	return toProject(&p), nil
}

type agentWithProject struct {
	models.Agent
	ProjectName string `gorm:"column:project_name"`
}

func (c *coreReadAPI) agents(ctx context.Context) *gorm.DB {
	return c.db.WithContext(ctx).Table("agents").
		Select("agents.*, projects.name AS project_name").
		Joins("JOIN projects ON projects.org_id = agents.org_id AND projects.id = agents.project_id")
}

func toAgent(a *agentWithProject) modkit.Agent {
	return modkit.Agent{
		ID: a.ID.String(), OrgID: a.OrgID.String(), ProjectID: a.ProjectID.String(), ProjectName: a.ProjectName,
		Name: a.Name, DisplayName: a.DisplayName, Provisioning: a.ProvisioningType, CreatedAt: a.CreatedAt,
	}
}

func (c *coreReadAPI) ListAgents(ctx context.Context, orgID, projectID string) ([]modkit.Agent, error) {
	if foreign(ctx, orgID) {
		return []modkit.Agent{}, nil
	}

	org, ok := orgRows(orgID)
	if !ok {
		return []modkit.Agent{}, nil
	}
	q := c.agents(ctx).Where("agents.org_id = ?", org)
	if projectID != "" {
		pid, ok := orgRows(projectID)
		if !ok {
			return []modkit.Agent{}, nil
		}
		q = q.Where("agents.project_id = ?", pid)
	}
	var rows []agentWithProject
	if err := q.Order("agents.name").Scan(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]modkit.Agent, 0, len(rows))
	for i := range rows {
		out = append(out, toAgent(&rows[i]))
	}
	return out, nil
}

func (c *coreReadAPI) GetAgent(ctx context.Context, orgID, agentID string) (modkit.Agent, error) {
	if foreign(ctx, orgID) {
		return modkit.Agent{}, modkit.ErrNotFound
	}

	ids, err := parseIDs(orgID, agentID)
	if err != nil {
		return modkit.Agent{}, err
	}
	var rows []agentWithProject
	if err := c.agents(ctx).Where("agents.org_id = ? AND agents.id = ?", ids[0], ids[1]).Scan(&rows).Error; err != nil {
		return modkit.Agent{}, err
	}
	if len(rows) == 0 {
		return modkit.Agent{}, modkit.ErrNotFound
	}
	return toAgent(&rows[0]), nil
}

func (c *coreReadAPI) ListEnvironments(ctx context.Context, orgID string) ([]modkit.Environment, error) {
	if foreign(ctx, orgID) {
		return []modkit.Environment{}, nil
	}

	org, ok := orgRows(orgID)
	if !ok {
		return []modkit.Environment{}, nil
	}
	var rows []models.Environment
	if err := c.db.WithContext(ctx).Where("org_id = ?", org).Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	out := make([]modkit.Environment, 0, len(rows))
	for i := range rows {
		out = append(out, toEnvironment(&rows[i]))
	}
	return out, nil
}

func (c *coreReadAPI) GetProjectByName(ctx context.Context, orgID, name string) (modkit.Project, error) {
	if foreign(ctx, orgID) {
		return modkit.Project{}, modkit.ErrNotFound
	}
	ids, err := parseIDs(orgID)
	if err != nil {
		return modkit.Project{}, err
	}
	var p models.Project
	if err := c.db.WithContext(ctx).Where("org_id = ? AND name = ?", ids[0], name).First(&p).Error; err != nil {
		return modkit.Project{}, notFound(err)
	}
	return toProject(&p), nil
}

func (c *coreReadAPI) GetAgentByName(ctx context.Context, orgID, projectName, name string) (modkit.Agent, error) {
	if foreign(ctx, orgID) {
		return modkit.Agent{}, modkit.ErrNotFound
	}
	ids, err := parseIDs(orgID)
	if err != nil {
		return modkit.Agent{}, err
	}
	var rows []agentWithProject
	if err := c.agents(ctx).Where("agents.org_id = ? AND projects.name = ? AND agents.name = ?", ids[0], projectName, name).
		Scan(&rows).Error; err != nil {
		return modkit.Agent{}, err
	}
	if len(rows) == 0 {
		return modkit.Agent{}, modkit.ErrNotFound
	}
	return toAgent(&rows[0]), nil
}

func toEnvironment(e *models.Environment) modkit.Environment {
	return modkit.Environment{
		ID: e.ID.String(), OrgID: e.OrgID.String(), Name: e.Name, DisplayName: e.DisplayName,
		IsProduction: e.IsProduction, CreatedAt: e.CreatedAt,
	}
}

func (c *coreReadAPI) GetEnvironmentByName(ctx context.Context, orgID, name string) (modkit.Environment, error) {
	if foreign(ctx, orgID) {
		return modkit.Environment{}, modkit.ErrNotFound
	}
	ids, err := parseIDs(orgID)
	if err != nil {
		return modkit.Environment{}, err
	}
	var e models.Environment
	if err := c.db.WithContext(ctx).Where("org_id = ? AND name = ?", ids[0], name).First(&e).Error; err != nil {
		return modkit.Environment{}, notFound(err)
	}
	return toEnvironment(&e), nil
}

// LockAgent locks the agent's row through tx, the caller's transaction, so it
// is held until that transaction ends.
func (c *coreReadAPI) LockAgent(ctx context.Context, tx *gorm.DB, orgID, agentID string) (modkit.Agent, error) {
	if tx == nil {
		return modkit.Agent{}, errors.New("modkit: LockAgent needs a transaction")
	}
	if foreign(ctx, orgID) {
		return modkit.Agent{}, modkit.ErrNotFound
	}
	ids, err := parseIDs(orgID, agentID)
	if err != nil {
		return modkit.Agent{}, err
	}
	if err := dbx.LockRow(tx.WithContext(ctx), "agents", "id", "org_id = ? AND id = ?", ids[0], ids[1]); err != nil {
		return modkit.Agent{}, notFound(err)
	}
	var rows []agentWithProject
	if err := tx.WithContext(ctx).Table("agents").Select("agents.*, projects.name AS project_name").
		Joins("JOIN projects ON projects.org_id = agents.org_id AND projects.id = agents.project_id").
		Where("agents.id = ?", ids[1]).Scan(&rows).Error; err != nil {
		return modkit.Agent{}, err
	}
	if len(rows) == 0 {
		return modkit.Agent{}, modkit.ErrNotFound
	}
	return toAgent(&rows[0]), nil
}

// EnsureOrg makes the organization ready, creating its default environment, as the core does on its first request.
func (c *coreReadAPI) EnsureOrg(ctx context.Context, orgID string) (modkit.Org, error) {
	if foreign(ctx, orgID) {
		return modkit.Org{}, modkit.ErrNotFound
	}
	if _, err := parseIDs(orgID); err != nil {
		return modkit.Org{}, err
	}
	id, err := EnsureOrg(ctx, c.db, orgID)
	if err != nil {
		return modkit.Org{}, err
	}
	return modkit.Org{ID: id.String(), Handle: orgHandle(ctx, id.String())}, nil
}
