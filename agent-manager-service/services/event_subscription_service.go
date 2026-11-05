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

package services

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/audit"
	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/events"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/repositories"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

const (
	maxEventSubscriptionsPerOwner = 25
	// subscriptionCountsWindow is how far back a subscription's delivery
	// counts look.
	subscriptionCountsWindow = 24 * time.Hour
	// eventCleanupTimeout bounds each cleanup step after a project or agent
	// delete, which no longer has the request's deadline.
	eventCleanupTimeout = 30 * time.Second
)

// EventSubscriptionInput is the input for creating or updating a
// subscription.
type EventSubscriptionInput struct {
	Name          string
	Description   string
	DestinationID uuid.UUID
	// EventTypes are catalog types the subscription's scope can receive, or
	// just events.AllEvents.
	EventTypes []string
	// Environments limits the subscription to events in these environments;
	// empty selects every environment.
	Environments []string
	Enabled      bool
}

// SubscriptionList is what a level shows: the subscriptions it owns, those of
// wider levels that reach it, the destinations they send to, and their
// deliveries of the last 24 hours by status.
type SubscriptionList struct {
	Own []models.EventSubscription
	// Inherited are the wider levels' subscriptions that can send this
	// level's events, read-only here, with EventTypes narrowed to the types
	// that can reach this level.
	Inherited    []models.EventSubscription
	Destinations map[uuid.UUID]*models.EventDestination
	// DestinationStats holds the stats of the org's destinations.
	DestinationStats map[uuid.UUID]repositories.DestinationStats
	// Counts are per subscription. An inherited subscription counts only the
	// deliveries of this level's events.
	Counts repositories.SubscriptionCounts
}

// SubscriptionDetail is what one of a level's own subscriptions shows: its
// destination, if it still exists, the destination's stats, and its
// deliveries of the last 24 hours by status.
type SubscriptionDetail struct {
	Destination      *models.EventDestination
	DestinationStats repositories.DestinationStats
	Counts           map[string]int
}

// EventsCleaner removes the subscriptions of a deleted project or agent, so
// one created later under the same name does not inherit them.
type EventsCleaner interface {
	RemoveForProject(ctx context.Context, ouID, projectName string)
	RemoveForAgent(ctx context.Context, ouID, projectName, agentName string)
}

// EventSubscriptionService manages event subscriptions at org, project and
// agent level. Every change is recorded fail-closed in the audit trail before
// it applies.
type EventSubscriptionService interface {
	// EventTypes lists the event types a subscription at scope can name; all
	// of them for an empty scope.
	EventTypes(scope string) ([]events.Type, error)
	List(ctx context.Context, target repositories.EventTarget) (*SubscriptionList, error)
	Get(ctx context.Context, target repositories.EventTarget, id uuid.UUID) (*models.EventSubscription, error)
	// Describe loads what one of target's own subscriptions shows, without
	// the rest of the level.
	Describe(ctx context.Context, target repositories.EventTarget, sub *models.EventSubscription) (*SubscriptionDetail, error)
	Create(ctx context.Context, target repositories.EventTarget, in EventSubscriptionInput) (*models.EventSubscription, error)
	Update(ctx context.Context, target repositories.EventTarget, id uuid.UUID, in EventSubscriptionInput) (*models.EventSubscription, error)
	Delete(ctx context.Context, target repositories.EventTarget, id uuid.UUID) error

	EventsCleaner
}

type eventSubscriptionService struct {
	repo   repositories.EventRepository
	oc     client.OpenChoreoClient
	logger *slog.Logger
	now    func() time.Time
}

// NewEventSubscriptionService creates an EventSubscriptionService.
func NewEventSubscriptionService(
	repo repositories.EventRepository, oc client.OpenChoreoClient, logger *slog.Logger,
) EventSubscriptionService {
	return &eventSubscriptionService{repo: repo, oc: oc, logger: logger, now: time.Now}
}

