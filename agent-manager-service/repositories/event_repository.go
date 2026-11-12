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
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/db/dberr"
	"github.com/wso2/agent-manager/agent-manager-service/db/dbx"
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

// DeliveryFilter selects a page of a delivery log, newest first.
type DeliveryFilter struct {
	OUID           string
	Location       DeliveryLocation
	DestinationID  *uuid.UUID
	SubscriptionID *uuid.UUID
	Status         string
	Limit          int
	Offset         int
}

// SubscriptionCounts holds, per subscription, its deliveries by status.
type SubscriptionCounts map[uuid.UUID]map[string]int

// EventRepository persists event destinations, event subscriptions and the
// delivery log.
//
//go:generate moq -rm -fmt goimports -skip-ensure -pkg repomocks -out repomocks/event_repository_mock.go . EventRepository:EventRepositoryMock
type EventRepository interface {
	MonitorAlertWants(ctx context.Context, ouID string, alertID, destinationID uuid.UUID, project, agent string) (bool, error)
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

	// RecordQueuedDelivery creates the pending row of a newly queued
	// delivery. It never overwrites an existing row, so a fan-out repeated
	// after the delivery already ran cannot turn "delivered" back into
	// "pending".
	RecordQueuedDelivery(ctx context.Context, d *models.EventDelivery) error
	// ClaimDelivery leases a pending delivery to one worker for lease, so a
	// redelivered job cannot send it while another worker is. It reports
	// false when the delivery is no longer pending or is claimed already.
	ClaimDelivery(ctx context.Context, id uuid.UUID, lease time.Duration) (bool, error)
	// RecordDelivery records the outcome of a delivery attempt and releases
	// its claim. A delivery already delivered or skipped is left as it is.
	RecordDelivery(ctx context.Context, d *models.EventDelivery) error
	// GetDelivery returns the delivery of an event to a destination, or
	// gorm.ErrRecordNotFound.
	GetDelivery(ctx context.Context, destinationID uuid.UUID, eventID string) (*models.EventDelivery, error)
	// ListDeliveries returns a page of deliveries and the number matching
	// the filter.
	ListDeliveries(ctx context.Context, f DeliveryFilter) ([]models.EventDelivery, int64, error)
	// PruneDeliveries deletes deliveries queued before cutoff.
	PruneDeliveries(ctx context.Context, cutoff time.Time) (int64, error)
	// FailStaleDeliveries marks as failed, with reason, the deliveries still
	// pending and unclaimed with no activity since cutoff: their job is gone.
	FailStaleDeliveries(ctx context.Context, cutoff time.Time, reason string) (int64, error)
}

type eventRepository struct {
	db *gorm.DB
}

// NewEventRepository creates an EventRepository.
func NewEventRepository(db *gorm.DB) EventRepository {
	return &eventRepository{db: db}
}

// Child rows: a destination's projects, a subscription's event types and environments, a delivery's subscriptions.

type destinationProject struct {
	DestinationID uuid.UUID `gorm:"column:destination_id;primaryKey"`
	ProjectID     uuid.UUID `gorm:"column:project_id;primaryKey"`
	OrgID         string    `gorm:"column:org_id"`
}

func (destinationProject) TableName() string { return "event_destination_projects" }

type subscriptionType struct {
	SubscriptionID uuid.UUID `gorm:"column:subscription_id;primaryKey"`
	EventType      string    `gorm:"column:event_type;primaryKey"`
	OrgID          string    `gorm:"column:org_id"`
}

func (subscriptionType) TableName() string { return "event_subscription_types" }

type subscriptionEnvironment struct {
	SubscriptionID uuid.UUID `gorm:"column:subscription_id;primaryKey"`
	EnvironmentID  uuid.UUID `gorm:"column:environment_id;primaryKey"`
	OrgID          string    `gorm:"column:org_id"`
}

func (subscriptionEnvironment) TableName() string { return "event_subscription_environments" }

type deliverySubscription struct {
	DeliveryID     uuid.UUID `gorm:"column:delivery_id;primaryKey"`
	SubscriptionID uuid.UUID `gorm:"column:subscription_id;primaryKey"`
}

func (deliverySubscription) TableName() string { return "event_delivery_subscriptions" }

// eventDestinationSecretOwner is the owner_kind of a destination's secret.
const eventDestinationSecretOwner = "event_destination"

// projectIDQuery selects the id of the project named name in ouID, for a WHERE clause.
func projectIDQuery(db *gorm.DB, ouID, name string) *gorm.DB {
	return db.Session(&gorm.Session{NewDB: true}).Table("projects").Select("id").Where("org_id = ? AND name = ?", ouID, name)
}

