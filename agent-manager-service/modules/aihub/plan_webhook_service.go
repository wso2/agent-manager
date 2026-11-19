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

// upsertFromWebhook copies a plan created or updated at its source.
//   - A new plan is created dated when it was made at the source, so a later deletion event isn't mistaken
//     for the deletion of an older plan with the same handle.
//   - An existing plan's name, description and conditions are updated when they differ, so an echo of
//     a change made here changes nothing. The in-use lock does not apply: the source made the change.
//
// Senders that predate conditions send only request limits (requestWindows); those change only the
// request-count condition. A priced plan is stored without its price: Agent Manager plans are free for now.
func (s *planService) upsertFromWebhook(ctx context.Context, orgID, handle, name, description string,
	conditions *[]Condition, requestWindows []Window, paid bool, occurredAt time.Time,
) (*plan, error) {
	if paid {
		s.log.InfoContext(ctx, "storing a priced plan from the webhook without its price", "orgId", orgID, "plan", handle)
	}
	// Stored trimmed, so compare trimmed: a name with edge spaces would otherwise update on every event.
	name = strings.TrimSpace(name)
	// The source may hold a longer description than the column; keep the plan in sync.
	description = truncateRunes(strings.TrimSpace(description), maxDescriptionChars)

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
			Conditions: planConditions, CreatedAt: occurredAt.UTC(),
		})
		if err == nil || !errors.Is(err, errPlanExists) {
			return created, err
		}
		// A concurrent delivery created it first: reconcile against that copy instead.
		if existing, err = s.get(ctx, orgID, handle); err != nil {
			return nil, err
		}
	}
	planConditions, err := webhookConditions(existing.Conditions, conditions, requestWindows)
	if err != nil {
		return nil, err
	}
	if existing.Name == name && existing.Description == description &&
		sameConditions(existing.Conditions, planConditions) {
		return existing, nil
	}
	return s.update(ctx, orgID, handle, "", &planUpdate{
		Name: &name, Description: &description,
		Conditions: &planConditions, FromWebhook: true,
	})
}

// deleteFromWebhook follows a plan deleted at its source.
//   - A plan that is already gone counts as deleted, so a repeated delivery succeeds.
//   - A plan created after occurredAt is a newer plan with the same handle (made again here); the
//     deletion was of the older one, so the newer plan is kept.
//
// The in-use check does not apply: the source deleted the plan.
func (s *planService) deleteFromWebhook(ctx context.Context, orgID, handle string, occurredAt time.Time) error {
	var deleted *plan
	err := s.db().WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		existing, err := s.getTx(tx, orgID, handle)
		if errors.Is(err, errPlanNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		if existing.CreatedAt.After(occurredAt) {
			s.log.InfoContext(ctx, "ignoring the deletion of an older subscription plan with the same handle",
				"orgId", orgID, "plan", handle)
			return nil
		}
		if err := tx.Exec(`DELETE FROM mod_aihub_subscription_plans WHERE org_id = ? AND id = ?`, orgID, existing.UUID).Error; err != nil {
			return fmt.Errorf("delete plan %s: %w", handle, err)
		}
		deleted = existing
		return s.publish(ctx, tx, planChanged{
			OrgID: orgID, PlanID: existing.UUID, Handle: existing.Handle,
			PlanName: existing.Name, Action: "deleted",
		})
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
