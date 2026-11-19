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

// Package subplanv1 is the subscription plan capabilities between the module that owns plans and the
// gateway: "subscription.plans/v1" lists an organization's plans, for gateways to pull when they connect,
// and "gateway.plans/v1" pushes a plan change to the organization's gateways as it happens. Neither module
// imports the other.
package subplanv1

import (
	"context"
	"encoding/json"
	"time"
)

// ID is the capability of the module that owns subscription plans: Source.
const ID = "subscription.plans/v1"

// GatewayID is the gateway's capability: Gateways.
const GatewayID = "gateway.plans/v1"

// Action is what happened to a plan.
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
	ID                 string
	Handle             string
	Name               string
	Status             string // ACTIVE or INACTIVE
	ExpiryTime         *time.Time
	Conditions         []Condition
	StopOnQuotaReach   bool
	ThrottleLimitCount *int
	ThrottleLimitUnit  string // MINUTE, HOUR, DAY or MONTH; empty without a throttle limit
	CreatedAt          time.Time
	UpdatedAt          time.Time
}

// Source lists an organization's subscription plans.
type Source interface {
	ListPlans(ctx context.Context, orgID string) ([]Plan, error)
}

// Gateways sends a plan change to every gateway of the organization. For a deleted plan only its ID,
// Handle and Name are read.
type Gateways interface {
	PlanChanged(ctx context.Context, orgID string, action Action, plan Plan) error
}
