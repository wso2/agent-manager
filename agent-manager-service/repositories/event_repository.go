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
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgconn"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// EventTarget selects the subscriptions of one owner: an org, a project or an
// agent. Fields below the scope are ignored.
type EventTarget struct {
	OUID        string
	Scope       string
	ProjectName string
	AgentName   string
}

// DestinationStats summarises how a destination is used and how its
// deliveries go.
type DestinationStats struct {
	// UsedBy is the number of subscriptions that send to the destination.
	UsedBy int
	// LastDeliveryAt is when the latest delivery was queued, nil if none.
	LastDeliveryAt *time.Time
	// LastStatus is the status of the latest delivery that was not skipped.
	LastStatus string
	// ConsecutiveFailures counts the deliveries that failed, or are being
	// retried, since the latest one that was delivered.
	ConsecutiveFailures int
}

// DeliveryLocation narrows deliveries to the events of a project or an
// agent. An empty ProjectName keeps the whole org.
type DeliveryLocation struct {
	ProjectName string
	AgentName   string
	// EventScopes lists the event scopes kept; empty keeps all.
	EventScopes []string
}

// SubscriptionCounts holds, per subscription, its deliveries by status.
type SubscriptionCounts map[uuid.UUID]map[string]int

// EventRepository persists event destinations, event subscriptions and the
// delivery log.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/event_repository_mock.go . EventRepository:EventRepositoryMock
type EventRepository interface {
	ListDestinations(ctx context.Context, ouID string) ([]models.EventDestination, error)
	// GetDestination returns the org's destination with id, or
	// gorm.ErrRecordNotFound.
	GetDestination(ctx context.Context, ouID string, id uuid.UUID) (*models.EventDestination, error)
	// DestinationsByID returns those of ids that are the org's destinations.
	DestinationsByID(ctx context.Context, ouID string, ids []uuid.UUID) ([]models.EventDestination, error)
	// CreateDestination saves a new destination. A name the org already uses,
	// in any case, fails with utils.ErrEventDestinationNameTaken.
	CreateDestination(ctx context.Context, d *models.EventDestination) error
	// UpdateDestination saves d with its row locked. check gets the
	// subscriptions that send to it, read under the lock so none can be added
	// meanwhile, and can veto the update: its error is returned as is.
	UpdateDestination(ctx context.Context, d *models.EventDestination, check func(using []models.EventSubscription) error) error
	// DeleteDestination deletes the org's destination with id. One that
	// subscriptions still send to fails with utils.ErrEventDestinationInUse.
	DeleteDestination(ctx context.Context, ouID string, id uuid.UUID) (bool, error)
	// DestinationStats returns the stats of each of the org's destinations.
	DestinationStats(ctx context.Context, ouID string) (map[uuid.UUID]DestinationStats, error)
	// DestinationStatsOf returns the stats of one of the org's destinations,
	// zero when it has none.
	DestinationStatsOf(ctx context.Context, ouID string, id uuid.UUID) (DestinationStats, error)
	// SubscriptionsUsing returns the subscriptions that send to a destination.
	SubscriptionsUsing(ctx context.Context, ouID string, destinationID uuid.UUID) ([]models.EventSubscription, error)
	// RemoveProjectFromDestinations takes a deleted project out of the
	// destinations made available to it.
	RemoveProjectFromDestinations(ctx context.Context, ouID, projectName string) error

	// ListSubscriptions returns the subscriptions target owns, oldest first.
	ListSubscriptions(ctx context.Context, target EventTarget) ([]models.EventSubscription, error)
	// GetSubscription returns target's subscription with id, or
	// gorm.ErrRecordNotFound.
	GetSubscription(ctx context.Context, target EventTarget, id uuid.UUID) (*models.EventSubscription, error)
	// SubscriptionsByID returns those of ids that are the org's
	// subscriptions.
	SubscriptionsByID(ctx context.Context, ouID string, ids []uuid.UUID) ([]models.EventSubscription, error)
	// CreateSubscription saves a new subscription. check gets its
	// destination, read with the row share-locked so its availability cannot
	// change meanwhile, and can veto the save: its error is returned as is.
	// A destination that is not the org's fails with gorm.ErrRecordNotFound.
	CreateSubscription(ctx context.Context, s *models.EventSubscription, check func(*models.EventDestination) error) error
	// UpdateSubscription saves s, checking its destination as
	// CreateSubscription does.
	UpdateSubscription(ctx context.Context, s *models.EventSubscription, check func(*models.EventDestination) error) error
	DeleteSubscription(ctx context.Context, target EventTarget, id uuid.UUID) (bool, error)
	// DeleteSubscriptionsUnder removes the subscriptions of a deleted project
	// (agentName empty: its own and its agents') or agent.
	DeleteSubscriptionsUnder(ctx context.Context, ouID, projectName, agentName string) error
	// CandidateSubscriptions returns the enabled subscriptions an event of
	// the project and agent could reach: the org's, the project's and the
	// agent's. The caller applies the matching rules.
	CandidateSubscriptions(ctx context.Context, ouID, projectName, agentName string) ([]models.EventSubscription, error)
	// SubscriptionCounts counts, by status, each subscription's deliveries
	// queued since since, keeping only the deliveries in loc.
	SubscriptionCounts(ctx context.Context, ouID string, ids []uuid.UUID, since time.Time, loc DeliveryLocation) (SubscriptionCounts, error)
}

