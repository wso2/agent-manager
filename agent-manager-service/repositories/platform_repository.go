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
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/orgctx"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// CreateProjectInput is the data needed to create a project.
type CreateProjectInput struct {
	Name        string
	DisplayName string
	Description string
	CreatedBy   string
}

// CreateAgentInput is the data needed to register an agent.
type CreateAgentInput struct {
	Name             string
	DisplayName      string
	Description      string
	ProvisioningType string
	AgentType        string
	SubType          string
	Labels           map[string]string
	CreatedBy        string
}

// PlatformRepository stores organizations, projects, the per-org default
// environment and agents. Callers address an organization by the Thunder OU
// id carried in their token; the organization row is created the first time
// an OU is seen, together with its default environment.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/platform_repository_mock.go . PlatformRepository:PlatformRepositoryMock
type PlatformRepository interface {
	GetOrganization(ctx context.Context, ouID string) (*models.OrganizationResponse, error)
	ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error)

	CreateProject(ctx context.Context, ouID string, in CreateProjectInput) (*models.ProjectResponse, error)
	GetProject(ctx context.Context, ouID, projectName string) (*models.ProjectResponse, error)
	PatchProject(ctx context.Context, ouID, projectName string, displayName, description *string) error
	DeleteProject(ctx context.Context, ouID, projectName string) error
	ListProjects(ctx context.Context, ouID string) ([]*models.ProjectResponse, error)

	CreateAgent(ctx context.Context, ouID, projectName string, in CreateAgentInput) (*models.AgentResponse, error)
	GetAgent(ctx context.Context, ouID, projectName, agentName string) (*models.AgentResponse, error)
	// UpdateAgentBasicInfo replaces display name and description; labels, when
	// non-nil, replace the agent's label set.
	UpdateAgentBasicInfo(ctx context.Context, ouID, projectName, agentName, displayName, description string, labels *map[string]string) error
	DeleteAgent(ctx context.Context, ouID, projectName, agentName string) error
	ListAgents(ctx context.Context, ouID, projectName string) ([]*models.AgentResponse, error)
	CountProjectAgents(ctx context.Context, ouID, projectName string) (int, error)
	AgentExists(ctx context.Context, ouID, projectName, agentName string) (bool, error)

	// GetEnvironment returns the org's default environment. The name is
	// accepted for API compatibility and ignored: there is only one.
	GetEnvironment(ctx context.Context, ouID, envName string) (*models.EnvironmentResponse, error)
	ListEnvironments(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error)
}

type platformRepo struct {
	db *gorm.DB
}

func NewPlatformRepo(db *gorm.DB) PlatformRepository {
	return &platformRepo{db: db}
}

func newID() uuid.UUID { return uuid.Must(uuid.NewV7()) }

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// orgByOUID returns the organization for an OU, creating it and its default
// environment on first use.
func (r *platformRepo) orgByOUID(ctx context.Context, ouID string) (*models.Organization, error) {
	if ouID == "" {
		return nil, utils.ErrOrganizationNotFound
	}
	var org models.Organization
	err := r.db.WithContext(ctx).Where("thunder_ou_id = ?", ouID).First(&org).Error
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
	now := time.Now().UTC()
	org = models.Organization{
		ID: newID(), Handle: handle, DisplayName: handle, ThunderOUID: &ouID,
		CreatedAt: now, UpdatedAt: now,
	}
	env := models.Environment{
		ID: newID(), OrgID: org.ID, Name: models.DefaultEnvironmentName, DisplayName: "Default",
		CreatedAt: now, UpdatedAt: now,
	}
	txErr := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(&org).Error; err != nil {
			return err
		}
		return tx.Create(&env).Error
	})
	if txErr != nil {
		// A concurrent request may have created it first.
		var existing models.Organization
		if err := r.db.WithContext(ctx).Where("thunder_ou_id = ?", ouID).First(&existing).Error; err == nil {
			return &existing, nil
		}
		return nil, fmt.Errorf("failed to create organization for OU %q: %w", ouID, txErr)
	}
	return &org, nil
}

