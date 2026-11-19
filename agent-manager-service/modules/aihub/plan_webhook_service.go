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
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"
)

// Plans made, changed or deleted elsewhere reach Agent Manager as webhook events
// (plan_webhook_controller.go) and are copied here.

// deletedPlanRetention is how long a plan deleted at its source is remembered (mod_aihub_deleted_plans): long
// enough to outlast a sender's retries of the events before the deletion.
const deletedPlanRetention = 30 * 24 * time.Hour

// upsertFromWebhook copies a plan created or updated at its source, as of occurredAt there.
//   - A change made there before the plan's last change there, or before its deletion there, is out of date:
//     it changes nothing, so a retried older event can't bring back an older state or a deleted plan.
//   - A new plan is created dated when it was made at the source, so a later deletion event isn't mistaken
//     for the deletion of an older plan with the same handle.
//   - An existing plan's name, description and conditions are updated when they differ, so an echo of
//     a change made here changes nothing. The in-use lock does not apply: the source made the change.
//
// Senders that predate conditions send only request limits (requestWindows); those change only the
// request-count condition. A priced plan is stored without its price: Agent Manager plans are free for now.
func (s *planService) upsertFromWebhook(ctx context.Context, orgID, handle, name, description string,
	conditions *[]Condition, requestWindows []Window, paid bool, occurredAt time.Time,
) error {
	if paid {
		s.log.InfoContext(ctx, "storing a priced plan from the webhook without its price", "orgId", orgID, "plan", handle)
	}
	// Stored trimmed, so compare trimmed: a name with edge spaces would otherwise update on every event.
	name = strings.TrimSpace(name)
	// The source may hold a longer description than the column; keep the plan in sync.
	description = truncateRunes(strings.TrimSpace(description), maxDescriptionChars)
	at := occurredAt.UTC()

	_, err := s.applyFromWebhook(ctx, orgID, handle, name, description, conditions, requestWindows, at)
	if errors.Is(err, errStaleEvent) {
		s.log.InfoContext(ctx, "ignoring an out-of-date subscription plan event", "orgId", orgID, "plan", handle,
			"occurredAt", at)
		return nil
	}
	return err
}

func (s *planService) applyFromWebhook(ctx context.Context, orgID, handle, name, description string,
	conditions *[]Condition, requestWindows []Window, at time.Time,
) (*plan, error) {
	existing, err := s.get(ctx, orgID, handle)
	if err != nil && !errors.Is(err, errPlanNotFound) {
		return nil, err
	}
	if existing == nil {
		planConditions, err := webhookConditions(nil, conditions, requestWindows)
		if err != nil {
			return nil, err
		}
		created, err := s.create(ctx, orgID, "", &plan{
			Handle: handle, Name: name, Description: description,
			Conditions: planConditions, CreatedAt: at, SourceChangedAt: &at,
		})
		if err == nil || !errors.Is(err, errPlanExists) {
			return created, err
		}
		// A concurrent delivery created it first: reconcile against that copy instead.
		if existing, err = s.get(ctx, orgID, handle); err != nil {
			return nil, err
		}
	}
	if existing.SourceChangedAt != nil && at.Before(*existing.SourceChangedAt) {
		return nil, errStaleEvent
	}
	planConditions, err := webhookConditions(existing.Conditions, conditions, requestWindows)
	if err != nil {
		return nil, err
	}
	if existing.Name == name && existing.Description == description &&
		sameConditions(existing.Conditions, planConditions) {
		// An echo of the plan as it is: only the date of the source's change is recorded.
		return existing, s.recordSourceChange(ctx, orgID, handle, at)
	}
	// The update checks the dates again with the plan locked.
	return s.update(ctx, orgID, handle, "", &planUpdate{
		Name: &name, Description: &description,
		Conditions: &planConditions, FromWebhook: true, SourceChangedAt: &at,
	})
}

// recordSourceChange moves the plan's source date forward to at; it publishes nothing, since the plan is unchanged.
func (s *planService) recordSourceChange(ctx context.Context, orgID, handle string, at time.Time) error {
	err := s.db().WithContext(ctx).Exec(`UPDATE mod_aihub_subscription_plans SET source_changed_at = ? `+
		`WHERE org_id = ? AND handle = ? AND (source_changed_at IS NULL OR source_changed_at < ?)`,
		at, orgID, handle, at).Error
	if err != nil {
		return fmt.Errorf("record the source change of plan %s: %w", handle, err)
	}
	return nil
}