func (s *eventSubscriptionService) EventTypes(scope string) ([]events.Type, error) {
	if scope == "" {
		return events.Types(""), nil
	}
	if !events.Scope(scope).Valid() {
		return nil, fmt.Errorf("%w: scope must be org, project or agent", utils.ErrInvalidInput)
	}
	return events.Subscribable(events.Scope(scope)), nil
}

// widerTargets lists the levels above t whose subscriptions t inherits,
// nearest first.
func widerTargets(t repositories.EventTarget) []repositories.EventTarget {
	org := repositories.EventTarget{OUID: t.OUID, Scope: models.EventScopeOrg}
	switch t.Scope {
	case models.EventScopeProject:
		return []repositories.EventTarget{org}
	case models.EventScopeAgent:
		project := repositories.EventTarget{OUID: t.OUID, Scope: models.EventScopeProject, ProjectName: t.ProjectName}
		return []repositories.EventTarget{project, org}
	default:
		return nil
	}
}

// levelLocation narrows deliveries to the events of t's level: an org's
// subscriptions see every event, a project's those of the project and its
// agents, an agent's those of the agent. An org event that names the
// project, like project.deleted, is not the project's.
func levelLocation(t repositories.EventTarget) repositories.DeliveryLocation {
	switch t.Scope {
	case models.EventScopeProject:
		return repositories.DeliveryLocation{
			ProjectName: t.ProjectName,
			EventScopes: []string{models.EventScopeProject, models.EventScopeAgent},
		}
	case models.EventScopeAgent:
		return repositories.DeliveryLocation{
			ProjectName: t.ProjectName,
			AgentName:   t.AgentName,
			EventScopes: []string{models.EventScopeAgent},
		}
	default:
		return repositories.DeliveryLocation{}
	}
}

func (s *eventSubscriptionService) List(ctx context.Context, t repositories.EventTarget) (*SubscriptionList, error) {
	own, err := s.repo.ListSubscriptions(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("failed to list event subscriptions: %w", err)
	}
	out := &SubscriptionList{Own: own, Inherited: []models.EventSubscription{}}
	for _, wider := range widerTargets(t) {
		subs, err := s.repo.ListSubscriptions(ctx, wider)
		if err != nil {
			return nil, fmt.Errorf("failed to list inherited event subscriptions: %w", err)
		}
		for i := range subs {
			types := typesReaching(subs[i].EventTypes, events.Scope(t.Scope))
			if len(types) == 0 {
				continue // none of its events can happen at this level
			}
			subs[i].EventTypes = types
			out.Inherited = append(out.Inherited, subs[i])
		}
	}

	ids := make(map[uuid.UUID]bool)
	for i := range out.Own {
		ids[out.Own[i].DestinationID] = true
	}
	for i := range out.Inherited {
		ids[out.Inherited[i].DestinationID] = true
	}
	if out.Destinations, err = s.destinations(ctx, t.OUID, ids); err != nil {
		return nil, err
	}
	if out.DestinationStats, err = s.repo.DestinationStats(ctx, t.OUID); err != nil {
		return nil, fmt.Errorf("failed to read event destination stats: %w", err)
	}

	since := s.now().Add(-subscriptionCountsWindow)
	ownIDs := make([]uuid.UUID, len(out.Own))
	for i := range out.Own {
		ownIDs[i] = out.Own[i].ID
	}
	if out.Counts, err = s.repo.SubscriptionCounts(ctx, t.OUID, ownIDs, since, repositories.DeliveryLocation{}); err != nil {
		return nil, fmt.Errorf("failed to count event deliveries: %w", err)
	}
	if len(out.Inherited) > 0 {
		inheritedIDs := make([]uuid.UUID, len(out.Inherited))
		for i := range out.Inherited {
			inheritedIDs[i] = out.Inherited[i].ID
		}
		counts, err := s.repo.SubscriptionCounts(ctx, t.OUID, inheritedIDs, since, levelLocation(t))
		if err != nil {
			return nil, fmt.Errorf("failed to count event deliveries: %w", err)
		}
		for id, byStatus := range counts {
			out.Counts[id] = byStatus
		}
	}
	return out, nil
}