func orgResponse(o *models.Organization) *models.OrganizationResponse {
	return &models.OrganizationResponse{
		Name:        o.Handle,
		DisplayName: o.DisplayName,
		Description: deref(o.Description),
		CreatedAt:   o.CreatedAt,
		Handle:      o.Handle,
	}
}

func (r *platformRepo) GetOrganization(ctx context.Context, ouID string) (*models.OrganizationResponse, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return nil, err
	}
	return orgResponse(org), nil
}

func (r *platformRepo) ListOrganizations(ctx context.Context) ([]*models.OrganizationResponse, error) {
	var orgs []models.Organization
	if err := r.db.WithContext(ctx).Order("created_at, id").Find(&orgs).Error; err != nil {
		return nil, err
	}
	out := make([]*models.OrganizationResponse, 0, len(orgs))
	for i := range orgs {
		out = append(out, orgResponse(&orgs[i]))
	}
	return out, nil
}

func projectResponse(org *models.Organization, p *models.Project) *models.ProjectResponse {
	return &models.ProjectResponse{
		UUID:        p.ID.String(),
		Name:        p.Name,
		OrgName:     org.Handle,
		DisplayName: p.DisplayName,
		Description: deref(p.Description),
		CreatedAt:   p.CreatedAt,
	}
}

func (r *platformRepo) project(ctx context.Context, org *models.Organization, projectName string) (*models.Project, error) {
	var p models.Project
	err := r.db.WithContext(ctx).Where("org_id = ? AND name = ?", org.ID, projectName).First(&p).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, utils.ErrProjectNotFound
	}
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *platformRepo) CreateProject(ctx context.Context, ouID string, in CreateProjectInput) (*models.ProjectResponse, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return nil, err
	}
	displayName := in.DisplayName
	if displayName == "" {
		displayName = in.Name
	}
	now := time.Now().UTC()
	p := models.Project{
		ID: newID(), OrgID: org.ID, Name: in.Name, DisplayName: displayName,
		Description: optional(in.Description), CreatedBy: optional(in.CreatedBy),
		CreatedAt: now, UpdatedAt: now,
	}
	res := r.db.WithContext(ctx).Clauses(clause.OnConflict{DoNothing: true}).Create(&p)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, utils.ErrProjectAlreadyExists
	}
	return projectResponse(org, &p), nil
}

func (r *platformRepo) GetProject(ctx context.Context, ouID, projectName string) (*models.ProjectResponse, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return nil, err
	}
	p, err := r.project(ctx, org, projectName)
	if err != nil {
		return nil, err
	}
	return projectResponse(org, p), nil
}

func (r *platformRepo) PatchProject(ctx context.Context, ouID, projectName string, displayName, description *string) error {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return err
	}
	p, err := r.project(ctx, org, projectName)
	if err != nil {
		return err
	}
	updates := map[string]interface{}{"updated_at": time.Now().UTC()}
	if displayName != nil {
		updates["display_name"] = *displayName
	}
	if description != nil {
		updates["description"] = optional(*description)
	}
	return r.db.WithContext(ctx).Model(&models.Project{}).Where("id = ?", p.ID).Updates(updates).Error
}

func (r *platformRepo) DeleteProject(ctx context.Context, ouID, projectName string) error {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return err
	}
	p, err := r.project(ctx, org, projectName)
	if err != nil {
		return err
	}
	var count int64
	if err := r.db.WithContext(ctx).Model(&models.Agent{}).Where("org_id = ? AND project_id = ?", org.ID, p.ID).Count(&count).Error; err != nil {
		return err
	}
	if count > 0 {
		return fmt.Errorf("%w: project %q still has %d agent(s)", utils.ErrConflict, projectName, count)
	}
	return r.db.WithContext(ctx).Delete(&models.Project{}, "id = ?", p.ID).Error
}