// namesByID reads the name of each id in table.
func namesByID(db *gorm.DB, table string, ids []uuid.UUID) (map[uuid.UUID]string, error) {
	out := make(map[uuid.UUID]string, len(ids))
	for _, chunk := range dbx.Chunks(ids) {
		var rows []struct {
			ID   uuid.UUID
			Name string
		}
		if err := db.Table(table).Select("id, name").Where("id IN ?", chunk).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.ID] = row.Name
		}
	}
	return out, nil
}

// idsByName reads the id of each named row of the org in table; unknown names are left out.
func idsByName(db *gorm.DB, table, ouID string, names []string) (map[string]uuid.UUID, error) {
	out := make(map[string]uuid.UUID, len(names))
	for _, chunk := range dbx.Chunks(names) {
		var rows []struct {
			ID   uuid.UUID
			Name string
		}
		if err := db.Table(table).Select("id, name").Where("org_id = ? AND name IN ?", ouID, chunk).Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			out[row.Name] = row.ID
		}
	}
	return out, nil
}

// optionalID is the id named in ids, or nil.
func optionalID(ids map[string]uuid.UUID, name string) *uuid.UUID {
	if id, ok := ids[name]; ok && name != "" {
		return &id
	}
	return nil
}

// loadDestinations fills the destinations' projects and secrets.
func loadDestinations(db *gorm.DB, ds []models.EventDestination) error {
	if len(ds) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(ds))
	for i := range ds {
		ids[i] = ds[i].ID
	}
	var links []destinationProject
	for _, chunk := range dbx.Chunks(ids) {
		var part []destinationProject
		if err := db.Where("destination_id IN ?", chunk).Find(&part).Error; err != nil {
			return err
		}
		links = append(links, part...)
	}
	projectIDs := make([]uuid.UUID, 0, len(links))
	for _, l := range links {
		projectIDs = append(projectIDs, l.ProjectID)
	}
	names, err := namesByID(db, "projects", projectIDs)
	if err != nil {
		return err
	}
	byDestination := make(map[uuid.UUID][]string, len(ds))
	for _, l := range links {
		byDestination[l.DestinationID] = append(byDestination[l.DestinationID], names[l.ProjectID])
	}
	for i := range ds {
		projects := byDestination[ds[i].ID]
		sort.Strings(projects)
		if projects == nil {
			projects = []string{}
		}
		ds[i].Projects = projects
		secret, err := getSecret(db, ds[i].SecretID)
		if err != nil {
			return err
		}
		if secret != "" {
			ds[i].SecretEncrypted = []byte(secret)
		}
	}
	return nil
}

// saveDestinationProjects makes the destination's project rows exactly d.Projects.
func saveDestinationProjects(tx *gorm.DB, d *models.EventDestination) error {
	if err := tx.Where("destination_id = ?", d.ID).Delete(&destinationProject{}).Error; err != nil {
		return err
	}
	ids, err := idsByName(tx, "projects", d.OUID, d.Projects)
	if err != nil {
		return err
	}
	for _, name := range d.Projects {
		id, ok := ids[name]
		if !ok {
			return fmt.Errorf("%w: %s", utils.ErrProjectNotFound, name)
		}
		if _, err := dbx.InsertIfAbsent(tx, &destinationProject{DestinationID: d.ID, ProjectID: id, OrgID: d.OUID}); err != nil {
			return err
		}
	}
	return nil
}

func (r *eventRepository) ListDestinations(ctx context.Context, ouID string) ([]models.EventDestination, error) {
	var out []models.EventDestination
	db := r.db.WithContext(ctx)
	err := db.Where("org_id = ?", ouID).Order("LOWER(name) ASC").Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, loadDestinations(db, out)
}

func (r *eventRepository) GetDestination(ctx context.Context, ouID string, id uuid.UUID) (*models.EventDestination, error) {
	var d models.EventDestination
	db := r.db.WithContext(ctx)
	if err := db.Where("org_id = ? AND id = ?", ouID, id).Take(&d).Error; err != nil {
		return nil, err
	}
	ds := []models.EventDestination{d}
	if err := loadDestinations(db, ds); err != nil {
		return nil, err
	}
	return &ds[0], nil
}