type eventRepository struct {
	db *gorm.DB
}

// NewEventRepository creates an EventRepository.
func NewEventRepository(db *gorm.DB) EventRepository {
	return &eventRepository{db: db}
}

// pgErrorCode returns the PostgreSQL error code of err, if it has one.
func pgErrorCode(err error) string {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) {
		return pgErr.Code
	}
	return ""
}

const (
	pgUniqueViolation     = "23505"
	pgForeignKeyViolation = "23503"
)

func (r *eventRepository) ListDestinations(ctx context.Context, ouID string) ([]models.EventDestination, error) {
	var out []models.EventDestination
	err := r.db.WithContext(ctx).Where("ou_id = ?", ouID).Order("lower(name) ASC").Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *eventRepository) GetDestination(ctx context.Context, ouID string, id uuid.UUID) (*models.EventDestination, error) {
	var d models.EventDestination
	if err := r.db.WithContext(ctx).Where("ou_id = ? AND id = ?", ouID, id).First(&d).Error; err != nil {
		return nil, err
	}
	return &d, nil
}

func (r *eventRepository) DestinationsByID(ctx context.Context, ouID string, ids []uuid.UUID) ([]models.EventDestination, error) {
	out := []models.EventDestination{}
	if len(ids) == 0 {
		return out, nil
	}
	if err := r.db.WithContext(ctx).Where("ou_id = ? AND id IN ?", ouID, ids).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *eventRepository) CreateDestination(ctx context.Context, d *models.EventDestination) error {
	now := time.Now().UTC().Truncate(time.Microsecond)
	d.CreatedAt, d.UpdatedAt = now, now
	err := r.db.WithContext(ctx).Create(d).Error
	if pgErrorCode(err) == pgUniqueViolation {
		return utils.ErrEventDestinationNameTaken
	}
	return err
}

