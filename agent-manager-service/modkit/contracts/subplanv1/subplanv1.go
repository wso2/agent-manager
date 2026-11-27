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

// Package subplanv1 is the subscription plan capability between the module that owns plans and the gateway:
// "subscription.plans/v1" lists an organization's plans and the subscriptions to them, and names the event its
// provider publishes, with a SubscriptionChange, when a subscription changes. The gateway module pushes each
// subscription change, after its plan, to the organization's gateways, and gateways pull the subscriptions and
// the plans they are on when they connect; a plan change alone isn't pushed. Only the gateway depends on the
// provider (optionally), and neither imports the other.
package subplanv1

import (
	"context"
	"encoding/json"
	"time"
)

// ID is the capability of the module that owns subscription plans: Source.
const ID = "subscription.plans/v1"

// Action is what happened to a subscription.
type Action string

const (
	Created Action = "created"
	Updated Action = "updated"
	Deleted Action = "deleted"
)

// Condition is one condition of a plan: a built-in limit type (request-count, tool-call-count,
// token-count) with its windows, or a custom condition a policy reads.
type Condition struct {
	ID     string          `json:"id"`
	Type   string          `json:"type"`
	Params json.RawMessage `json:"params"`
}

// Plan is a subscription plan as gateways see it. The conditions are the plan; the single throttle limit
// (from the first request-count window of length one) and StopOnQuotaReach are for gateways that read
// only that limit.
type Plan struct {
	ID     string
	Handle string
	Name   string
	Status string // ACTIVE or INACTIVE
	// Deprecated marks a plan that can't be offered or subscribed to any more. Gateways still get it, for the
	// subscriptions already on it.
	Deprecated         bool
	ExpiryTime         *time.Time
	Conditions         []Condition
	StopOnQuotaReach   bool
	ThrottleLimitCount *int
	ThrottleLimitUnit  string // MINUTE, HOUR, DAY or MONTH; empty without a throttle limit
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Source lists an organization's subscription plans, and the subscriptions to them.
type Source interface {
	// ListPlans lists every plan of the organization, deprecated ones included.
	ListPlans(ctx context.Context, orgID string) ([]Plan, error)
	// GetPlan reads one plan by its ID. found is false when the organization has no such plan.
	GetPlan(ctx context.Context, orgID, planID string) (plan Plan, found bool, err error)
	// CountOffersWith hands the provider a count of what offers a plan (the MCP proxies of a gateway), which
	// the provider reports as the plan's usage. The consumer calls it on every activation; nil stops counting.
	CountOffersWith(count OfferCounter)

	// ListSubscriptions lists the subscriptions to one artifact (an MCP proxy: kind ArtifactKindMCP and its
	// handle), with their tokens, for gateways to pull.
	ListSubscriptions(ctx context.Context, orgID, artifactKind, artifactHandle string) ([]Subscription, error)
	// SubscribedPlanIDs lists the IDs of the plans that a subscription to one of these artifacts (all of
	// artifactKind) is on, subscriptions of every status included, without reading any subscription key.
	SubscribedPlanIDs(ctx context.Context, orgID, artifactKind string, artifactHandles []string) ([]string, error)
	// GetSubscription reads one subscription by its ID. found is false when it is gone.
	GetSubscription(ctx context.Context, orgID, subscriptionID string) (sub Subscription, found bool, err error)
	// SubscriptionChangeEvent is the event the provider publishes, with a SubscriptionChange, whenever a
	// subscription is created, changed or deleted.
	SubscriptionChangeEvent() string
}

// ArtifactKindMCP is the kind of an MCP proxy in a subscription, as developer portals and gateways name it.
const ArtifactKindMCP = "Mcp"

// Subscription statuses.
const (
	SubscriptionActive   = "ACTIVE"
	SubscriptionInactive = "INACTIVE"
)

// Subscription is a subscriber's subscription to an artifact on a plan. Token is the subscription key the
// subscriber sends (the Subscription-Key header); gateways keep only its hash.
type Subscription struct {
	ID             string
	ArtifactKind   string // ArtifactKindMCP
	ArtifactHandle string // the MCP proxy's handle
	PlanID         string // the plan's ID, as gateways know it
	PlanHandle     string
	SubscriberID   string
	Token          string
	Status         string // SubscriptionActive or SubscriptionInactive
	CreatedAt      time.Time
	UpdatedAt      time.Time
}

// SubscriptionChange is what the provider's subscription event carries. A deleted subscription is named
// by its ID and artifact only; read the others back with GetSubscription.
type SubscriptionChange struct {
	OrgID          string `json:"orgId"`
	SubscriptionID string `json:"subscriptionId"`
	ArtifactKind   string `json:"artifactKind"`
	ArtifactHandle string `json:"artifactHandle"`
	Action         Action `json:"action"`
}

// OfferCounter counts what in an organization offers the plan with this handle.
type OfferCounter func(ctx context.Context, orgID, handle string) (int, error)
