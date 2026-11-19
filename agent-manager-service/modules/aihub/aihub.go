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

// Package aihub is the AI Hub module: the self-service catalog where developers find MCP servers and agents and
// subscribe to them. Its first part is subscription plans: organizations define plans (sets of request, tool-call,
// token and custom conditions) that MCP proxies offer and agents subscribe to. The gateway enforces them with the
// subscription-validation (v2) and subscription rate-limit policies.
//
// It is an installable module that the distribution enables by default (amp.yaml `modules:`). It holds the plans: their API (plan_controller.go), and a webhook
// (plan_webhook_controller.go) through which plans made elsewhere are copied in. Offering plans on MCP proxies,
// agent subscriptions and the gateway sync come next.
package aihub

import (
	"context"
	"io/fs"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/builtin"
	"github.com/wso2/agent-manager/agent-manager-service/coreext"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/wiring"
)

// Name is the module's name: its routes are under /api/v1/m/aihub (the plans API at
// /api/v1/m/aihub/subscription-plans), its tables are subscription_plans and subscription_plan_conditions
// (core schema),
// and its permissions are aihub:plan-<verb>.
const Name = "aihub"

// EventPlanChanged is published, in the same transaction, when a plan is created, updated or deleted.
const EventPlanChanged = "aihub.plan_changed"

// Permissions, by their short names: the host names them aihub:<name>, and each action
// has its own, so the audit trail names it.
const (
	PermPlanRead   = "plan-read"
	PermPlanCreate = "plan-create"
	PermPlanUpdate = "plan-update"
	PermPlanDelete = "plan-delete"
)

func init() {
	modkit.Register(Module())
	builtin.Register(builtin.Bundle{Name: Name, Build: buildBundle})
}

// WebhookCapability is how the internal webhook route reaches the module: the module provides it while it is
// active, so a disabled (or never installed) module answers no delivery.
const WebhookCapability = "aihub.webhook/v1"

// buildBundle adds what a module's Runtime can't: the plan webhook on the internal server.
func buildBundle(context.Context, *wiring.AppParams) (*builtin.Parts, error) { return Parts(), nil }

// Parts is what the module adds to the server outside its Runtime: the plan webhook,
// POST /api/internal/v1/webhook/events on the internal server, which has no user sign-in. The receiver
// authenticates each delivery by its signature.
func Parts() *builtin.Parts {
	return &builtin.Parts{
		InternalRoutes: func(rr *middleware.RouteRegistrar, _ *http.ServeMux) {
			rr.HandleFuncWithValidation(http.MethodPost+" "+WebhookRoutePath, func(w http.ResponseWriter, r *http.Request) {
				current, ok := coreext.Lookup[http.Handler](WebhookCapability)
				if !ok {
					reject(w)
					return
				}
				current.ServeHTTP(w, r)
			})
		},
	}
}

// Module returns the module, for tests that build their own host.
func Module() modkit.Module { return module{} }

type module struct{}

func (module) Manifest() modkit.Manifest {
	return modkit.Manifest{
		Name:    Name,
		Version: "0.1.0",
		CoreAPI: modkit.CoreAPIVersion,
		// Installable: the distribution enables it by default, and an administrator can disable it.
		// Its plans, for gateways to pull; and the gateway, when there is one, to push plan changes to.
		Provides: []string{WebhookCapability, subplanv1.ID},
		Optional: []string{subplanv1.GatewayID},
		Permissions: []modkit.Permission{
			{Name: PermPlanRead, Description: "View subscription plans"},
			{Name: PermPlanCreate, Description: "Create subscription plans"},
			{Name: PermPlanUpdate, Description: "Change subscription plans"},
			{Name: PermPlanDelete, Description: "Delete subscription plans"},
		},
		RoleGrants: map[string][]string{
			"Agent Manager Admin": {PermPlanRead, PermPlanCreate, PermPlanUpdate, PermPlanDelete},
			"Platform Engineer":   {PermPlanRead, PermPlanCreate, PermPlanUpdate, PermPlanDelete},
			"AI Lead":             {PermPlanRead},
			"Developer":           {PermPlanRead},
		},
		Events: []string{EventPlanChanged},
		// The console's Organization > Manage > Subscription plans page (UI module "aihub").
		UI: &modkit.UI{Pages: []modkit.Page{{ID: "aihub/subscription-plans", Permission: PermPlanRead}}},
	}
}

// Migrations is empty: its tables (subscription_plans, subscription_plan_conditions) are in the core schema.
func (module) Migrations() fs.FS { return nil }

func (module) UI() fs.FS { return nil }

// Activate registers the routes. It runs on every replica whenever the set of active modules changes, so it only
// registers.
func (module) Activate(_ context.Context, rt modkit.Runtime) error {
	log := rt.Logger()
	plans := &planService{
		db: rt.DB,
		publish: func(ctx context.Context, tx *gorm.DB, data planChanged) error {
			return rt.PublishTx(ctx, tx, EventPlanChanged, data)
		},
		usage: noUsage,
		log:   log,
		now:   time.Now,
	}
	a := &planController{plans: plans, core: rt.Core, log: log}
	// The webhook route is on the internal server (see Parts); it serves with what this activation built,
	// for as long as the module is active.
	rt.Provide(WebhookCapability, http.Handler(&webhookReceiver{
		cfg: webhookConfigFrom(rt.Config, log), plans: plans,
		db: rt.DB, log: log.With("component", "plan-webhook"), now: time.Now,
	}))
	// Gateways pull the plans when they connect, and get each change pushed once it is committed.
	rt.Provide(subplanv1.ID, planSource{plans: plans})
	rt.Subscribe(EventPlanChanged, syncPlanChange(rt))
	// The subscription plans API, under /api/v1/m/aihub.
	rt.Handle(http.MethodPost, "/subscription-plans", PermPlanCreate, a.createPlan)
	rt.Handle(http.MethodGet, "/subscription-plans", PermPlanRead, a.listPlans)
	rt.Handle(http.MethodGet, "/subscription-plans/{subscriptionPlanId}", PermPlanRead, a.getPlan)
	rt.Handle(http.MethodPut, "/subscription-plans/{subscriptionPlanId}", PermPlanUpdate, a.updatePlan)
	rt.Handle(http.MethodDelete, "/subscription-plans/{subscriptionPlanId}", PermPlanDelete, a.deletePlan)
	return nil
}
