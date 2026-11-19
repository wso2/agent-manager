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
// It is an installable module the amp distribution installs by default. It holds the plans: their API
// (plan_controller.go) at /m/aihub, and a webhook (plan_webhook_controller.go) through which plans made in a
// developer portal are copied in. Gateways get every plan through the AI Gateway (subplanv1).
package aihub

import (
	"context"
	"embed"
	"io/fs"
	"net/http"
	"time"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aihub/aihubperm"
)

// Name is the module's name: its routes are under /m/aihub (the plans API at /m/aihub/subscription-plans),
// its tables are mod_aihub_*, and its permissions are aihub:plan-<verb>.
const Name = "aihub"

// EventPlanChanged is published, in the same transaction, when a plan is created, updated or deleted. The AI
// Gateway subscribes to it and pushes the change to the organization's gateways.
const EventPlanChanged = subplanv1.EventPlanChanged

// Permissions, by their short names: the host names them aihub:<name>, and each action
// has its own, so the audit trail names it.
const (
	PermPlanRead   = "plan-read"
	PermPlanCreate = "plan-create"
	PermPlanUpdate = "plan-update"
	PermPlanDelete = "plan-delete"
)

// ActionPlanSync is the audit action of a webhook delivery that creates, changes or deletes a plan.
const ActionPlanSync = "plan-sync"

func init() { modkit.Register(Module()) }

// Module returns the module, for tests that build their own host.
func Module() modkit.Module { return module{} }

type module struct{}

func (module) Manifest() modkit.Manifest {
	return modkit.Manifest{
		Name:        Name,
		DisplayName: "AI Hub",
		Version:     "0.1.0",
		Description: "AI Hub: subscription plans for MCP servers and agents",
		CoreAPI:     modkit.CoreAPIVersion,
		// Its plans, for gateways to pull. It depends on no gateway: the AI Gateway reads the plans and
		// subscribes to EventPlanChanged, so the dependency runs one way only.
		Provides: []string{subplanv1.ID},
		Permissions: []modkit.Permission{
			{Name: PermPlanRead, DisplayName: "View Subscription Plans", Description: "View subscription plans"},
			{Name: PermPlanCreate, DisplayName: "Create Subscription Plans", Description: "Create subscription plans"},
			{Name: PermPlanUpdate, DisplayName: "Change Subscription Plans", Description: "Change subscription plans"},
			{Name: PermPlanDelete, DisplayName: "Delete Subscription Plans", Description: "Delete subscription plans"},
		},
		RoleGrants: map[string][]string{
			"Agent Manager Admin": {PermPlanRead, PermPlanCreate, PermPlanUpdate, PermPlanDelete},
			"Platform Engineer":   {PermPlanRead, PermPlanCreate, PermPlanUpdate, PermPlanDelete},
			"AI Lead":             {PermPlanRead},
			"Developer":           {PermPlanRead},
		},
		Events: []modkit.EventType{
			{Name: EventPlanChanged, Description: "A subscription plan was created, changed or deleted."},
		},
		AuditActions: []modkit.AuditAction{
			// A plan made in a developer portal, copied in by a signed webhook delivery: a change to the
			// organization's configuration, made on the internal server.
			{Name: ActionPlanSync, Description: "Copy in a subscription plan from a developer portal"},
		},
		// The console's Organization > Manage > Subscription Plans page (UI module "aihub").
		UI: &modkit.UI{Pages: []modkit.Page{{ID: "aihub/subscription-plans", Permission: PermPlanRead}}},
	}
}

//go:embed migrations
var migrations embed.FS

// Migrations creates the module's tables (mod_aihub_*), rendered from schema/ by make schema.
func (module) Migrations() fs.FS {
	sub, err := fs.Sub(migrations, "migrations")
	if err != nil {
		panic(err)
	}
	return sub
}

func (module) UI() fs.FS { return nil }

//go:embed api/openapi.yaml
var openAPI []byte

// OpenAPI is the module's API document, at /m/aihub.
func (module) OpenAPI() []byte { return openAPI }

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
	// The webhook a developer portal delivers plans to, on the internal server at the fixed path portals are
	// configured with. It has no user sign-in: the receiver authenticates each delivery by its signature.
	rt.HandleInternalAt(http.MethodPost, WebhookRoutePath, (&webhookReceiver{
		cfg: webhookConfigFrom(rt.Config, log), plans: plans, core: rt.Core(),
		log: log.With("component", "plan-webhook"), now: time.Now,
	}).ServeHTTP, modkit.Audit(ActionPlanSync))
	// Gateways pull the plans when they connect; the AI Gateway pushes each change on EventPlanChanged.
	rt.Provide(subplanv1.ID, planSource{plans: plans})
	// The subscription plans API, under /m/aihub.
	rt.Handle(http.MethodPost, "/subscription-plans", aihubperm.PlanCreate, a.createPlan)
	rt.Handle(http.MethodGet, "/subscription-plans", aihubperm.PlanRead, a.listPlans)
	rt.Handle(http.MethodGet, "/subscription-plans/{subscriptionPlanId}", aihubperm.PlanRead, a.getPlan)
	rt.Handle(http.MethodPut, "/subscription-plans/{subscriptionPlanId}", aihubperm.PlanUpdate, a.updatePlan)
	rt.Handle(http.MethodDelete, "/subscription-plans/{subscriptionPlanId}", aihubperm.PlanDelete, a.deletePlan)
	return nil
}
