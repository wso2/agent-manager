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

package aihub

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/dberr"
)

// Subscriptions are made in a developer portal: a subscriber subscribes to an MCP server there, on one of the
// plans it offers, and gets a subscription key. The portal's webhook events copy each subscription in
// (plan_webhook_controller.go), and the AI Gateway sends it to the gateways, which accept the key only on that
// MCP server (subscription-validation) and hold it to the plan's limits.

// subscriptionChange is what a webhook delivery asks of a subscription, as of At at the portal.
type subscriptionChange struct {
	Event          string // created, updated, plan_changed, token_regenerated or deleted
	At             time.Time
	SourceID       string // the portal's subscription id
	SubscriberID   string
	ArtifactKind   string
	ArtifactHandle string
	PlanHandle     string
	Status         string
	Token          *string // on created and token_regenerated
}

// subscriptionRow is a stored subscription.
type subscriptionRow struct {
	ID              string
	OrgID           string
	SourceID        string
	ArtifactKind    string
	ArtifactHandle  string
	PlanID          string
	SubscriberID    string
	Status          string
	CreatedAt       time.Time
	UpdatedAt       time.Time
	SourceChangedAt time.Time
}

const subscriptionColumns = `id, org_id, source_id, artifact_kind, artifact_handle, plan_id, subscriber_id, status, ` +
	`created_at, updated_at, source_changed_at`

// subscriptionService holds the subscriptions. It reads and writes only the module's own tables, and keeps
// each subscription key in the core's secret store.
type subscriptionService struct {
	db      func() *gorm.DB
	secrets func() modkit.Secrets
	plans   *planService
	publish func(ctx context.Context, tx *gorm.DB, data subplanv1.SubscriptionChange) error
	log     *slog.Logger
	now     func() time.Time
}

// tokenPath is where a subscription's key is kept in the core's secret store.
func tokenPath(subscriptionID string) string { return "subscriptions/" + subscriptionID + "/token" }

var errSubscriptionPlanUnknown = errors.New("the subscription's plan is not a plan of the organization")

// applyFromWebhook copies a subscription event in. Events are applied in the order they happened at the portal:
// one older than the subscription's last change there changes nothing. An event for a subscription that is
// not here (an update before its creation arrived, or after its deletion) changes nothing either; the portal
// sends the subscription whole on its next creation. Repeats change nothing.
func (s *subscriptionService) applyFromWebhook(ctx context.Context, orgID string, ch subscriptionChange) error {
	return s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := s.find(tx, orgID, ch)
		if err != nil {
			return err
		}
		if existing != nil && ch.At.Before(existing.SourceChangedAt.UTC()) {
			s.log.InfoContext(ctx, "ignoring an out-of-date subscription event", "orgId", orgID,
				"subscription", existing.ID, "event", ch.Event)
			return nil
		}
		switch ch.Event {
		case "deleted":
			if existing == nil {
				return nil
			}
			return s.remove(ctx, tx, orgID, existing)
		case "created":
			if existing == nil {
				return s.insert(ctx, tx, orgID, ch)
			}
		}
		if existing == nil {
			s.log.InfoContext(ctx, "ignoring an event for a subscription that is not here", "orgId", orgID,
				"sourceId", ch.SourceID, "event", ch.Event)
			return nil
		}
		return s.update(ctx, tx, orgID, existing, ch)
	})
}

// find reads the subscription an event is about: by the portal's subscription id, or else by subscriber and
// artifact (one subscription per subscriber and artifact).
func (s *subscriptionService) find(tx *gorm.DB, orgID string, ch subscriptionChange) (*subscriptionRow, error) {
	var rows []subscriptionRow
	q := tx.Raw(`SELECT `+subscriptionColumns+` FROM mod_ai_hub_subscriptions WHERE org_id = ? AND source_id = ?`,
		orgID, ch.SourceID)
	if ch.SourceID == "" {
		q = tx.Raw(`SELECT `+subscriptionColumns+` FROM mod_ai_hub_subscriptions `+
			`WHERE org_id = ? AND artifact_kind = ? AND artifact_handle = ? AND subscriber_id = ?`,
			orgID, ch.ArtifactKind, ch.ArtifactHandle, ch.SubscriberID)
	}
	if err := q.Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("read subscription: %w", err)
	}
	if len(rows) == 0 {
		return nil, nil //nolint:nilnil // not found is no error here: the caller decides
	}
	return &rows[0], nil
}

