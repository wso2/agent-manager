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

package gwsvc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// defaultBillingPlan is the billing plan gateways record for a plan; plans here are free.
const defaultBillingPlan = "free"

// SubscriptionPlanEvent is the payload of a subscriptionPlan.created or subscriptionPlan.updated gateway
// event. The conditions are the plan; the single throttle limit is for gateways that read only that.
type SubscriptionPlanEvent struct {
	PlanID             string                `json:"planId"`
	Handle             string                `json:"handle"`
	PlanName           string                `json:"planName"`
	BillingPlan        string                `json:"billingPlan,omitempty"`
	StopOnQuotaReach   bool                  `json:"stopOnQuotaReach"`
	ThrottleLimitCount *int                  `json:"throttleLimitCount,omitempty"`
	ThrottleLimitUnit  string                `json:"throttleLimitUnit,omitempty"`
	ExpiryTime         *time.Time            `json:"expiryTime,omitempty"`
	Status             string                `json:"status"`
	Conditions         []subplanv1.Condition `json:"conditions,omitempty"`
	// UpdatedAt is when the plan last changed, as in the pull response, so a gateway can drop an older state
	// that arrives after a newer one.
	UpdatedAt time.Time `json:"updatedAt"`
}

// SubscriptionPlanDeletedEvent is the payload of a subscriptionPlan.deleted gateway event.
type SubscriptionPlanDeletedEvent struct {
	PlanID   string `json:"planId"`
	Handle   string `json:"handle"`
	PlanName string `json:"planName"`
}

// orgGateways lists an organization's gateways.
type orgGateways interface {
	GetByOrganizationID(ctx context.Context, ouID string) ([]*gwmodels.Gateway, error)
}

// PlanGateways sends a plan to every gateway of the organization (SubscriptionPlanBroadcaster). For a deleted
// plan only its ID, Handle and Name are read.
type PlanGateways interface {
	PlanChanged(ctx context.Context, orgID string, action subplanv1.Action, plan subplanv1.Plan) error
}

// SubscriptionPlanBroadcaster sends plan changes to the organization's gateways over their control
// plane connection (PlanGateways).
type SubscriptionPlanBroadcaster struct {
	gateways orgGateways
	events   *GatewayEventsService
}

// NewSubscriptionPlanBroadcaster creates the broadcaster.
func NewSubscriptionPlanBroadcaster(gateways orgGateways, events *GatewayEventsService) *SubscriptionPlanBroadcaster {
	return &SubscriptionPlanBroadcaster{gateways: gateways, events: events}
}

var _ PlanGateways = (*SubscriptionPlanBroadcaster)(nil)

// PlanChanged sends subscriptionPlan.created, .updated or .deleted to every gateway of the organization.
// A gateway that is not connected gets the plans when it connects (it pulls them), so a failed send is
// reported but does not stop the others.
func (b *SubscriptionPlanBroadcaster) PlanChanged(ctx context.Context, orgID string, action subplanv1.Action, p subplanv1.Plan) error {
	eventType, gatewayAction, payload, err := planEvent(action, p)
	if err != nil {
		return err
	}
	gateways, err := b.gateways.GetByOrganizationID(ctx, orgID)
	if err != nil {
		return fmt.Errorf("listing the gateways of organization %s: %w", orgID, err)
	}
	var errs []error
	for _, gw := range gateways {
		if gw == nil {
			continue
		}
		if err := b.events.broadcastEvent(gw.UUID.String(), eventType, gatewayAction, p.ID, payload); err != nil {
			slog.WarnContext(ctx, "Failed to send a subscription plan event to a gateway",
				"gatewayId", gw.UUID.String(), "type", eventType, "planId", p.ID, "error", err)
			errs = append(errs, err)
		}
	}
	slog.InfoContext(ctx, "Subscription plan event sent to gateways", "type", eventType, "planId", p.ID,
		"handle", p.Handle, "orgId", orgID, "gateways", len(gateways), "failed", len(errs))
	return errors.Join(errs...)
}

// planEvent is the gateway event for a plan change: its type, action and payload.
func planEvent(action subplanv1.Action, p subplanv1.Plan) (string, string, any, error) {
	switch action {
	case subplanv1.Created, subplanv1.Updated:
		payload := SubscriptionPlanEvent{
			PlanID: p.ID, Handle: p.Handle, PlanName: p.Name, BillingPlan: defaultBillingPlan,
			StopOnQuotaReach: p.StopOnQuotaReach, ThrottleLimitCount: p.ThrottleLimitCount,
			ThrottleLimitUnit: p.ThrottleLimitUnit, ExpiryTime: p.ExpiryTime, Status: p.Status,
			Conditions: p.Conditions, UpdatedAt: p.UpdatedAt,
		}
		if action == subplanv1.Created {
			return "subscriptionPlan.created", "CREATE", payload, nil
		}
		return "subscriptionPlan.updated", "UPDATE", payload, nil
	case subplanv1.Deleted:
		return "subscriptionPlan.deleted", "DELETE", SubscriptionPlanDeletedEvent{PlanID: p.ID, Handle: p.Handle, PlanName: p.Name}, nil
	}
	return "", "", nil, fmt.Errorf("unknown subscription plan action %q", action)
}