func (r *eventRepository) UpdateDestination(
	ctx context.Context, d *models.EventDestination, check func(using []models.EventSubscription) error,
) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var locked models.EventDestination
		err := tx.Clauses(clause.Locking{Strength: "UPDATE"}).
			Where("ou_id = ? AND id = ?", d.OUID, d.ID).First(&locked).Error
		if err != nil {
			return err
		}
		// The caller loaded d before this transaction. A row lock alone does
		// not prevent its stale settings or secret from undoing another write.
		if !locked.UpdatedAt.Equal(d.UpdatedAt) {
			return fmt.Errorf("%w: destination changed; reload it before saving", utils.ErrConflict)
		}
		var using []models.EventSubscription
		if err := tx.Where("destination_id = ?", d.ID).Order("created_at ASC").Find(&using).Error; err != nil {
			return err
		}
		if check != nil {
			if err := check(using); err != nil {
				return err
			}
		}
		// PostgreSQL stores microseconds. Keep the returned revision identical
		// to the stored one and advance it even for writes in one clock tick.
		d.UpdatedAt = time.Now().UTC().Truncate(time.Microsecond)
		if !d.UpdatedAt.After(locked.UpdatedAt) {
			d.UpdatedAt = locked.UpdatedAt.Add(time.Microsecond)
		}
		// Update from the struct, not a map, so the JSON columns go through
		// their serializers; Select also writes zero values.
		return tx.Model(&models.EventDestination{}).Where("id = ?", d.ID).
			Select("name", "description", "config", "secret_encrypted", "availability", "projects", "updated_at").
			UpdateColumns(d).Error
	})
	if pgErrorCode(err) == pgUniqueViolation {
		return utils.ErrEventDestinationNameTaken
	}
	return err
}

func (r *eventRepository) DeleteDestination(ctx context.Context, ouID string, id uuid.UUID) (bool, error) {
	res := r.db.WithContext(ctx).Where("ou_id = ? AND id = ?", ouID, id).Delete(&models.EventDestination{})
	if res.Error != nil {
		if pgErrorCode(res.Error) == pgForeignKeyViolation {
			return false, utils.ErrEventDestinationInUse
		}
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

// destinationStatsRow is one row of the destination stats query.
type destinationStatsRow struct {
	DestinationID       uuid.UUID
	UsedBy              int
	LastDeliveryAt      *time.Time
	LastStatus          string
	ConsecutiveFailures int
}

func (r *eventRepository) DestinationStats(ctx context.Context, ouID string) (map[uuid.UUID]DestinationStats, error) {
	return r.destinationStats(ctx, ouID, nil)
}

func (r *eventRepository) DestinationStatsOf(ctx context.Context, ouID string, id uuid.UUID) (DestinationStats, error) {
	stats, err := r.destinationStats(ctx, ouID, &id)
	if err != nil {
		return DestinationStats{}, err
	}
	return stats[id], nil
}

// destinationStats reads the stats of the org's destinations, or of only the
// one with id when id is set.
func (r *eventRepository) destinationStats(
	ctx context.Context, ouID string, id *uuid.UUID,
) (map[uuid.UUID]DestinationStats, error) {
	only := ""
	args := []any{
		models.EventDeliverySkipped,
		models.EventDeliveryFailed, models.EventDeliveryPending,
		models.EventDeliveryDelivered,
		ouID,
	}
	if id != nil {
		only = " AND d.id = ?"
		args = append(args, *id)
	}
	var rows []destinationStatsRow
	err := r.db.WithContext(ctx).Raw(`
		SELECT d.id AS destination_id,
			COALESCE(u.used_by, 0) AS used_by,
			l.created_at AS last_delivery_at,
			COALESCE(l.status, '') AS last_status,
			COALESCE(f.failures, 0) AS consecutive_failures
		FROM event_destinations d
		LEFT JOIN LATERAL (
			SELECT count(*) AS used_by FROM event_subscriptions s WHERE s.destination_id = d.id
		) u ON TRUE
		LEFT JOIN LATERAL (
			SELECT x.created_at, x.status FROM event_deliveries x
			WHERE x.destination_id = d.id AND x.status <> ?
			ORDER BY x.created_at DESC LIMIT 1
		) l ON TRUE
		LEFT JOIN LATERAL (
			SELECT count(*) AS failures FROM event_deliveries x
			WHERE x.destination_id = d.id
				AND (x.status = ? OR (x.status = ? AND x.attempts > 0))
				AND x.created_at > COALESCE((
					SELECT max(y.created_at) FROM event_deliveries y
					WHERE y.destination_id = d.id AND y.status = ?
				), '-infinity'::timestamptz)
		) f ON TRUE
		WHERE d.ou_id = ?`+only,
		args...,
	).Scan(&rows).Error
	if err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]DestinationStats, len(rows))
	for _, row := range rows {
		out[row.DestinationID] = DestinationStats{
			UsedBy:              row.UsedBy,
			LastDeliveryAt:      row.LastDeliveryAt,
			LastStatus:          row.LastStatus,
			ConsecutiveFailures: row.ConsecutiveFailures,
		}
	}
	return out, nil
}