func (r *eventRepository) DestinationsByID(ctx context.Context, ouID string, ids []uuid.UUID) ([]models.EventDestination, error) {
	out := []models.EventDestination{}
	db := r.db.WithContext(ctx)
	for _, chunk := range dbx.Chunks(ids) {
		var part []models.EventDestination
		if err := db.Where("org_id = ? AND id IN ?", ouID, chunk).Find(&part).Error; err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, loadDestinations(db, out)
}

// nameTaken reports whether another of the org's destinations has name, in any case.
func nameTaken(tx *gorm.DB, d *models.EventDestination) (bool, error) {
	var n int64
	err := tx.Model(&models.EventDestination{}).
		Where("org_id = ? AND LOWER(name) = LOWER(?) AND id <> ?", d.OUID, d.Name, d.ID).Count(&n).Error
	return n > 0, err
}

func (r *eventRepository) CreateDestination(ctx context.Context, d *models.EventDestination) error {
	now := dbx.Now()
	d.CreatedAt, d.UpdatedAt = now, now
	if d.ID == uuid.Nil {
		d.ID = newID()
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if _, err := ensureOrg(ctx, tx, d.OUID); err != nil {
			return err
		}
		if taken, err := nameTaken(tx, d); err != nil || taken {
			if taken {
				return utils.ErrEventDestinationNameTaken
			}
			return err
		}
		secretID, err := putSecret(tx, d.OUID, eventDestinationSecretOwner, d.ID, nil, string(d.SecretEncrypted))
		if err != nil {
			return err
		}
		d.SecretID = secretID
		if err := tx.Create(d).Error; err != nil {
			return err
		}
		return saveDestinationProjects(tx, d)
	})
	if dberr.IsDuplicateKey(err) {
		return utils.ErrEventDestinationNameTaken
	}
	return err
}

func (r *eventRepository) UpdateDestination(
	ctx context.Context, d *models.EventDestination, check func(using []models.EventSubscription) error,
) error {
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := dbx.LockRow(tx, "event_destinations", "name", "org_id = ? AND id = ?", d.OUID, d.ID); err != nil {
			return err
		}
		var locked models.EventDestination
		if err := tx.Where("org_id = ? AND id = ?", d.OUID, d.ID).Take(&locked).Error; err != nil {
			return err
		}
		// The caller loaded d before this transaction. A row lock alone does
		// not prevent its stale settings or secret from undoing another write.
		if !locked.UpdatedAt.Equal(d.UpdatedAt) {
			return fmt.Errorf("%w: destination changed; reload it before saving", utils.ErrConflict)
		}
		using, err := subscriptionsUsing(tx, d.OUID, d.ID)
		if err != nil {
			return err
		}
		if check != nil {
			if err := check(using); err != nil {
				return err
			}
		}
		if taken, err := nameTaken(tx, d); err != nil || taken {
			if taken {
				return utils.ErrEventDestinationNameTaken
			}
			return err
		}
		// Keep the returned revision identical to the stored one and advance it even for writes in one clock tick.
		d.UpdatedAt = dbx.Now()
		if !d.UpdatedAt.After(locked.UpdatedAt) {
			d.UpdatedAt = locked.UpdatedAt.Add(time.Microsecond)
		}
		secretID, err := putSecret(tx, d.OUID, eventDestinationSecretOwner, d.ID, locked.SecretID, string(d.SecretEncrypted))
		if err != nil {
			return err
		}
		d.SecretID = secretID
		if err := tx.Model(&models.EventDestination{}).Where("id = ?", d.ID).
			Select("name", "description", "config", "secret_id", "availability", "updated_at").
			UpdateColumns(d).Error; err != nil {
			return err
		}
		if secretID == nil {
			if err := deleteSecret(tx, locked.SecretID); err != nil {
				return err
			}
		}
		return saveDestinationProjects(tx, d)
	})
	if dberr.IsDuplicateKey(err) {
		return utils.ErrEventDestinationNameTaken
	}
	return err
}