func (r *platformRepo) ListProjects(ctx context.Context, ouID string) ([]*models.ProjectResponse, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return nil, err
	}
	var projects []models.Project
	if err := r.db.WithContext(ctx).Where("org_id = ?", org.ID).Order("created_at, id").Find(&projects).Error; err != nil {
		return nil, err
	}
	out := make([]*models.ProjectResponse, 0, len(projects))
	for i := range projects {
		out = append(out, projectResponse(org, &projects[i]))
	}
	return out, nil
}

func agentResponse(projectName string, a *models.Agent) *models.AgentResponse {
	var labels map[string]string
	if len(a.Labels) > 0 {
		labels = make(map[string]string, len(a.Labels))
		for _, l := range a.Labels {
			labels[l.LabelKey] = deref(l.LabelValue)
		}
	}
	return &models.AgentResponse{
		UUID:         a.ID.String(),
		Name:         a.Name,
		DisplayName:  a.DisplayName,
		Description:  deref(a.Description),
		ProjectName:  projectName,
		CreatedAt:    a.CreatedAt,
		Provisioning: models.Provisioning{Type: a.ProvisioningType},
		Type:         models.AgentType{Type: a.AgentType, SubType: deref(a.SubType)},
		Labels:       labels,
	}
}

func (r *platformRepo) agent(ctx context.Context, ouID, projectName, agentName string) (*models.Organization, *models.Project, *models.Agent, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return nil, nil, nil, err
	}
	p, err := r.project(ctx, org, projectName)
	if err != nil {
		return nil, nil, nil, err
	}
	var a models.Agent
	err = r.db.WithContext(ctx).Preload("Labels").
		Where("org_id = ? AND project_id = ? AND name = ?", org.ID, p.ID, agentName).First(&a).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil, nil, utils.ErrAgentNotFound
	}
	if err != nil {
		return nil, nil, nil, err
	}
	return org, p, &a, nil
}

func (r *platformRepo) CreateAgent(ctx context.Context, ouID, projectName string, in CreateAgentInput) (*models.AgentResponse, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return nil, err
	}
	p, err := r.project(ctx, org, projectName)
	if err != nil {
		return nil, err
	}
	displayName := in.DisplayName
	if displayName == "" {
		displayName = in.Name
	}
	now := time.Now().UTC()
	a := models.Agent{
		ID: newID(), OrgID: org.ID, ProjectID: p.ID, Name: in.Name, DisplayName: displayName,
		Description: optional(in.Description), ProvisioningType: in.ProvisioningType,
		AgentType: in.AgentType, SubType: optional(in.SubType), CreatedBy: optional(in.CreatedBy),
		CreatedAt: now, UpdatedAt: now,
	}
	keys := make([]string, 0, len(in.Labels))
	for k := range in.Labels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		a.Labels = append(a.Labels, models.AgentLabel{AgentID: a.ID, OrgID: org.ID, LabelKey: k, LabelValue: optional(in.Labels[k])})
	}
	err = r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).Omit("Labels").Create(&a)
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return utils.ErrAgentAlreadyExists
		}
		if len(a.Labels) > 0 {
			return tx.Create(&a.Labels).Error
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return agentResponse(p.Name, &a), nil
}

func (r *platformRepo) GetAgent(ctx context.Context, ouID, projectName, agentName string) (*models.AgentResponse, error) {
	_, p, a, err := r.agent(ctx, ouID, projectName, agentName)
	if err != nil {
		return nil, err
	}
	return agentResponse(p.Name, a), nil
}

