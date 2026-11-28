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
// It is an optional module: the amp distribution links it, and an operator installs it with
// `amp module install ai-hub`. It holds the plans: their API (plan_controller.go) at /m/ai-hub, and a webhook
// (plan_webhook_controller.go) at /m/ai-hub/webhooks/events on the internal server, through which plans made in
// a developer portal are copied in, with the subscriptions made there. Gateways get each subscription, and the
// plan it is on, through the AI Gateway (subplanv1).
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
)

// Name is the module's name: its routes are under /m/ai-hub (the plans API at /m/ai-hub/subscription-plans),
// its tables are mod_ai_hub_*, and its permissions are ai-hub:plan-<verb>.
const Name = "ai-hub"

// EventPlanChanged is published, in the same transaction, when a plan is created, updated or deprecated. Gateways
// don't get plan changes: a plan reaches them with its subscriptions (EventSubscriptionChanged).
const EventPlanChanged = Name + ".plan_changed"

// EventSubscriptionChanged is published, in the same transaction, when a subscription is created, changed or
// deleted, with a subplanv1.SubscriptionChange. The AI Gateway sends the change to the organization's gateways.
const EventSubscriptionChanged = Name + ".subscription_changed"

// Permissions, by their short names: the host names them ai-hub:<name>, and each action
// has its own, so the audit trail names it.
const (
	PermPlanRead   = "plan-read"
	PermPlanCreate = "plan-create"
	PermPlanUpdate = "plan-update"
	PermPlanDelete = "plan-delete"

	PermWebhookSecretRead   = "webhook-secret-read"
	PermWebhookSecretCreate = "webhook-secret-create"
	PermWebhookSecretRotate = "webhook-secret-rotate"
	PermWebhookSecretDelete = "webhook-secret-delete"
)

// ActionPortalSync is the audit action of a developer portal's webhook delivery, which creates, changes or
// deletes a plan or a subscription.
const ActionPortalSync = "portal-sync"

// auditResourceWebhookSecret and auditDetailVersion name, in the audit records of webhook secret changes, the secret
// and the version made or removed.
const (
	auditResourceWebhookSecret = "webhook-secret"
	auditDetailVersion         = "version"
)

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
		Settings:    settings,
		// Its plans and subscriptions, for gateways. It depends on no gateway: the AI Gateway reads them and
		// subscribes to EventSubscriptionChanged, so the dependency runs one way only.
		Provides: []string{subplanv1.ID},
		Permissions: []modkit.Permission{
			{Name: PermPlanRead, DisplayName: "View Subscription Plans", Description: "View subscription plans"},
			{Name: PermPlanCreate, DisplayName: "Create Subscription Plans", Description: "Create subscription plans"},
			{Name: PermPlanUpdate, DisplayName: "Change Subscription Plans", Description: "Change subscription plans"},
			{Name: PermPlanDelete, DisplayName: "Deprecate Subscription Plans", Description: "Deprecate (soft-delete) subscription plans"},
			{Name: PermWebhookSecretRead, DisplayName: "View Webhook Secrets", Description: "View developer portal webhook secrets (masked)"},
			{Name: PermWebhookSecretCreate, DisplayName: "Create Webhook Secrets", Description: "Create developer portal webhook secrets"},
			{Name: PermWebhookSecretRotate, DisplayName: "Rotate Webhook Secrets", Description: "Rotate developer portal webhook secrets"},
			{Name: PermWebhookSecretDelete, DisplayName: "Delete Webhook Secrets", Description: "Delete developer portal webhook secrets and their versions"},
		},
		RoleGrants: map[string][]string{
			"Agent Manager Admin": {
				PermPlanRead, PermPlanCreate, PermPlanUpdate, PermPlanDelete,
				PermWebhookSecretRead, PermWebhookSecretCreate, PermWebhookSecretRotate, PermWebhookSecretDelete,
			},
			"Platform Engineer": {
				PermPlanRead, PermPlanCreate, PermPlanUpdate, PermPlanDelete,
				PermWebhookSecretRead, PermWebhookSecretCreate, PermWebhookSecretRotate, PermWebhookSecretDelete,
			},
			"AI Lead":   {PermPlanRead},
			"Developer": {PermPlanRead},
		},
		Events: []modkit.EventType{
			{Name: EventPlanChanged, Description: "A subscription plan was created, changed or deprecated."},
			{Name: EventSubscriptionChanged, Description: "A subscription was created, changed or deleted."},
		},
		AuditActions: []modkit.AuditAction{
			// A plan made in a developer portal, copied in by a signed webhook delivery: a change to the
			// organization's configuration, made on the internal server.
			{Name: ActionPortalSync, Description: "Copy in a subscription plan or a subscription from a developer portal"},
			// Webhook secrets are credentials: their records name the secret (AuditResource, by its handle) and the
			// version made or removed, never its value or hash.
			{
				Name: PermWebhookSecretCreate, Class: "credential", Description: "Create a developer portal webhook secret",
				Details: map[string]string{auditDetailVersion: "count"},
			},
			{
				Name: PermWebhookSecretRotate, Class: "credential", Description: "Rotate a developer portal webhook secret",
				Details: map[string]string{auditDetailVersion: "count"},
			},
			{
				Name: PermWebhookSecretDelete, Class: "credential", Description: "Delete a developer portal webhook secret or one of its versions",
				Details: map[string]string{auditDetailVersion: "count"},
			},
		},
		// The console's Organization > Manage > Subscription Plans page (UI module "ai-hub").
		UI: &modkit.UI{Pages: []modkit.Page{{ID: "ai-hub/subscription-plans", Permission: PermPlanRead}}},
	}
}