// destinations loads the org's destinations among ids, keyed by ID.
func (s *eventSubscriptionService) destinations(
	ctx context.Context, ouID string, ids map[uuid.UUID]bool,
) (map[uuid.UUID]*models.EventDestination, error) {
	list := make([]uuid.UUID, 0, len(ids))
	for id := range ids {
		list = append(list, id)
	}
	found, err := s.repo.DestinationsByID(ctx, ouID, list)
	if err != nil {
		return nil, fmt.Errorf("failed to load event destinations: %w", err)
	}
	out := make(map[uuid.UUID]*models.EventDestination, len(found))
	for i := range found {
		out[found[i].ID] = &found[i]
	}
	return out, nil
}

func (s *eventSubscriptionService) Get(
	ctx context.Context, t repositories.EventTarget, id uuid.UUID,
) (*models.EventSubscription, error) {
	sub, err := s.repo.GetSubscription(ctx, t, id)
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, utils.ErrEventSubscriptionNotFound
		}
		return nil, fmt.Errorf("failed to get event subscription: %w", err)
	}
	return sub, nil
}

func (s *eventSubscriptionService) Describe(
	ctx context.Context, t repositories.EventTarget, sub *models.EventSubscription,
) (*SubscriptionDetail, error) {
	out := &SubscriptionDetail{}
	d, err := s.repo.GetDestination(ctx, t.OUID, sub.DestinationID)
	switch {
	case err == nil:
		out.Destination = d
		if out.DestinationStats, err = s.repo.DestinationStatsOf(ctx, t.OUID, d.ID); err != nil {
			return nil, fmt.Errorf("failed to read event destination stats: %w", err)
		}
	case !errors.Is(err, gorm.ErrRecordNotFound):
		return nil, fmt.Errorf("failed to load event destination: %w", err)
	}
	counts, err := s.repo.SubscriptionCounts(ctx, t.OUID, []uuid.UUID{sub.ID},
		s.now().Add(-subscriptionCountsWindow), repositories.DeliveryLocation{})
	if err != nil {
		return nil, fmt.Errorf("failed to count event deliveries: %w", err)
	}
	out.Counts = counts[sub.ID]
	return out, nil
}

func (s *eventSubscriptionService) Create(
	ctx context.Context, t repositories.EventTarget, in EventSubscriptionInput,
) (*models.EventSubscription, error) {
	if err := s.validate(t, &in); err != nil {
		return nil, err
	}
	if err := s.checkTarget(ctx, t, in.Environments); err != nil {
		return nil, err
	}
	existing, err := s.repo.ListSubscriptions(ctx, t)
	if err != nil {
		return nil, fmt.Errorf("failed to list event subscriptions: %w", err)
	}
	if len(existing) >= maxEventSubscriptionsPerOwner {
		return nil, fmt.Errorf("%w: at most %d event subscriptions are allowed here",
			utils.ErrInvalidInput, maxEventSubscriptionsPerOwner)
	}
	sub := &models.EventSubscription{
		ID:            uuid.New(),
		OUID:          t.OUID,
		Scope:         t.Scope,
		ProjectName:   t.ProjectName,
		AgentName:     t.AgentName,
		Name:          in.Name,
		Description:   in.Description,
		DestinationID: in.DestinationID,
		EventTypes:    in.EventTypes,
		Environments:  in.Environments,
		Enabled:       in.Enabled,
		CreatedBy:     audit.BuildEvent(ctx, audit.ActionEventSubscriptionCreate).ActorID,
	}
	attempt, err := audit.Begin(ctx, audit.ActionEventSubscriptionCreate, subscriptionAuditOpts(t, sub)...)
	if err != nil {
		return nil, err
	}
	err = s.repo.CreateSubscription(ctx, sub, destinationCheck(t))
	if err != nil {
		err = subscriptionSaveError(err, "failed to create event subscription")
		attempt.Complete(ctx, err)
		return nil, err
	}
	attempt.Complete(ctx, nil)
	return sub, nil
}