// planID resolves the plan an event names by its handle.
func (s *subscriptionService) planID(tx *gorm.DB, orgID, handle string) (string, error) {
	p, err := s.plans.getTx(tx, orgID, handle)
	if errors.Is(err, errPlanNotFound) {
		return "", fmt.Errorf("%w: %s", errSubscriptionPlanUnknown, handle)
	}
	if err != nil {
		return "", err
	}
	return p.UUID, nil
}

func (s *subscriptionService) insert(ctx context.Context, tx *gorm.DB, orgID string, ch subscriptionChange) error {
	if ch.Token == nil || *ch.Token == "" {
		return invalid("a created subscription carries its token")
	}
	planID, err := s.planID(tx, orgID, ch.PlanHandle)
	if err != nil {
		return err
	}
	id, err := uuid.NewV7()
	if err != nil {
		return fmt.Errorf("new subscription id: %w", err)
	}
	now := s.now().UTC()
	err = tx.Transaction(func(sp *gorm.DB) error {
		return sp.Exec(`INSERT INTO mod_ai_hub_subscriptions (`+subscriptionColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			id.String(), orgID, ch.SourceID, ch.ArtifactKind, ch.ArtifactHandle, planID, ch.SubscriberID,
			statusOrActive(ch.Status), now, now, ch.At.UTC()).Error
	})
	if dberr.IsDuplicateKey(err) {
		// The subscriber already has a subscription to this artifact under another portal id: a repeat.
		s.log.InfoContext(ctx, "ignoring a second subscription of the subscriber to the artifact", "orgId", orgID,
			"artifact", ch.ArtifactHandle, "sourceId", ch.SourceID)
		return nil
	}
	if err != nil {
		return fmt.Errorf("insert subscription: %w", err)
	}
	if err := s.secrets().In(tx).Put(ctx, orgID, tokenPath(id.String()), *ch.Token); err != nil {
		return fmt.Errorf("store the subscription key: %w", err)
	}
	s.log.InfoContext(ctx, "subscription copied in from the developer portal", "orgId", orgID,
		"subscription", id.String(), "artifact", ch.ArtifactHandle, "plan", ch.PlanHandle)
	return s.publish(ctx, tx, subplanv1.SubscriptionChange{
		OrgID: orgID, SubscriptionID: id.String(), ArtifactKind: ch.ArtifactKind, ArtifactHandle: ch.ArtifactHandle,
		Action: subplanv1.Created,
	})
}

// update applies an update, plan change, token regeneration or a repeated creation.
func (s *subscriptionService) update(ctx context.Context, tx *gorm.DB, orgID string, row *subscriptionRow, ch subscriptionChange) error {
	planID, status := row.PlanID, row.Status
	switch ch.Event {
	case "created", "plan_changed":
		var err error
		if planID, err = s.planID(tx, orgID, ch.PlanHandle); err != nil {
			return err
		}
	}
	if ch.Event == "created" || ch.Event == "updated" {
		status = statusOrActive(ch.Status)
	}
	if ch.Token != nil && *ch.Token != "" {
		if err := s.secrets().In(tx).Put(ctx, orgID, tokenPath(row.ID), *ch.Token); err != nil {
			return fmt.Errorf("store the subscription key: %w", err)
		}
	}
	if err := tx.Exec(`UPDATE mod_ai_hub_subscriptions SET plan_id = ?, status = ?, updated_at = ?, source_changed_at = ? `+
		`WHERE org_id = ? AND id = ?`, planID, status, s.now().UTC(), ch.At.UTC(), orgID, row.ID).Error; err != nil {
		return fmt.Errorf("update subscription %s: %w", row.ID, err)
	}
	return s.publish(ctx, tx, subplanv1.SubscriptionChange{
		OrgID: orgID, SubscriptionID: row.ID, ArtifactKind: row.ArtifactKind, ArtifactHandle: row.ArtifactHandle,
		Action: subplanv1.Updated,
	})
}