//go:embed migrations
var migrations embed.FS

// Migrations creates the module's tables (mod_ai_hub_*), rendered from schema/ by make schema.
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

// OpenAPI is the module's API document, at /m/ai-hub.
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
		log: log,
		now: time.Now,
	}
	plans.usage = plans.proxyUsage
	subs := &subscriptionService{
		db: rt.DB, secrets: rt.Secrets, plans: plans,
		publish: func(ctx context.Context, tx *gorm.DB, data subplanv1.SubscriptionChange) error {
			return rt.PublishTx(ctx, tx, EventSubscriptionChanged, data)
		},
		log: log.With("component", "subscriptions"), now: time.Now,
	}
	a := &planController{plans: plans, core: rt.Core, log: log}
	cfg, err := webhookConfigFrom(rt.Config)
	if err != nil {
		return err
	}
	secrets := &webhookSecretService{
		db: rt.DB, secrets: rt.Secrets, maxVersions: cfg.SecretMaxVersions,
		log: log.With("component", "webhook-secrets"), now: time.Now, generate: newWebhookSecretValue,
	}
	// The webhook a developer portal delivers plans to, on the internal server at /m/ai-hub/webhooks/events. It
	// has no user sign-in: the receiver authenticates each delivery by its signature, with one of the
	// organizations' webhook secrets.
	rt.HandleWebhook(http.MethodPost, webhookPath, (&webhookReceiver{
		cfg: cfg, plans: plans, subscriptions: subs, secrets: secrets, core: rt.Core(),
		log: log.With("component", "plan-webhook"), now: time.Now,
	}).ServeHTTP, modkit.Audit(ActionPortalSync))
	// The organization's webhook secrets, one per developer portal that sends it deliveries, under /m/ai-hub.
	// Changes to them are credential changes, so they are audited fail-closed, as the AI Gateway's API keys and
	// gateway tokens are: the core writes an intent record before the handler runs, and answers 503 when it can't.
	ws := &webhookSecretController{secrets: secrets, core: rt.Core, log: log}
	failClosed := modkit.AuditFailClosed()
	rt.Handle(http.MethodPost, "/webhook-secrets", PermWebhookSecretCreate, ws.createSecret, failClosed)
	rt.Handle(http.MethodGet, "/webhook-secrets", PermWebhookSecretRead, ws.listSecrets)
	rt.Handle(http.MethodGet, "/webhook-secrets/{webhookSecretId}", PermWebhookSecretRead, ws.getSecret)
	rt.Handle(http.MethodDelete, "/webhook-secrets/{webhookSecretId}", PermWebhookSecretDelete, ws.deleteSecret, failClosed)
	rt.Handle(http.MethodPost, "/webhook-secrets/{webhookSecretId}/rotate", PermWebhookSecretRotate, ws.rotateSecret,
		failClosed)
	rt.Handle(http.MethodDelete, "/webhook-secrets/{webhookSecretId}/versions/{version}", PermWebhookSecretDelete,
		ws.deleteSecretVersion, failClosed)
	// Gateways pull the subscriptions, and the plans they are on, when they connect; the AI Gateway pushes each
	// subscription change, after its plan, on EventSubscriptionChanged.
	rt.Provide(subplanv1.ID, planSource{plans: plans, subscriptions: subs})
	// The subscription plans API, under /m/ai-hub.
	rt.Handle(http.MethodPost, "/subscription-plans", PermPlanCreate, a.createPlan)
	rt.Handle(http.MethodGet, "/subscription-plans", PermPlanRead, a.listPlans)
	rt.Handle(http.MethodGet, "/subscription-plans/{subscriptionPlanId}", PermPlanRead, a.getPlan)
	rt.Handle(http.MethodPut, "/subscription-plans/{subscriptionPlanId}", PermPlanUpdate, a.updatePlan)
	rt.Handle(http.MethodDelete, "/subscription-plans/{subscriptionPlanId}", PermPlanDelete, a.deprecatePlan)
	return nil
}
