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
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// Subscriptions reach the gateways the way gateways expect them from a control plane: pushed over their
// WebSocket on every change (subscription.created, .updated, .deleted), and pulled for each MCP proxy they
// hold when they connect (GET /api/internal/v1/apis/{apiId}/subscriptions). A gateway keeps the
// subscription key hashed, and accepts it only on that MCP proxy (subscription-validation v2).
//
// The developer portal is where plans are subscribed to, so a plan reaches the gateways with its first
// subscription, not when it is made or changed here: each created or updated subscription is preceded by its
// plan (subscriptionPlan.created, which a gateway stores or replaces). Gateways pull only the plans that have
// a subscription (GET /api/internal/v1/subscription-plans).

// SubscriptionEvent is the payload of a subscription.created or subscription.updated gateway event.
type SubscriptionEvent struct {
	APIID              string `json:"apiId"`
	SubscriptionID     string `json:"subscriptionId"`
	SubscriptionToken  string `json:"subscriptionToken"`
	SubscriptionPlanID string `json:"subscriptionPlanId,omitempty"`
	Status             string `json:"status"`
}

// SubscriptionDeletedEvent is the payload of a subscription.deleted gateway event.
type SubscriptionDeletedEvent struct {
	APIID          string `json:"apiId"`
	SubscriptionID string `json:"subscriptionId"`
}

