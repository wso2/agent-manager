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
	"encoding/json"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
)

// WebhookTarget selects the endpoints of one scope: an org, a project, or an
// agent. Fields below the scope are ignored.
type WebhookTarget struct {
	OUID        string
	Scope       string
	ProjectName string
	AgentName   string
}

// WebhookRepository persists webhook endpoints and their delivery log.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/webhook_repository_mock.go . WebhookRepository:WebhookRepositoryMock
type WebhookRepository interface {
	ListEndpoints(ctx context.Context, target WebhookTarget) ([]models.WebhookEndpoint, error)
	// GetEndpoint returns the endpoint with id within target, or
	// gorm.ErrRecordNotFound.
	GetEndpoint(ctx context.Context, target WebhookTarget, id uuid.UUID) (*models.WebhookEndpoint, error)
	// GetEndpointByID returns an endpoint regardless of scope, for the dispatcher.
	GetEndpointByID(ctx context.Context, id uuid.UUID) (*models.WebhookEndpoint, error)
	CreateEndpoint(ctx context.Context, endpoint *models.WebhookEndpoint) error
	UpdateEndpoint(ctx context.Context, endpoint *models.WebhookEndpoint) error
	DeleteEndpoint(ctx context.Context, target WebhookTarget, id uuid.UUID) (bool, error)
	// DeleteEndpointsUnder removes the endpoints of a deleted project (agent
	// empty) or agent.
	DeleteEndpointsUnder(ctx context.Context, ouID, projectName, agentName string) error

	// MatchingEndpoints returns the enabled endpoints an event can reach:
	// the org's org endpoints, the project's project endpoints, or the
	// agent's endpoints that selected the event's environment. An agent event
	// tied to no environment (a build, say) reaches all of the agent's
	// endpoints. Subscription to the event type is checked by the caller.
	MatchingEndpoints(ctx context.Context, ouID, scope, projectName, agentName, environment string) ([]models.WebhookEndpoint, error)

	// RecordDelivery creates or updates the delivery of an event to an endpoint.
	RecordDelivery(ctx context.Context, delivery *models.WebhookDelivery) error
	// RecordQueuedDelivery creates the pending row of a newly queued delivery.
	// It never overwrites an existing row, so a fan-out repeated after the
	// delivery already ran cannot turn "delivered" back into "pending".
	RecordQueuedDelivery(ctx context.Context, delivery *models.WebhookDelivery) error
	ListDeliveries(ctx context.Context, endpointID uuid.UUID, limit int) ([]models.WebhookDelivery, error)
	// PruneDeliveries deletes deliveries created before cutoff.
	PruneDeliveries(ctx context.Context, cutoff time.Time) (int64, error)
}

type webhookRepository struct {
	db *gorm.DB
}

// NewWebhookRepository creates a WebhookRepository.
func NewWebhookRepository(db *gorm.DB) WebhookRepository {
	return &webhookRepository{db: db}
}

func (r *webhookRepository) scoped(ctx context.Context, t WebhookTarget) *gorm.DB {
	q := r.db.WithContext(ctx).Where("ou_id = ? AND scope = ?", t.OUID, t.Scope)
	switch t.Scope {
	case models.WebhookScopeProject:
		q = q.Where("project_name = ?", t.ProjectName)
	case models.WebhookScopeAgent:
		q = q.Where("project_name = ? AND agent_name = ?", t.ProjectName, t.AgentName)
	}
	return q
}

func (r *webhookRepository) ListEndpoints(ctx context.Context, t WebhookTarget) ([]models.WebhookEndpoint, error) {
	var out []models.WebhookEndpoint
	if err := r.scoped(ctx, t).Order("created_at ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *webhookRepository) GetEndpoint(ctx context.Context, t WebhookTarget, id uuid.UUID) (*models.WebhookEndpoint, error) {
	var e models.WebhookEndpoint
	if err := r.scoped(ctx, t).Where("id = ?", id).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *webhookRepository) GetEndpointByID(ctx context.Context, id uuid.UUID) (*models.WebhookEndpoint, error) {
	var e models.WebhookEndpoint
	if err := r.db.WithContext(ctx).Where("id = ?", id).First(&e).Error; err != nil {
		return nil, err
	}
	return &e, nil
}

func (r *webhookRepository) CreateEndpoint(ctx context.Context, e *models.WebhookEndpoint) error {
	now := time.Now()
	e.CreatedAt, e.UpdatedAt = now, now
	return r.db.WithContext(ctx).Create(e).Error
}

func (r *webhookRepository) UpdateEndpoint(ctx context.Context, e *models.WebhookEndpoint) error {
	e.UpdatedAt = time.Now()
	// Update from the struct, not a map, so event_types goes through its JSON
	// serializer; Select also writes zero values such as enabled=false.
	return r.db.WithContext(ctx).Model(&models.WebhookEndpoint{}).Where("id = ?", e.ID).
		Select("name", "description", "url", "environments", "event_types", "enabled", "secret_encrypted", "updated_at").
		Updates(e).Error
}

func (r *webhookRepository) DeleteEndpoint(ctx context.Context, t WebhookTarget, id uuid.UUID) (bool, error) {
	res := r.scoped(ctx, t).Where("id = ?", id).Delete(&models.WebhookEndpoint{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *webhookRepository) DeleteEndpointsUnder(ctx context.Context, ouID, projectName, agentName string) error {
	q := r.db.WithContext(ctx).Where("ou_id = ? AND project_name = ?", ouID, projectName)
	if agentName != "" {
		q = q.Where("agent_name = ?", agentName)
	}
	return q.Delete(&models.WebhookEndpoint{}).Error
}

func (r *webhookRepository) MatchingEndpoints(
	ctx context.Context, ouID, scope, projectName, agentName, environment string,
) ([]models.WebhookEndpoint, error) {
	q := r.scoped(ctx, WebhookTarget{OUID: ouID, Scope: scope, ProjectName: projectName, AgentName: agentName}).
		Where("enabled = TRUE")
	if scope == models.WebhookScopeAgent && environment != "" {
		selected, err := json.Marshal([]string{environment})
		if err != nil {
			return nil, err
		}
		q = q.Where("environments @> ?::jsonb", string(selected))
	}
	var out []models.WebhookEndpoint
	if err := q.Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *webhookRepository) RecordDelivery(ctx context.Context, d *models.WebhookDelivery) error {
	d.UpdatedAt = time.Now()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = d.UpdatedAt
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns: []clause.Column{{Name: "endpoint_id"}, {Name: "event_id"}},
		DoUpdates: clause.AssignmentColumns([]string{
			"status", "attempts", "response_code", "last_error", "delivered_at", "updated_at",
		}),
	}).Create(d).Error
}

func (r *webhookRepository) RecordQueuedDelivery(ctx context.Context, d *models.WebhookDelivery) error {
	d.UpdatedAt = time.Now()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = d.UpdatedAt
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "endpoint_id"}, {Name: "event_id"}},
		DoNothing: true,
	}).Create(d).Error
}

func (r *webhookRepository) ListDeliveries(ctx context.Context, endpointID uuid.UUID, limit int) ([]models.WebhookDelivery, error) {
	var out []models.WebhookDelivery
	err := r.db.WithContext(ctx).Where("endpoint_id = ?", endpointID).
		Order("created_at DESC").Limit(limit).Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *webhookRepository) PruneDeliveries(ctx context.Context, cutoff time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", cutoff).Delete(&models.WebhookDelivery{})
	return res.RowsAffected, res.Error
}