func (r *eventRepository) DeleteDestination(ctx context.Context, ouID string, id uuid.UUID) (bool, error) {
	deleted := false
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var destination models.EventDestination
		err := tx.Where("org_id = ? AND id = ?", ouID, id).Take(&destination).Error
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		var using int64
		if err := tx.Model(&models.EventSubscription{}).Where("destination_id = ?", id).Count(&using).Error; err != nil {
			return err
		}
		if using > 0 {
			return utils.ErrEventDestinationInUse
		}
		res := tx.Where("id = ?", id).Delete(&models.EventDestination{})
		if res.Error != nil {
			return res.Error
		}
		deleted = res.RowsAffected > 0
		return deleteSecret(tx, destination.SecretID)
	})
	if dberr.IsForeignKeyViolation(err) {
		return false, utils.ErrEventDestinationInUse
	}
	return deleted, err
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
	db := r.db.WithContext(ctx)
	q := db.Model(&models.EventDestination{}).Where("org_id = ?", ouID)
	if id != nil {
		q = q.Where("id = ?", *id)
	}
	var ids []uuid.UUID
	if err := q.Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	out := make(map[uuid.UUID]DestinationStats, len(ids))
	for _, destinationID := range ids {
		var stats DestinationStats
		var usedBy int64
		if err := db.Model(&models.EventSubscription{}).Where("destination_id = ?", destinationID).
			Count(&usedBy).Error; err != nil {
			return nil, err
		}
		stats.UsedBy = int(usedBy)

		var last []models.EventDelivery
		if err := db.Select("created_at, status").
			Where("destination_id = ? AND status <> ?", destinationID, models.EventDeliverySkipped).
			Order("created_at DESC").Limit(1).Find(&last).Error; err != nil {
			return nil, err
		}
		if len(last) > 0 {
			stats.LastDeliveryAt, stats.LastStatus = &last[0].CreatedAt, last[0].Status
		}

		// Failures, and pending deliveries being retried, since the latest delivered one.
		failures := db.Model(&models.EventDelivery{}).
			Where("destination_id = ? AND (status = ? OR (status = ? AND attempts > 0))",
				destinationID, models.EventDeliveryFailed, models.EventDeliveryPending)
		var delivered []models.EventDelivery
		if err := db.Select("created_at").
			Where("destination_id = ? AND status = ?", destinationID, models.EventDeliveryDelivered).
			Order("created_at DESC").Limit(1).Find(&delivered).Error; err != nil {
			return nil, err
		}
		if len(delivered) > 0 {
			failures = failures.Where("created_at > ?", delivered[0].CreatedAt)
		}
		var n int64
		if err := failures.Count(&n).Error; err != nil {
			return nil, err
		}
		stats.ConsecutiveFailures = int(n)
		out[destinationID] = stats
	}
	return out, nil
}

// subscriptionsUsing reads the subscriptions that send to a destination, oldest first.
func subscriptionsUsing(db *gorm.DB, ouID string, destinationID uuid.UUID) ([]models.EventSubscription, error) {
	var out []models.EventSubscription
	if err := db.Where("org_id = ? AND destination_id = ?", ouID, destinationID).
		Order("created_at ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, loadSubscriptions(db, out)
}

func (r *eventRepository) SubscriptionsUsing(
	ctx context.Context, ouID string, destinationID uuid.UUID,
) ([]models.EventSubscription, error) {
	return subscriptionsUsing(r.db.WithContext(ctx), ouID, destinationID)
}

// RemoveProjectFromDestinations drops the project from the destinations made available to it. A destination left
// with no project becomes org-only: available to none, rather than silently to all.
func (r *eventRepository) RemoveProjectFromDestinations(ctx context.Context, ouID, projectName string) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return removeProjectFromDestinations(tx, ouID, projectName)
	})
}

func removeProjectFromDestinations(tx *gorm.DB, ouID, projectName string) error {
	var affected []uuid.UUID
	if err := tx.Model(&destinationProject{}).
		Where("org_id = ? AND project_id IN (?)", ouID, projectIDQuery(tx, ouID, projectName)).
		Pluck("destination_id", &affected).Error; err != nil {
		return err
	}
	if len(affected) == 0 {
		return nil
	}
	if err := tx.Where("org_id = ? AND project_id IN (?)", ouID, projectIDQuery(tx, ouID, projectName)).
		Delete(&destinationProject{}).Error; err != nil {
		return err
	}
	now := dbx.Now()
	for _, id := range affected {
		var left int64
		if err := tx.Model(&destinationProject{}).Where("destination_id = ?", id).Count(&left).Error; err != nil {
			return err
		}
		updates := map[string]any{"updated_at": now}
		if left == 0 {
			updates["availability"] = models.DestinationAvailableOrg
		}
		if err := tx.Model(&models.EventDestination{}).
			Where("id = ? AND availability = ?", id, models.DestinationAvailableProjects).
			Updates(updates).Error; err != nil {
			return err
		}
	}
	return nil
}

// loadSubscriptions fills the subscriptions' project and agent names, event types and environments.
func loadSubscriptions(db *gorm.DB, subs []models.EventSubscription) error {
	if len(subs) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(subs))
	var projectIDs, agentIDs []uuid.UUID
	for i, s := range subs {
		ids[i] = s.ID
		if s.ProjectID != nil {
			projectIDs = append(projectIDs, *s.ProjectID)
		}
		if s.AgentUUID != nil {
			agentIDs = append(agentIDs, *s.AgentUUID)
		}
	}
	projects, err := namesByID(db, "projects", projectIDs)
	if err != nil {
		return err
	}
	agents, err := namesByID(db, "agents", agentIDs)
	if err != nil {
		return err
	}
	types := make(map[uuid.UUID][]string, len(subs))
	envs := make(map[uuid.UUID][]uuid.UUID, len(subs))
	var envIDs []uuid.UUID
	for _, chunk := range dbx.Chunks(ids) {
		var ts []subscriptionType
		if err := db.Where("subscription_id IN ?", chunk).Order("event_type").Find(&ts).Error; err != nil {
			return err
		}
		for _, t := range ts {
			types[t.SubscriptionID] = append(types[t.SubscriptionID], t.EventType)
		}
		var es []subscriptionEnvironment
		if err := db.Where("subscription_id IN ?", chunk).Find(&es).Error; err != nil {
			return err
		}
		for _, e := range es {
			envs[e.SubscriptionID] = append(envs[e.SubscriptionID], e.EnvironmentID)
			envIDs = append(envIDs, e.EnvironmentID)
		}
	}
	envNames, err := namesByID(db, "environments", envIDs)
	if err != nil {
		return err
	}
	for i := range subs {
		s := &subs[i]
		if s.ProjectID != nil {
			s.ProjectName = projects[*s.ProjectID]
		}
		if s.AgentUUID != nil {
			s.AgentName = agents[*s.AgentUUID]
		}
		s.EventTypes = append([]string{}, types[s.ID]...)
		s.Environments = []string{}
		for _, id := range envs[s.ID] {
			s.Environments = append(s.Environments, envNames[id])
		}
		sort.Strings(s.Environments)
	}
	return nil
}

