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
	"fmt"
	"log/slog"
	"slices"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwrepo"
)

// Gateway events that carry control-plane budgets (Interfaces 4.1).
const (
	BudgetEventCreated = "budget.created"
	BudgetEventUpdated = "budget.updated"
	BudgetEventDeleted = "budget.deleted"
)

// BudgetGatewayPayload is a budget as a gateway gets it, in budget.created and
// budget.updated events and from GET /api/internal/v1/budgets. A gateway ignores one whose
// updatedAt is older than its stored copy.
type BudgetGatewayPayload struct {
	BudgetID  string     `json:"budgetId"`
	UpdatedAt time.Time  `json:"updatedAt"`
	Budget    BudgetKind `json:"budget"`
}

// budgetDeletedPayload is the payload of budget.deleted.
type budgetDeletedPayload struct {
	BudgetID string `json:"budgetId"`
}

// BudgetGatewaySender sends control-plane budgets to the gateways that enforce them.
type BudgetGatewaySender interface {
	// SendBudget sends a created or updated budget.
	SendBudget(ctx context.Context, b *gwmodels.Budget, created bool)
	// SendBudgetDeleted tells the gateways a budget was deleted.
	SendBudgetDeleted(ctx context.Context, b *gwmodels.Budget)
}

// BudgetGateways sends a control-plane budget to the gateways that enforce it: every
// egress gateway of its organization. Gateways' own budgets are never sent back to them.
type BudgetGateways struct {
	events      *GatewayEventsService
	gatewayRepo gwrepo.GatewayRepository
	budgetRepo  gwrepo.BudgetRepository
}

// NewBudgetGateways creates a BudgetGateways.
func NewBudgetGateways(
	events *GatewayEventsService,
	gatewayRepo gwrepo.GatewayRepository,
	budgetRepo gwrepo.BudgetRepository,
) *BudgetGateways {
	return &BudgetGateways{events: events, gatewayRepo: gatewayRepo, budgetRepo: budgetRepo}
}

// gatewayPayload is the budget as gateways get it.
func gatewayPayload(b *gwmodels.Budget) BudgetGatewayPayload {
	return BudgetGatewayPayload{
		BudgetID: b.ID.String(), UpdatedAt: b.UpdatedAt.UTC(), Budget: BuildBudgetKind(b.Name, b.OrgName, b.Configuration, true),
	}
}

// SendBudget sends a created or updated budget to the gateways it covers. A failure is
// logged: the gateway gets the budget when it next connects (GET /budgets).
func (g *BudgetGateways) SendBudget(ctx context.Context, b *gwmodels.Budget, created bool) {
	eventType, action := BudgetEventUpdated, "UPDATE"
	if created {
		eventType, action = BudgetEventCreated, "CREATE"
	}
	g.send(ctx, b, eventType, action, gatewayPayload(b))
}

// SendBudgetDeleted tells the gateways a budget covered that it was deleted.
func (g *BudgetGateways) SendBudgetDeleted(ctx context.Context, b *gwmodels.Budget) {
	g.send(ctx, b, BudgetEventDeleted, "DELETE", budgetDeletedPayload{BudgetID: b.ID.String()})
}

func (g *BudgetGateways) send(ctx context.Context, b *gwmodels.Budget, eventType, action string, payload any) {
	if b.Origin != gwmodels.BudgetOriginControlPlane {
		return
	}
	gateways, err := g.targets(ctx, b.OrgID)
	if err != nil {
		slog.Error("Failed to find the gateways for a budget; they get it when they next connect",
			"budget", b.Name, "event", eventType, "error", err)
		return
	}
	for _, gatewayID := range gateways {
		// Keyed by the budget's ID, so a reconnecting gateway is replayed only its latest state.
		if err := g.events.broadcastEvent(gatewayID, eventType, action, b.ID.String(), payload); err != nil {
			slog.Error("Failed to send a budget to a gateway; it gets it when it next connects",
				"budget", b.Name, "gatewayID", gatewayID, "event", eventType, "error", err)
		}
	}
}

// targets returns the IDs of the organization's egress gateways.
func (g *BudgetGateways) targets(ctx context.Context, ouID string) ([]string, error) {
	gateways, err := g.gatewayRepo.ListWithFiltersCtx(ctx, gwrepo.GatewayFilterOptions{
		OrganizationID: ouID, FunctionalityTypeIn: gwmodels.EgressGatewayRoles,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to list gateways: %w", err)
	}
	out := make([]string, 0, len(gateways))
	for _, gw := range gateways {
		out = append(out, gw.UUID.String())
	}
	return out, nil
}

// ForGatewayID is ForGateway for the gateway with that ID in the organization.
func (g *BudgetGateways) ForGatewayID(ctx context.Context, ouID, gatewayID string) ([]BudgetGatewayPayload, error) {
	gateway, err := g.gatewayRepo.GetByUUID(gatewayID)
	if err != nil {
		return nil, fmt.Errorf("failed to load gateway %s: %w", gatewayID, err)
	}
	if gateway == nil || gateway.OUID != ouID {
		return nil, gwerr.ErrGatewayNotFound
	}
	return g.ForGateway(ctx, ouID, gateway)
}

// ForGateway returns the control-plane budgets a gateway enforces, as the gateway gets
// them: what GET /api/internal/v1/budgets answers when it connects.
func (g *BudgetGateways) ForGateway(ctx context.Context, ouID string, gateway *gwmodels.Gateway) ([]BudgetGatewayPayload, error) {
	out := []BudgetGatewayPayload{}
	if !slices.Contains(gwmodels.EgressGatewayRoles, gateway.GatewayFunctionalityType) {
		return out, nil
	}
	budgets, err := g.budgetRepo.List(ctx, ouID)
	if err != nil {
		return nil, err
	}
	for i := range budgets {
		b := &budgets[i]
		if b.Origin == gwmodels.BudgetOriginControlPlane {
			out = append(out, gatewayPayload(b))
		}
	}
	return out, nil
}