func (s *subscriptionService) remove(ctx context.Context, tx *gorm.DB, orgID string, row *subscriptionRow) error {
	res := tx.Exec(`DELETE FROM mod_ai_hub_subscriptions WHERE org_id = ? AND id = ?`, orgID, row.ID)
	if res.Error != nil {
		return fmt.Errorf("delete subscription %s: %w", row.ID, res.Error)
	}
	if res.RowsAffected == 0 {
		return nil
	}
	if err := s.secrets().In(tx).Delete(ctx, orgID, tokenPath(row.ID)); err != nil {
		return fmt.Errorf("delete the subscription key: %w", err)
	}
	s.log.InfoContext(ctx, "subscription deleted at the developer portal", "orgId", orgID, "subscription", row.ID)
	return s.publish(ctx, tx, subplanv1.SubscriptionChange{
		OrgID: orgID, SubscriptionID: row.ID, ArtifactKind: row.ArtifactKind, ArtifactHandle: row.ArtifactHandle,
		Action: subplanv1.Deleted,
	})
}

func statusOrActive(status string) string {
	if strings.EqualFold(status, subplanv1.SubscriptionInactive) {
		return subplanv1.SubscriptionInactive
	}
	return subplanv1.SubscriptionActive
}

// list returns the subscriptions to one artifact, with their keys, for gateways.
func (s *subscriptionService) list(ctx context.Context, orgID, kind, handle string) ([]subplanv1.Subscription, error) {
	var rows []subscriptionRow
	if err := s.db().WithContext(ctx).Raw(`SELECT `+subscriptionColumns+` FROM mod_ai_hub_subscriptions `+
		`WHERE org_id = ? AND artifact_kind = ? AND artifact_handle = ? ORDER BY created_at`, orgID, kind, handle).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("list subscriptions: %w", err)
	}
	out := make([]subplanv1.Subscription, 0, len(rows))
	for i := range rows {
		sub, err := s.withToken(ctx, &rows[i])
		if err != nil {
			return nil, err
		}
		out = append(out, sub)
	}
	return out, nil
}

// get returns one subscription, with its key.
func (s *subscriptionService) get(ctx context.Context, orgID, id string) (subplanv1.Subscription, bool, error) {
	// A malformed id names no subscription.
	if !validID(id) {
		return subplanv1.Subscription{}, false, nil
	}
	var rows []subscriptionRow
	if err := s.db().WithContext(ctx).Raw(`SELECT `+subscriptionColumns+` FROM mod_ai_hub_subscriptions `+
		`WHERE org_id = ? AND id = ?`, orgID, id).Scan(&rows).Error; err != nil {
		return subplanv1.Subscription{}, false, fmt.Errorf("read subscription %s: %w", id, err)
	}
	if len(rows) == 0 {
		return subplanv1.Subscription{}, false, nil
	}
	sub, err := s.withToken(ctx, &rows[0])
	return sub, err == nil, err
}

func (s *subscriptionService) withToken(ctx context.Context, r *subscriptionRow) (subplanv1.Subscription, error) {
	token, err := s.secrets().Get(ctx, r.OrgID, tokenPath(r.ID))
	if err != nil {
		return subplanv1.Subscription{}, fmt.Errorf("read the key of subscription %s: %w", r.ID, err)
	}
	var planHandle string
	if err := s.db().WithContext(ctx).Raw(`SELECT handle FROM mod_ai_hub_subscription_plans WHERE org_id = ? AND id = ?`,
		r.OrgID, r.PlanID).Scan(&planHandle).Error; err != nil {
		return subplanv1.Subscription{}, fmt.Errorf("read the plan of subscription %s: %w", r.ID, err)
	}
	return subplanv1.Subscription{
		ID: r.ID, ArtifactKind: r.ArtifactKind, ArtifactHandle: r.ArtifactHandle, PlanID: r.PlanID,
		PlanHandle: planHandle, SubscriberID: r.SubscriberID, Token: token, Status: r.Status,
		CreatedAt: r.CreatedAt.UTC(), UpdatedAt: r.UpdatedAt.UTC(),
	}, nil
}

// countOnPlan counts the subscriptions on a plan, which keep it in use.
func countOnPlan(tx *gorm.DB, orgID, handle string) (int, error) {
	var n int64
	if err := tx.Raw(`SELECT COUNT(*) FROM mod_ai_hub_subscriptions s JOIN mod_ai_hub_subscription_plans p `+
		`ON p.org_id = s.org_id AND p.id = s.plan_id WHERE s.org_id = ? AND p.handle = ?`, orgID, handle).
		Scan(&n).Error; err != nil {
		return 0, fmt.Errorf("count subscriptions on plan %s: %w", handle, err)
	}
	return int(n), nil
}

// validID reports whether id is a UUID, as every subscription id is.
func validID(id string) bool { return uuid.Validate(id) == nil }