// resolveSubscription sets the subscription's project and agent ids from its names, as its scope needs.
func resolveSubscription(ctx context.Context, tx *gorm.DB, s *models.EventSubscription) error {
	s.ProjectID, s.AgentUUID = nil, nil
	switch s.Scope {
	case models.EventScopeProject:
		orgID, err := orgIDFor(ctx, tx, s.OUID)
		if err != nil {
			return err
		}
		id, err := projectIDFor(ctx, tx, orgID, s.ProjectName)
		if err != nil {
			return err
		}
		s.ProjectID = &id
	case models.EventScopeAgent:
		scope, err := agentScopeFor(ctx, tx, s.OUID, s.ProjectName, s.AgentName)
		if err != nil {
			return err
		}
		s.ProjectID, s.AgentUUID = &scope.ProjectID, &scope.AgentID
	}
	return nil
}

// saveSubscriptionChildren makes the subscription's event type and environment rows exactly its lists.
func saveSubscriptionChildren(tx *gorm.DB, s *models.EventSubscription) error {
	if err := tx.Where("subscription_id = ?", s.ID).Delete(&subscriptionType{}).Error; err != nil {
		return err
	}
	if err := tx.Where("subscription_id = ?", s.ID).Delete(&subscriptionEnvironment{}).Error; err != nil {
		return err
	}
	for _, t := range s.EventTypes {
		if _, err := dbx.InsertIfAbsent(tx, &subscriptionType{SubscriptionID: s.ID, EventType: t, OrgID: s.OUID}); err != nil {
			return err
		}
	}
	envs, err := idsByName(tx, "environments", s.OUID, s.Environments)
	if err != nil {
		return err
	}
	for _, name := range s.Environments {
		id, ok := envs[name]
		if !ok {
			return fmt.Errorf("%w: %s", utils.ErrEnvironmentNotFound, name)
		}
		if _, err := dbx.InsertIfAbsent(tx, &subscriptionEnvironment{SubscriptionID: s.ID, EnvironmentID: id, OrgID: s.OUID}); err != nil {
			return err
		}
	}
	return nil
}

func (r *eventRepository) owned(ctx context.Context, t EventTarget) *gorm.DB {
	db := r.db.WithContext(ctx)
	q := db.Where("org_id = ? AND scope = ?", t.OUID, t.Scope)
	switch t.Scope {
	case models.EventScopeProject:
		q = q.Where("project_id IN (?)", projectIDQuery(db, t.OUID, t.ProjectName))
	case models.EventScopeAgent:
		q = q.Where("agent_id IN (?)", models.AgentIDQuery(db, t.OUID, t.ProjectName, t.AgentName))
	}
	return q
}

func (r *eventRepository) ListSubscriptions(ctx context.Context, t EventTarget) ([]models.EventSubscription, error) {
	var out []models.EventSubscription
	if err := r.owned(ctx, t).Order("created_at ASC").Find(&out).Error; err != nil {
		return nil, err
	}
	return out, loadSubscriptions(r.db.WithContext(ctx), out)
}

func (r *eventRepository) GetSubscription(ctx context.Context, t EventTarget, id uuid.UUID) (*models.EventSubscription, error) {
	var s models.EventSubscription
	if err := r.owned(ctx, t).Where("id = ?", id).Take(&s).Error; err != nil {
		return nil, err
	}
	subs := []models.EventSubscription{s}
	if err := loadSubscriptions(r.db.WithContext(ctx), subs); err != nil {
		return nil, err
	}
	return &subs[0], nil
}