func (r *platformRepo) UpdateAgentBasicInfo(ctx context.Context, ouID, projectName, agentName, displayName, description string, labels *map[string]string) error {
	_, _, a, err := r.agent(ctx, ouID, projectName, agentName)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Model(&models.Agent{}).Where("id = ?", a.ID).Updates(map[string]interface{}{
			"display_name": displayName,
			"description":  optional(description),
			"updated_at":   time.Now().UTC(),
		}).Error; err != nil {
			return err
		}
		if labels == nil {
			return nil
		}
		if err := tx.Delete(&models.AgentLabel{}, "agent_id = ?", a.ID).Error; err != nil {
			return err
		}
		rows := make([]models.AgentLabel, 0, len(*labels))
		for k, v := range *labels {
			rows = append(rows, models.AgentLabel{AgentID: a.ID, OrgID: a.OrgID, LabelKey: k, LabelValue: optional(v)})
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.Create(&rows).Error
	})
}

func (r *platformRepo) DeleteAgent(ctx context.Context, ouID, projectName, agentName string) error {
	_, _, a, err := r.agent(ctx, ouID, projectName, agentName)
	if err != nil {
		return err
	}
	return r.db.WithContext(ctx).Delete(&models.Agent{}, "id = ?", a.ID).Error
}

func (r *platformRepo) ListAgents(ctx context.Context, ouID, projectName string) ([]*models.AgentResponse, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return nil, err
	}
	p, err := r.project(ctx, org, projectName)
	if err != nil {
		return nil, err
	}
	var agents []models.Agent
	if err := r.db.WithContext(ctx).Preload("Labels").
		Where("org_id = ? AND project_id = ?", org.ID, p.ID).Order("created_at DESC, id").Find(&agents).Error; err != nil {
		return nil, err
	}
	out := make([]*models.AgentResponse, 0, len(agents))
	for i := range agents {
		out = append(out, agentResponse(p.Name, &agents[i]))
	}
	return out, nil
}

func (r *platformRepo) CountProjectAgents(ctx context.Context, ouID, projectName string) (int, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return 0, err
	}
	p, err := r.project(ctx, org, projectName)
	if err != nil {
		return 0, err
	}
	var count int64
	err = r.db.WithContext(ctx).Model(&models.Agent{}).Where("org_id = ? AND project_id = ?", org.ID, p.ID).Count(&count).Error
	return int(count), err
}

func (r *platformRepo) AgentExists(ctx context.Context, ouID, projectName, agentName string) (bool, error) {
	_, _, _, err := r.agent(ctx, ouID, projectName, agentName)
	if errors.Is(err, utils.ErrAgentNotFound) {
		return false, nil
	}
	return err == nil, err
}

func environmentResponse(e *models.Environment) *models.EnvironmentResponse {
	return &models.EnvironmentResponse{
		UUID:         e.ID.String(),
		Name:         e.Name,
		DisplayName:  e.DisplayName,
		IsProduction: e.IsProduction,
		CreatedAt:    e.CreatedAt,
	}
}

func (r *platformRepo) GetEnvironment(ctx context.Context, ouID, _ string) (*models.EnvironmentResponse, error) {
	org, err := r.orgByOUID(ctx, ouID)
	if err != nil {
		return nil, err
	}
	var env models.Environment
	err = r.db.WithContext(ctx).Where("org_id = ? AND name = ?", org.ID, models.DefaultEnvironmentName).First(&env).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, utils.ErrEnvironmentNotFound
	}
	if err != nil {
		return nil, err
	}
	return environmentResponse(&env), nil
}

func (r *platformRepo) ListEnvironments(ctx context.Context, ouID string) ([]*models.EnvironmentResponse, error) {
	env, err := r.GetEnvironment(ctx, ouID, models.DefaultEnvironmentName)
	if err != nil {
		return nil, err
	}
	return []*models.EnvironmentResponse{env}, nil
}

// IsDefaultEnvironment reports whether name addresses the only environment.
// An empty name is treated as the default.
func IsDefaultEnvironment(name string) bool {
	return name == "" || strings.EqualFold(name, models.DefaultEnvironmentName)
}