func (s *eventSubscriptionService) Update(
	ctx context.Context, t repositories.EventTarget, id uuid.UUID, in EventSubscriptionInput,
) (*models.EventSubscription, error) {
	sub, err := s.Get(ctx, t, id)
	if err != nil {
		return nil, err
	}
	if err := s.validate(t, &in); err != nil {
		return nil, err
	}
	if err := s.checkTarget(ctx, t, in.Environments); err != nil {
		return nil, err
	}
	sub.Name, sub.Description, sub.DestinationID = in.Name, in.Description, in.DestinationID
	sub.EventTypes, sub.Environments, sub.Enabled = in.EventTypes, in.Environments, in.Enabled
	attempt, err := audit.Begin(ctx, audit.ActionEventSubscriptionUpdate, subscriptionAuditOpts(t, sub)...)
	if err != nil {
		return nil, err
	}
	if err := s.repo.UpdateSubscription(ctx, sub, destinationCheck(t)); err != nil {
		err = subscriptionSaveError(err, "failed to update event subscription")
		attempt.Complete(ctx, err)
		return nil, err
	}
	attempt.Complete(ctx, nil)
	return sub, nil
}

func (s *eventSubscriptionService) Delete(ctx context.Context, t repositories.EventTarget, id uuid.UUID) error {
	sub, err := s.Get(ctx, t, id)
	if err != nil {
		return err
	}
	attempt, err := audit.Begin(ctx, audit.ActionEventSubscriptionDelete, subscriptionAuditOpts(t, sub)...)
	if err != nil {
		return err
	}
	deleted, err := s.repo.DeleteSubscription(ctx, t, id)
	switch {
	case err != nil:
		err = fmt.Errorf("failed to delete event subscription: %w", err)
	case !deleted:
		err = utils.ErrEventSubscriptionNotFound
	}
	attempt.Complete(ctx, err)
	return err
}

func (s *eventSubscriptionService) RemoveForProject(ctx context.Context, ouID, projectName string) {
	if err := cleanupStep(ctx, func(ctx context.Context) error {
		return s.repo.DeleteSubscriptionsUnder(ctx, ouID, projectName, "")
	}); err != nil {
		s.logger.Error("Failed to remove the event subscriptions of a deleted project",
			"ouID", ouID, "project", projectName, "error", err)
	}
	if err := cleanupStep(ctx, func(ctx context.Context) error {
		return s.repo.RemoveProjectFromDestinations(ctx, ouID, projectName)
	}); err != nil {
		s.logger.Error("Failed to remove a deleted project from event destinations",
			"ouID", ouID, "project", projectName, "error", err)
	}
}

// RemoveForAgent deletes an agent's subscriptions. Errors are logged: the
// agent is already gone.
func (s *eventSubscriptionService) RemoveForAgent(ctx context.Context, ouID, projectName, agentName string) {
	if err := cleanupStep(ctx, func(ctx context.Context) error {
		return s.repo.DeleteSubscriptionsUnder(ctx, ouID, projectName, agentName)
	}); err != nil {
		s.logger.Error("Failed to remove the event subscriptions of a deleted agent",
			"ouID", ouID, "project", projectName, "agent", agentName, "error", err)
	}
}

// cleanupStep runs one cleanup step after a delete. The owner is already
// gone, so a client disconnecting now must not leave its subscriptions for a
// later project or agent of the same name; each step gets its own deadline
// instead, so one stalled step does not starve the next.
func cleanupStep(ctx context.Context, step func(context.Context) error) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), eventCleanupTimeout)
	defer cancel()
	return step(ctx)
}