// deleteFromWebhook follows a plan deleted at its source, at occurredAt there.
//   - The deletion is remembered for deletedPlanRetention, so a create or update made before it and
//     delivered after it changes nothing.
//   - A plan that is already gone counts as deleted, so a repeated delivery succeeds.
//   - A plan created after occurredAt is a newer plan with the same handle (made again here); the
//     deletion was of the older one, so the newer plan is kept.
//
// The in-use check does not apply: the source deleted the plan.
func (s *planService) deleteFromWebhook(ctx context.Context, orgID, handle string, occurredAt time.Time) error {
	at := occurredAt.UTC()
	var deleted *plan
	err := s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := s.recordDeletion(tx, orgID, handle, at); err != nil {
			return err
		}
		if err := lockPlan(tx, orgID, handle); err != nil {
			return err
		}
		existing, err := s.getTx(tx, orgID, handle)
		if errors.Is(err, errPlanNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if existing.CreatedAt.After(at) {
			s.log.InfoContext(ctx, "ignoring the deletion of an older subscription plan with the same handle",
				"orgId", orgID, "plan", handle)
			return nil
		}
		if err := deletePlanRow(tx, orgID, existing); errors.Is(err, errPlanNotFound) {
			return nil
		} else if err != nil {
			return err
		}
		deleted = existing
		return s.publish(ctx, tx, deletedEvent(existing))
	})
	if err != nil {
		return err
	}
	if deleted != nil {
		s.log.InfoContext(ctx, "subscription plan deleted at its source", "orgId", orgID, "plan", handle,
			"planId", deleted.UUID)
	}
	return nil
}

// recordDeletion remembers that the source deleted the plan at at (keeping the latest deletion) and forgets
// deletions older than deletedPlanRetention.
func (s *planService) recordDeletion(tx *gorm.DB, orgID, handle string, at time.Time) error {
	now := s.now().UTC()
	if err := tx.Exec(`DELETE FROM mod_aihub_deleted_plans WHERE recorded_at < ?`,
		now.Add(-deletedPlanRetention)).Error; err != nil {
		return fmt.Errorf("prune deleted plans: %w", err)
	}
	var rows []struct{ DeletedAt time.Time }
	if err := tx.Raw(`SELECT deleted_at FROM mod_aihub_deleted_plans WHERE org_id = ? AND handle = ?`,
		orgID, handle).Scan(&rows).Error; err != nil {
		return fmt.Errorf("read the deletion of plan %s: %w", handle, err)
	}
	switch {
	case len(rows) == 0:
		err := tx.Exec(`INSERT INTO mod_aihub_deleted_plans (org_id, handle, deleted_at, recorded_at) VALUES (?, ?, ?, ?)`,
			orgID, handle, at, now).Error
		if err != nil {
			return fmt.Errorf("record the deletion of plan %s: %w", handle, err)
		}
	case rows[0].DeletedAt.Before(at):
		err := tx.Exec(`UPDATE mod_aihub_deleted_plans SET deleted_at = ?, recorded_at = ? WHERE org_id = ? AND handle = ?`,
			at, now, orgID, handle).Error
		if err != nil {
			return fmt.Errorf("record the deletion of plan %s: %w", handle, err)
		}
	}
	return nil
}

// checkNotDeletedSince refuses (errStaleEvent) a change made at the source at or before the plan's remembered
// deletion there.
func checkNotDeletedSince(tx *gorm.DB, orgID, handle string, at time.Time) error {
	var rows []struct{ DeletedAt time.Time }
	if err := tx.Raw(`SELECT deleted_at FROM mod_aihub_deleted_plans WHERE org_id = ? AND handle = ?`,
		orgID, handle).Scan(&rows).Error; err != nil {
		return fmt.Errorf("read the deletion of plan %s: %w", handle, err)
	}
	if len(rows) > 0 && !at.After(rows[0].DeletedAt.UTC()) {
		return errStaleEvent
	}
	return nil
}

// webhookConditions returns the conditions an event gives the plan: the event's conditions, or,
// from an older sender, the current ones with only the request-count condition replaced by its limits
// (keeping onExceed, which that sender can't set).
func webhookConditions(current []Condition, conditions *[]Condition, requestWindows []Window) ([]Condition, error) {
	if conditions != nil {
		return normalizeConditions(*conditions)
	}
	if len(requestWindows) == 0 {
		return normalizeConditions(withRequestLimit(current, nil))
	}
	onExceed := OnExceedReject
	for _, c := range current {
		if c.Type != ConditionTypeRequestCount {
			continue
		}
		var params Limits
		if err := json.Unmarshal(c.Params, &params); err == nil && params.OnExceed == OnExceedAllow {
			onExceed = OnExceedAllow
		}
		break
	}
	params, err := json.Marshal(Limits{Limits: requestWindows, OnExceed: onExceed})
	if err != nil {
		return nil, err
	}
	requestLimit := Condition{ID: ConditionIDRequests, Type: ConditionTypeRequestCount, Params: params}
	return normalizeConditions(withRequestLimit(current, &requestLimit))
}

// truncateRunes cuts s to at most n characters.
func truncateRunes(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}