func (r *eventRepository) SubscriptionsByID(ctx context.Context, ouID string, ids []uuid.UUID) ([]models.EventSubscription, error) {
	out := []models.EventSubscription{}
	db := r.db.WithContext(ctx)
	for _, chunk := range dbx.Chunks(ids) {
		var part []models.EventSubscription
		if err := db.Where("org_id = ? AND id IN ?", ouID, chunk).Find(&part).Error; err != nil {
			return nil, err
		}
		out = append(out, part...)
	}
	return out, loadSubscriptions(db, out)
}

// withDestination runs save inside a transaction that first locks s's destination, so its availability cannot
// change meanwhile, and passes it to check.
func (r *eventRepository) withDestination(
	ctx context.Context, s *models.EventSubscription, check func(*models.EventDestination) error, save func(tx *gorm.DB) error,
) error {
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := dbx.LockRow(tx, "event_destinations", "name", "org_id = ? AND id = ?", s.OUID, s.DestinationID); err != nil {
			return err
		}
		var d models.EventDestination
		if err := tx.Where("org_id = ? AND id = ?", s.OUID, s.DestinationID).Take(&d).Error; err != nil {
			return err
		}
		ds := []models.EventDestination{d}
		if err := loadDestinations(tx, ds); err != nil {
			return err
		}
		if check != nil {
			if err := check(&ds[0]); err != nil {
				return err
			}
		}
		if err := resolveSubscription(ctx, tx, s); err != nil {
			return err
		}
		return save(tx)
	})
}

func (r *eventRepository) CreateSubscription(
	ctx context.Context, s *models.EventSubscription, check func(*models.EventDestination) error,
) error {
	return r.withDestination(ctx, s, check, func(tx *gorm.DB) error {
		now := dbx.Now()
		s.CreatedAt, s.UpdatedAt = now, now
		if s.ID == uuid.Nil {
			s.ID = newID()
		}
		if err := tx.Create(s).Error; err != nil {
			return err
		}
		return saveSubscriptionChildren(tx, s)
	})
}