// validate checks and normalises an input for t's scope.
func (s *eventSubscriptionService) validate(t repositories.EventTarget, in *EventSubscriptionInput) error {
	in.Name = strings.TrimSpace(in.Name)
	in.Description = strings.TrimSpace(in.Description)
	if in.Name == "" || len(in.Name) > maxEventNameLength {
		return fmt.Errorf("%w: name is required and must be at most %d characters", utils.ErrInvalidInput, maxEventNameLength)
	}
	if len(in.Description) > maxEventDescriptionLength {
		return fmt.Errorf("%w: description must be at most %d characters", utils.ErrInvalidInput, maxEventDescriptionLength)
	}
	if in.DestinationID == uuid.Nil {
		return fmt.Errorf("%w: destinationId is required", utils.ErrInvalidInput)
	}
	types := uniqueTrimmed(in.EventTypes)
	switch {
	case len(types) == 0:
		return fmt.Errorf("%w: select at least one event", utils.ErrInvalidInput)
	case len(types) > 1 && slices.Contains(types, events.AllEvents):
		return fmt.Errorf("%w: %q selects every event and cannot be combined with other events",
			utils.ErrInvalidInput, events.AllEvents)
	}
	for _, name := range types {
		if name != events.AllEvents && !events.CanSubscribe(events.Scope(t.Scope), name) {
			return fmt.Errorf("%w: %q is not an event a %s subscription can receive", utils.ErrInvalidInput, name, t.Scope)
		}
	}
	in.EventTypes = types
	in.Environments = uniqueTrimmed(in.Environments)
	return nil
}

// checkTarget refuses a subscription for a project or agent that does not
// exist, which would otherwise pass to whichever one later takes the name,
// and one naming an environment the org does not have, which would never
// match.
func (s *eventSubscriptionService) checkTarget(ctx context.Context, t repositories.EventTarget, environments []string) error {
	switch t.Scope {
	case models.EventScopeProject:
		if _, err := s.oc.GetProject(ctx, t.OUID, t.ProjectName); err != nil {
			return translateProjectError(err)
		}
	case models.EventScopeAgent:
		if _, err := getAgent(ctx, s.oc, t.OUID, t.ProjectName, t.AgentName); err != nil {
			return err
		}
	}
	if len(environments) == 0 {
		return nil
	}
	envs, err := s.oc.ListEnvironments(ctx, t.OUID)
	if err != nil {
		return fmt.Errorf("failed to list environments: %w", err)
	}
	known := make(map[string]bool, len(envs))
	for _, env := range envs {
		known[env.Name] = true
	}
	for _, name := range environments {
		if !known[name] {
			return fmt.Errorf("%w: environment %q does not exist", utils.ErrInvalidInput, name)
		}
	}
	return nil
}

// destinationCheck vetoes saving a subscription of t to a destination that
// is not available to t's project.
func destinationCheck(t repositories.EventTarget) func(*models.EventDestination) error {
	return func(d *models.EventDestination) error {
		if !d.UsableBy(t.Scope, t.ProjectName) {
			return fmt.Errorf("%w: the destination %q is not available to project %s",
				utils.ErrInvalidInput, d.Name, t.ProjectName)
		}
		return nil
	}
}

// subscriptionSaveError maps a repository error from saving a subscription.
func subscriptionSaveError(err error, fallback string) error {
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		return fmt.Errorf("%w: the destination does not exist", utils.ErrInvalidInput)
	case errors.Is(err, utils.ErrInvalidInput):
		return err
	default:
		return fmt.Errorf("%s: %w", fallback, err)
	}
}

func subscriptionAuditOpts(t repositories.EventTarget, sub *models.EventSubscription) []audit.Option {
	allEvents := len(sub.EventTypes) == 1 && sub.EventTypes[0] == events.AllEvents
	opts := []audit.Option{
		audit.Org(t.OUID),
		audit.Detail("scope", t.Scope),
		audit.Detail("destinationId", sub.DestinationID.String()),
		audit.Detail("eventCount", len(sub.EventTypes)),
		audit.Detail("allEvents", allEvents),
		audit.Detail("enabled", sub.Enabled),
		audit.ResourceNamed(audit.ResourceEventSubscription, sub.ID.String(), sub.Name),
	}
	if t.ProjectName != "" {
		opts = append(opts, audit.Project(t.ProjectName))
	}
	if len(sub.Environments) > 0 {
		opts = append(opts, audit.Detail("environments", sub.Environments))
	}
	return opts
}