// GatewaySubscription is one subscription in the pull response, as gateways read it.
type GatewaySubscription struct {
	ID                 string    `json:"id"`
	APIID              string    `json:"apiId"`
	ApplicationID      *string   `json:"applicationId"`
	SubscriptionToken  string    `json:"subscriptionToken"`
	SubscriptionPlanID string    `json:"subscriptionPlanId,omitempty"`
	Status             string    `json:"status"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
}

// ToGatewaySubscription is a subscription as the pull response lists it, for the deployed artifact apiID.
func ToGatewaySubscription(apiID string, s subplanv1.Subscription) GatewaySubscription {
	return GatewaySubscription{
		ID: GatewaySubscriptionID(s.ID, apiID), APIID: apiID, ApplicationID: nil, SubscriptionToken: s.Token, SubscriptionPlanID: s.PlanID,
		Status: s.Status, CreatedAt: s.CreatedAt, UpdatedAt: s.UpdatedAt,
	}
}

// ArtifactKindOf maps a subscription's artifact kind (as portals and gateways name it) to this module's.
func ArtifactKindOf(kind string) (string, bool) {
	if kind == subplanv1.ArtifactKindMCP {
		return gwmodels.KindMCPProxy, true
	}
	return "", false
}

// SubscriptionArtifacts finds what gateways know an MCP proxy's subscriptions by. A gateway doesn't hold the
// proxy itself: it holds one deployed artifact per endpoint and environment, so a subscription to the proxy
// is sent once for each deployed artifact.
type SubscriptionArtifacts interface {
	// DeployedArtifactIDs lists the IDs of the artifacts the proxy is deployed as; empty when the organization
	// has no such proxy or it isn't deployed.
	DeployedArtifactIDs(ctx context.Context, orgID, kind, handle string) ([]string, error)
	// DeployedProxies lists the handles of the organization's MCP proxies that are deployed, as subscriptions
	// name their artifact (subplanv1.ArtifactKindMCP).
	DeployedProxies(ctx context.Context, orgID string) ([]string, error)
	// ProxyOf finds the proxy a deployed artifact (or the proxy's own artifact) belongs to: its kind as
	// subscriptions name it, and its handle. found is false for an artifact that takes no subscriptions.
	ProxyOf(ctx context.Context, orgID, artifactID string) (kind, handle string, found bool, err error)
}

type subscriptionArtifacts struct{ db *gorm.DB }

// NewSubscriptionArtifacts returns the lookup over the module's MCP proxy deployments.
func NewSubscriptionArtifacts(db *gorm.DB) SubscriptionArtifacts {
	return subscriptionArtifacts{db: db}
}

func (a subscriptionArtifacts) DeployedArtifactIDs(ctx context.Context, orgID, kind, handle string) ([]string, error) {
	ownKind, ok := ArtifactKindOf(kind)
	if !ok {
		return nil, nil
	}
	var ids []string
	if err := a.db.WithContext(ctx).Raw(`SELECT ee.artifact_id FROM mod_ai_gateway_mcp_proxy_endpoint_environments ee `+
		`JOIN mod_ai_gateway_artifacts a ON a.id = ee.mcp_proxy_id `+
		`WHERE a.org_id = ? AND a.handle = ? AND a.kind = ? ORDER BY ee.artifact_id`, orgID, handle, ownKind).
		Scan(&ids).Error; err != nil {
		return nil, fmt.Errorf("list the deployments of %s: %w", handle, err)
	}
	return ids, nil
}

func (a subscriptionArtifacts) DeployedProxies(ctx context.Context, orgID string) ([]string, error) {
	var handles []string
	if err := a.db.WithContext(ctx).Raw(`SELECT DISTINCT a.handle FROM mod_ai_gateway_artifacts a `+
		`JOIN mod_ai_gateway_mcp_proxy_endpoint_environments ee ON ee.mcp_proxy_id = a.id `+
		`WHERE a.org_id = ? AND a.kind = ? ORDER BY a.handle`, orgID, gwmodels.KindMCPProxy).
		Scan(&handles).Error; err != nil {
		return nil, fmt.Errorf("list the deployed MCP proxies of organization %s: %w", orgID, err)
	}
	return handles, nil
}

func (a subscriptionArtifacts) ProxyOf(ctx context.Context, orgID, artifactID string) (string, string, bool, error) {
	var handles []string
	if err := a.db.WithContext(ctx).Raw(`SELECT a.handle FROM mod_ai_gateway_artifacts a `+
		`LEFT JOIN mod_ai_gateway_mcp_proxy_endpoint_environments ee ON ee.mcp_proxy_id = a.id AND ee.org_id = a.org_id `+
		`WHERE a.org_id = ? AND a.kind = ? AND (ee.artifact_id = ? OR a.id = ?) LIMIT 1`,
		orgID, gwmodels.KindMCPProxy, artifactID, artifactID).Scan(&handles).Error; err != nil {
		return "", "", false, fmt.Errorf("find the MCP proxy of artifact %s: %w", artifactID, err)
	}
	if len(handles) == 0 {
		return "", "", false, nil
	}
	return subplanv1.ArtifactKindMCP, handles[0], true, nil
}

// GatewaySubscriptionID is the ID a gateway keeps a subscription under for one deployed artifact: the
// subscription's own ID made distinct per artifact, so a gateway that holds two deployments of the proxy
// keeps both.
func GatewaySubscriptionID(subscriptionID, artifactID string) string {
	ns, err := uuid.Parse(subscriptionID)
	if err != nil {
		return subscriptionID
	}
	return uuid.NewSHA1(ns, []byte(artifactID)).String()
}

// SubscriptionGateways sends a subscription change to every gateway of the organization.
type SubscriptionGateways interface {
	SubscriptionChanged(ctx context.Context, orgID string, action subplanv1.Action, apiID string, sub subplanv1.Subscription) error
}

// SubscriptionChangeHandler pushes a committed subscription change (the event the plans module names with
// subplanv1.Source.SubscriptionChangeEvent) to the organization's gateways. A created or updated subscription
// is read back from the module that owns it, so the gateways get its current plan, status and key, and its
// plan is sent first (planGateways), as the plan has it now, so the gateway holds the plan the subscription
// names. A subscription to an artifact this module doesn't hold is not sent: no gateway serves it.
func SubscriptionChangeHandler(plans func() (subplanv1.Source, bool), artifacts SubscriptionArtifacts,
	planGateways PlanGateways, gateways SubscriptionGateways,
) modkit.EventHandler {
	return func(ctx context.Context, e modkit.Event) error {
		var ch subplanv1.SubscriptionChange
		if err := json.Unmarshal(e.Data, &ch); err != nil {
			return fmt.Errorf("decoding %s: %w", e.Type, err)
		}
		orgID := ch.OrgID
		if orgID == "" {
			orgID = e.OrgID
		}
		apiIDs, err := artifacts.DeployedArtifactIDs(ctx, orgID, ch.ArtifactKind, ch.ArtifactHandle)
		if err != nil {
			return err
		}
		if len(apiIDs) == 0 {
			slog.InfoContext(ctx, "Subscription to an artifact no gateway holds; not sent",
				"orgId", orgID, "artifact", ch.ArtifactHandle, "subscriptionId", ch.SubscriptionID)
			return nil
		}
		sub := subplanv1.Subscription{ID: ch.SubscriptionID}
		switch ch.Action {
		case subplanv1.Deleted:
		case subplanv1.Created, subplanv1.Updated:
			source, ok := plans()
			if !ok {
				return nil
			}
			var found bool
			if sub, found, err = source.GetSubscription(ctx, orgID, ch.SubscriptionID); err != nil {
				return fmt.Errorf("reading subscription %s: %w", ch.SubscriptionID, err)
			}
			if !found {
				// Deleted since: its own deletion event follows.
				return nil
			}
			if err := sendPlanOf(ctx, source, planGateways, orgID, sub); err != nil {
				return err
			}
		default:
			return fmt.Errorf("subscription change %q has an unknown action %q", ch.SubscriptionID, ch.Action)
		}
		var errs []error
		for _, apiID := range apiIDs {
			perArtifact := sub
			perArtifact.ID = GatewaySubscriptionID(sub.ID, apiID)
			if err := gateways.SubscriptionChanged(ctx, orgID, ch.Action, apiID, perArtifact); err != nil {
				errs = append(errs, err)
			}
		}
		return errors.Join(errs...)
	}
}

// sendPlanOf sends the subscription's plan to the organization's gateways, as subscriptionPlan.created: a
// gateway stores it, or replaces the one it has. A plan that is gone is not sent; the subscription still is.
func sendPlanOf(ctx context.Context, source subplanv1.Source, planGateways PlanGateways, orgID string,
	sub subplanv1.Subscription,
) error {
	if sub.PlanID == "" {
		return nil
	}
	p, found, err := source.GetPlan(ctx, orgID, sub.PlanID)
	if err != nil {
		return fmt.Errorf("reading plan %s of subscription %s: %w", sub.PlanID, sub.ID, err)
	}
	if !found {
		slog.WarnContext(ctx, "The plan of a subscription is gone; the subscription is sent without it",
			"orgId", orgID, "planId", sub.PlanID, "subscriptionId", sub.ID)
		return nil
	}
	return planGateways.PlanChanged(ctx, orgID, subplanv1.Created, p)
}

// SubscribedPlans keeps the plans that a subscription to one of the organization's deployed MCP proxies is
// on: the plans gateways pull. Subscriptions of every status count, since a gateway holds them all.
func SubscribedPlans(ctx context.Context, source subplanv1.Source, artifacts SubscriptionArtifacts, orgID string,
	plans []subplanv1.Plan,
) ([]subplanv1.Plan, error) {
	handles, err := artifacts.DeployedProxies(ctx, orgID)
	if err != nil {
		return nil, err
	}
	ids, err := source.SubscribedPlanIDs(ctx, orgID, subplanv1.ArtifactKindMCP, handles)
	if err != nil {
		return nil, fmt.Errorf("listing the subscribed plans: %w", err)
	}
	subscribed := make(map[string]bool, len(ids))
	for _, id := range ids {
		subscribed[id] = true
	}
	out := make([]subplanv1.Plan, 0, len(subscribed))
	for _, p := range plans {
		if subscribed[p.ID] {
			out = append(out, p)
		}
	}
	return out, nil
}

// replaySubscriptions writes, in tx, the subscriptions to the MCP proxy that the deployed artifact belongs to, each
// preceded by its plan (once per plan), as events for the one gateway that serves the artifact.
func replaySubscriptions(ctx context.Context, tx *gorm.DB, source subplanv1.Source, artifacts SubscriptionArtifacts,
	events *GatewayEventsService, p gwmodels.Placement,
) error {
	apiID := p.ArtifactUUID.String()
	kind, handle, found, err := artifacts.ProxyOf(ctx, p.OUID, apiID)
	if err != nil || !found {
		return err
	}
	subs, err := source.ListSubscriptions(ctx, p.OUID, kind, handle)
	if err != nil {
		return fmt.Errorf("listing the subscriptions to %s: %w", handle, err)
	}
	gatewayID := p.GatewayUUID.String()
	sentPlans := map[string]bool{}
	for _, sub := range subs {
		if sub.PlanID != "" && !sentPlans[sub.PlanID] {
			sentPlans[sub.PlanID] = true
			plan, found, err := source.GetPlan(ctx, p.OUID, sub.PlanID)
			if err != nil {
				return fmt.Errorf("reading plan %s of subscription %s: %w", sub.PlanID, sub.ID, err)
			}
			if found {
				eventType, action, payload, err := planEvent(subplanv1.Created, plan)
				if err != nil {
					return err
				}
				if err := events.publishTx(ctx, tx, gatewayID, eventType, action, plan.ID, payload); err != nil {
					return err
				}
			}
		}
		perArtifact := sub
		perArtifact.ID = GatewaySubscriptionID(sub.ID, apiID)
		eventType, action, payload, err := gatewaySubscriptionEvent(subplanv1.Created, apiID, perArtifact)
		if err != nil {
			return err
		}
		if err := events.publishTx(ctx, tx, gatewayID, eventType, action, perArtifact.ID, payload); err != nil {
			return err
		}
	}
	if len(subs) > 0 {
		slog.InfoContext(ctx, "Subscriptions sent to the gateway that now serves an MCP proxy deployment",
			"orgId", p.OUID, "proxy", handle, "apiId", apiID, "gatewayId", gatewayID, "subscriptions", len(subs),
			"plans", len(sentPlans))
	}
	return nil
}

// gatewaySubscriptionEvent is the gateway event for a subscription change to the deployed artifact apiID: its type,
// action and payload.
func gatewaySubscriptionEvent(action subplanv1.Action, apiID string, sub subplanv1.Subscription) (string, string, any, error) {
	switch action {
	case subplanv1.Created:
		return "subscription.created", "CREATE", SubscriptionEvent{
			APIID: apiID, SubscriptionID: sub.ID, SubscriptionToken: sub.Token, SubscriptionPlanID: sub.PlanID,
			Status: sub.Status,
		}, nil
	case subplanv1.Updated:
		return "subscription.updated", "UPDATE", SubscriptionEvent{
			APIID: apiID, SubscriptionID: sub.ID, SubscriptionToken: sub.Token, SubscriptionPlanID: sub.PlanID,
			Status: sub.Status,
		}, nil
	case subplanv1.Deleted:
		return "subscription.deleted", "DELETE", SubscriptionDeletedEvent{APIID: apiID, SubscriptionID: sub.ID}, nil
	}
	return "", "", nil, fmt.Errorf("unknown subscription action %q", action)
}

var _ SubscriptionGateways = (*SubscriptionPlanBroadcaster)(nil)

// SubscriptionChanged sends subscription.created, .updated or .deleted to every gateway of the organization.
// A gateway that is not connected gets the subscriptions when it connects (it pulls them).
func (b *SubscriptionPlanBroadcaster) SubscriptionChanged(ctx context.Context, orgID string, action subplanv1.Action,
	apiID string, sub subplanv1.Subscription,
) error {
	eventType, gatewayAction, payload, err := gatewaySubscriptionEvent(action, apiID, sub)
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
		if err := b.events.broadcastEvent(gw.UUID.String(), eventType, gatewayAction, sub.ID, payload); err != nil {
			slog.WarnContext(ctx, "Failed to send a subscription event to a gateway",
				"gatewayId", gw.UUID.String(), "type", eventType, "subscriptionId", sub.ID, "error", err)
			errs = append(errs, err)
		}
	}
	slog.InfoContext(ctx, "Subscription event sent to gateways", "type", eventType, "subscriptionId", sub.ID,
		"apiId", apiID, "orgId", orgID, "gateways", len(gateways), "failed", len(errs))
	return errors.Join(errs...)
}