func (r *eventRepository) UpdateSubscription(
	ctx context.Context, s *models.EventSubscription, check func(*models.EventDestination) error,
) error {
	return r.withDestination(ctx, s, check, func(tx *gorm.DB) error {
		s.UpdatedAt = dbx.Now()
		if err := tx.Model(&models.EventSubscription{}).Where("id = ?", s.ID).
			Select("name", "description", "destination_id", "enabled", "updated_at").
			Updates(s).Error; err != nil {
			return err
		}
		return saveSubscriptionChildren(tx, s)
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
	db := r.db.WithContext(ctx)
	q := db.Where("org_id = ?", ouID)
	if agentName != "" {
		q = q.Where("agent_id IN (?)", models.AgentIDQuery(db, ouID, projectName, agentName))
	} else {
		q = q.Where("project_id IN (?)", projectIDQuery(db, ouID, projectName))
	}
	return q.Delete(&models.EventSubscription{}).Error
}

func (r *eventRepository) CandidateSubscriptions(
	ctx context.Context, ouID, projectName, agentName string,
) ([]models.EventSubscription, error) {
	var out []models.EventSubscription
	db := r.db.WithContext(ctx)
	err := db.
		Where("org_id = ? AND enabled = ?", ouID, true).
		Where(db.Session(&gorm.Session{NewDB: true}).Where("scope = ?", models.EventScopeOrg).
			Or("scope = ? AND project_id IN (?)", models.EventScopeProject, projectIDQuery(db, ouID, projectName)).
			Or("scope = ? AND agent_id IN (?)", models.EventScopeAgent, models.AgentIDQuery(db, ouID, projectName, agentName))).
		Find(&out).Error
	if err != nil {
		return nil, err
	}
	return out, loadSubscriptions(db, out)
}

// located narrows q, on event_deliveries aliased x, to loc.
func located(db, q *gorm.DB, ouID string, loc DeliveryLocation) *gorm.DB {
	if loc.ProjectName != "" {
		q = q.Where("x.project_id IN (?)", projectIDQuery(db, ouID, loc.ProjectName))
		if loc.AgentName != "" {
			q = q.Where("x.agent_id IN (?)", models.AgentIDQuery(db, ouID, loc.ProjectName, loc.AgentName))
		}
	}
	if len(loc.EventScopes) > 0 {
		q = q.Where("x.event_scope IN ?", loc.EventScopes)
	}
	return q
}

// subscriptionCountRow is one row of the subscription counts query.
type subscriptionCountRow struct {
	SubscriptionID uuid.UUID
	Status         string
	Count          int
}

func (r *eventRepository) SubscriptionCounts(
	ctx context.Context, ouID string, ids []uuid.UUID, since time.Time, loc DeliveryLocation,
) (SubscriptionCounts, error) {
	out := SubscriptionCounts{}
	db := r.db.WithContext(ctx)
	for _, chunk := range dbx.Chunks(ids) {
		q := db.Table("event_deliveries x").
			Joins("JOIN event_delivery_subscriptions s ON s.delivery_id = x.id").
			Select("s.subscription_id AS subscription_id, x.status AS status, COUNT(*) AS count").
			Where("x.org_id = ? AND x.created_at >= ? AND s.subscription_id IN ?", ouID, since.UTC(), chunk)
		var rows []subscriptionCountRow
		if err := located(db, q, ouID, loc).Group("s.subscription_id, x.status").Scan(&rows).Error; err != nil {
			return nil, err
		}
		for _, row := range rows {
			if out[row.SubscriptionID] == nil {
				out[row.SubscriptionID] = map[string]int{}
			}
			out[row.SubscriptionID][row.Status] += row.Count
		}
	}
	return out, nil
}

// resolveDelivery sets the delivery's location ids from its names; a project, agent or environment that is gone is
// left unset.
func resolveDelivery(tx *gorm.DB, d *models.EventDelivery) error {
	d.ProjectID, d.AgentUUID, d.EnvironmentID = nil, nil, nil
	if d.ProjectName != "" {
		projects, err := idsByName(tx, "projects", d.OUID, []string{d.ProjectName})
		if err != nil {
			return err
		}
		d.ProjectID = optionalID(projects, d.ProjectName)
		if d.AgentName != "" {
			id, err := models.AgentIDByNames(tx, d.OUID, d.ProjectName, d.AgentName)
			if err != nil && !errors.Is(err, models.ErrAgentRefNotFound) {
				return err
			}
			if err == nil {
				d.AgentUUID = &id
			}
		}
	}
	if d.Environment != "" {
		envs, err := idsByName(tx, "environments", d.OUID, []string{d.Environment})
		if err != nil {
			return err
		}
		d.EnvironmentID = optionalID(envs, d.Environment)
	}
	return nil
}

// insertDelivery inserts the delivery and its subscription rows unless it exists, and reports whether it did.
func insertDelivery(tx *gorm.DB, d *models.EventDelivery) (bool, error) {
	if d.ID == uuid.Nil {
		d.ID = newID()
	}
	if err := resolveDelivery(tx, d); err != nil {
		return false, err
	}
	inserted, err := dbx.InsertIfAbsent(tx, d)
	if err != nil || !inserted {
		return inserted, err
	}
	for _, sid := range d.SubscriptionIDs {
		id, err := uuid.Parse(sid)
		if err != nil {
			continue
		}
		if _, err := dbx.InsertIfAbsent(tx, &deliverySubscription{DeliveryID: d.ID, SubscriptionID: id}); err != nil {
			return false, err
		}
	}
	return true, nil
}

func (r *eventRepository) RecordQueuedDelivery(ctx context.Context, d *models.EventDelivery) error {
	d.UpdatedAt = dbx.Now()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = d.UpdatedAt
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		_, err := insertDelivery(tx, d)
		return err
	})
}

func (r *eventRepository) ClaimDelivery(ctx context.Context, id uuid.UUID, lease time.Duration) (bool, error) {
	now := dbx.Now()
	res := r.db.WithContext(ctx).Model(&models.EventDelivery{}).
		Where("id = ? AND status = ? AND (claimed_until IS NULL OR claimed_until < ?)", id, models.EventDeliveryPending, now).
		Update("claimed_until", now.Add(lease))
	return res.RowsAffected == 1, res.Error
}

// RecordDelivery writes the attempt's outcome. A late worker, one whose job was redelivered say, must not turn a
// finished delivery back into pending or failed.
func (r *eventRepository) RecordDelivery(ctx context.Context, d *models.EventDelivery) error {
	d.UpdatedAt = dbx.Now()
	if d.CreatedAt.IsZero() {
		d.CreatedAt = d.UpdatedAt
	}
	d.ClaimedUntil = nil
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		inserted, err := insertDelivery(tx, d)
		if err != nil || inserted {
			return err
		}
		return tx.Model(&models.EventDelivery{}).
			Where("destination_id = ? AND event_id = ? AND status NOT IN ?", d.DestinationID, d.EventID,
				[]string{models.EventDeliveryDelivered, models.EventDeliverySkipped}).
			Updates(map[string]any{
				"status": d.Status, "attempts": d.Attempts, "response_code": d.ResponseCode, "last_error": d.LastError,
				"delivered_at": d.DeliveredAt, "claimed_until": nil, "updated_at": d.UpdatedAt,
			}).Error
	})
}