func (r *eventRepository) SubscriptionsUsing(
	ctx context.Context, ouID string, destinationID uuid.UUID,
) ([]models.EventSubscription, error) {
	var out []models.EventSubscription
	err := r.db.WithContext(ctx).Where("ou_id = ? AND destination_id = ?", ouID, destinationID).
		Order("created_at ASC").Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (r *eventRepository) RemoveProjectFromDestinations(ctx context.Context, ouID, projectName string) error {
	selected, err := json.Marshal([]string{projectName})
	if err != nil {
		return err
	}
	// "@>" rather than the jsonb "?" operator, which gorm would read as a
	// placeholder. A destination left with no project becomes org-only:
	// available to none, rather than silently to all, and still valid to save
	// unchanged.
	return r.db.WithContext(ctx).Exec(`
		UPDATE event_destinations
		SET projects = projects - ?::text,
			availability = CASE WHEN jsonb_array_length(projects - ?::text) = 0 THEN ? ELSE availability END,
			updated_at = NOW()
		WHERE ou_id = ? AND availability = ? AND projects @> ?::jsonb`,
		projectName, projectName, models.DestinationAvailableOrg,
		ouID, models.DestinationAvailableProjects, string(selected),
	).Error
}

func (r *eventRepository) owned(ctx context.Context, t EventTarget) *gorm.DB {
	q := r.db.WithContext(ctx).Where("ou_id = ? AND scope = ?", t.OUID, t.Scope)
	switch t.Scope {
	case models.EventScopeProject:
		q = q.Where("project_name = ?", t.ProjectName)
	case models.EventScopeAgent:
		q = q.Where("project_name = ? AND agent_name = ?", t.ProjectName, t.AgentName)
	}
	return q
}

func (r *eventRepository) ListSubscriptions(ctx context.Context, t EventTarget) ([]models.EventSubscription, error) {
	var out []models.EventSubscription
	if err := r.owned(ctx, t).Order("created_at ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

func (r *eventRepository) GetSubscription(ctx context.Context, t EventTarget, id uuid.UUID) (*models.EventSubscription, error) {
	var s models.EventSubscription
	if err := r.owned(ctx, t).Where("id = ?", id).First(&s).Error; err != nil {
		return nil, err
	}
	return &s, nil
}

func (r *eventRepository) SubscriptionsByID(ctx context.Context, ouID string, ids []uuid.UUID) ([]models.EventSubscription, error) {
	out := []models.EventSubscription{}
	if len(ids) == 0 {
		return out, nil
	}
	if err := r.db.WithContext(ctx).Where("ou_id = ? AND id IN ?", ouID, ids).Find(&out).Error; err != nil {
		return nil, err
	}
	return out, nil
}

// withDestination runs save inside a transaction that first reads s's
// destination with its row share-locked and passes it to check.
func (r *eventRepository) withDestination(
	ctx context.Context, s *models.EventSubscription, check func(*models.EventDestination) error, save func(tx *gorm.DB) error,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var d models.EventDestination
		err := tx.Clauses(clause.Locking{Strength: "SHARE"}).
			Where("ou_id = ? AND id = ?", s.OUID, s.DestinationID).First(&d).Error
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(&d); err != nil {
				return err
			}
		}
		return save(tx)
	})
}