// loadDeliveries fills the deliveries' location names and subscription ids.
func loadDeliveries(db *gorm.DB, ds []models.EventDelivery) error {
	if len(ds) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(ds))
	var projectIDs, agentIDs, envIDs []uuid.UUID
	for i, d := range ds {
		ids[i] = d.ID
		if d.ProjectID != nil {
			projectIDs = append(projectIDs, *d.ProjectID)
		}
		if d.AgentUUID != nil {
			agentIDs = append(agentIDs, *d.AgentUUID)
		}
		if d.EnvironmentID != nil {
			envIDs = append(envIDs, *d.EnvironmentID)
		}
	}
	projects, err := namesByID(db, "projects", projectIDs)
	if err != nil {
		return err
	}
	agents, err := namesByID(db, "agents", agentIDs)
	if err != nil {
		return err
	}
	envs, err := namesByID(db, "environments", envIDs)
	if err != nil {
		return err
	}
	subs := make(map[uuid.UUID][]string, len(ds))
	for _, chunk := range dbx.Chunks(ids) {
		var links []deliverySubscription
		if err := db.Where("delivery_id IN ?", chunk).Find(&links).Error; err != nil {
			return err
		}
		for _, l := range links {
			subs[l.DeliveryID] = append(subs[l.DeliveryID], l.SubscriptionID.String())
		}
	}
	for i := range ds {
		d := &ds[i]
		if d.ProjectID != nil {
			d.ProjectName = projects[*d.ProjectID]
		}
		if d.AgentUUID != nil {
			d.AgentName = agents[*d.AgentUUID]
		}
		if d.EnvironmentID != nil {
			d.Environment = envs[*d.EnvironmentID]
		}
		d.SubscriptionIDs = append([]string{}, subs[d.ID]...)
		sort.Strings(d.SubscriptionIDs)
	}
	return nil
}

func (r *eventRepository) GetDelivery(ctx context.Context, destinationID uuid.UUID, eventID string) (*models.EventDelivery, error) {
	var d models.EventDelivery
	db := r.db.WithContext(ctx)
	if err := db.Where("destination_id = ? AND event_id = ?", destinationID, eventID).Take(&d).Error; err != nil {
		return nil, err
	}
	ds := []models.EventDelivery{d}
	if err := loadDeliveries(db, ds); err != nil {
		return nil, err
	}
	return &ds[0], nil
}

func (r *eventRepository) ListDeliveries(ctx context.Context, f DeliveryFilter) ([]models.EventDelivery, int64, error) {
	db := r.db.WithContext(ctx)
	// Built afresh for the count and the page: a gorm query reused after
	// Count keeps the count's select.
	filtered := func() *gorm.DB {
		q := located(db, db.Table("event_deliveries x").Where("x.org_id = ?", f.OUID), f.OUID, f.Location)
		if f.DestinationID != nil {
			q = q.Where("x.destination_id = ?", *f.DestinationID)
		}
		if f.SubscriptionID != nil {
			q = q.Where("x.id IN (?)", db.Session(&gorm.Session{NewDB: true}).Model(&deliverySubscription{}).
				Select("delivery_id").Where("subscription_id = ?", *f.SubscriptionID))
		}
		if f.Status != "" {
			q = q.Where("x.status = ?", f.Status)
		}
		return q
	}
	var total int64
	if err := filtered().Count(&total).Error; err != nil {
		return nil, 0, err
	}
	var out []models.EventDelivery
	err := filtered().Select("x.*").Order("x.created_at DESC, x.id DESC").Limit(f.Limit).Offset(f.Offset).Find(&out).Error
	if err != nil {
		return nil, 0, err
	}
	return out, total, loadDeliveries(db, out)
}

func (r *eventRepository) FailStaleDeliveries(ctx context.Context, cutoff time.Time, reason string) (int64, error) {
	now := dbx.Now()
	cutoff = cutoff.UTC()
	// created_at never follows updated_at, so it narrows the scan through its index.
	res := r.db.WithContext(ctx).Model(&models.EventDelivery{}).
		Where("status = ? AND created_at < ? AND updated_at < ? AND (claimed_until IS NULL OR claimed_until < ?)",
			models.EventDeliveryPending, cutoff, cutoff, now).
		Updates(map[string]any{
			"status": models.EventDeliveryFailed, "last_error": reason, "claimed_until": nil, "updated_at": now,
		})
	return res.RowsAffected, res.Error
}

func (r *eventRepository) PruneDeliveries(ctx context.Context, cutoff time.Time) (int64, error) {
	res := r.db.WithContext(ctx).Where("created_at < ?", cutoff.UTC()).Delete(&models.EventDelivery{})
	return res.RowsAffected, res.Error
}

// MonitorAlertWants is always false: Lite runs no monitors, so no alert sends to a destination.
func (r *eventRepository) MonitorAlertWants(context.Context, string, uuid.UUID, uuid.UUID, string, string) (bool, error) {
	return false, nil
}