func (r *eventRepository) CreateSubscription(
	ctx context.Context, s *models.EventSubscription, check func(*models.EventDestination) error,
) error {
	return r.withDestination(ctx, s, check, func(tx *gorm.DB) error {
		now := time.Now()
		s.CreatedAt, s.UpdatedAt = now, now
		return tx.Create(s).Error
	})
}

func (r *eventRepository) UpdateSubscription(
	ctx context.Context, s *models.EventSubscription, check func(*models.EventDestination) error,
) error {
	return r.withDestination(ctx, s, check, func(tx *gorm.DB) error {
		s.UpdatedAt = time.Now()
		return tx.Model(&models.EventSubscription{}).Where("id = ?", s.ID).
			Select("name", "description", "destination_id", "event_types", "environments", "enabled", "updated_at").
			Updates(s).Error
	})
}

func (r *eventRepository) DeleteSubscription(ctx context.Context, t EventTarget, id uuid.UUID) (bool, error) {
	res := r.owned(ctx, t).Where("id = ?", id).Delete(&models.EventSubscription{})
	if res.Error != nil {
		return false, res.Error
	}
	return res.RowsAffected > 0, nil
}

func (r *eventRepository) DeleteSubscriptionsUnder(ctx context.Context, ouID, projectName, agentName string) error {
	q := r.db.WithContext(ctx).Where("ou_id = ? AND project_name = ?", ouID, projectName)
	if agentName != "" {
		q = q.Where("agent_name = ?", agentName)
	}
	return q.Delete(&models.EventSubscription{}).Error
}

func (r *eventRepository) CandidateSubscriptions(
	ctx context.Context, ouID, projectName, agentName string,
) ([]models.EventSubscription, error) {
	var out []models.EventSubscription
	err := r.db.WithContext(ctx).
		Where("ou_id = ? AND enabled = TRUE", ouID).
		Where(r.db.Where("scope = ?", models.EventScopeOrg).
			Or("scope = ? AND project_name = ?", models.EventScopeProject, projectName).
			Or("scope = ? AND project_name = ? AND agent_name = ?", models.EventScopeAgent, projectName, agentName)).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, nil
}

// located narrows q, on event_deliveries aliased x, to loc.
func located(q *gorm.DB, loc DeliveryLocation) *gorm.DB {
	if loc.ProjectName != "" {
		q = q.Where("x.project_name = ?", loc.ProjectName)
		if loc.AgentName != "" {
			q = q.Where("x.agent_name = ?", loc.AgentName)
		}
	}
	if len(loc.EventScopes) > 0 {
		q = q.Where("x.event_scope IN ?", loc.EventScopes)
	}
	return q
}

// subscriptionCountRow is one row of the subscription counts query.
type subscriptionCountRow struct {
	SubscriptionID string
	Status         string
	Count          int
}

func (r *eventRepository) SubscriptionCounts(
	ctx context.Context, ouID string, ids []uuid.UUID, since time.Time, loc DeliveryLocation,
) (SubscriptionCounts, error) {
	out := SubscriptionCounts{}
	if len(ids) == 0 {
		return out, nil
	}
	wanted := make([]string, len(ids))
	for i, id := range ids {
		wanted[i] = id.String()
	}
	q := r.db.WithContext(ctx).
		Table("event_deliveries AS x CROSS JOIN LATERAL jsonb_array_elements_text(x.subscription_ids) AS s(sid)").
		Select("s.sid AS subscription_id, x.status AS status, count(*) AS count").
		Where("x.ou_id = ? AND x.created_at >= ? AND s.sid IN ?", ouID, since, wanted)
	var rows []subscriptionCountRow
	if err := located(q, loc).Group("s.sid, x.status").Scan(&rows).Error; err != nil {
		return nil, err
	}
	for _, row := range rows {
		id, err := uuid.Parse(row.SubscriptionID)
		if err != nil {
			continue
		}
		if out[id] == nil {
			out[id] = map[string]int{}
		}
		out[id][row.Status] += row.Count
	}
	return out, nil
}
