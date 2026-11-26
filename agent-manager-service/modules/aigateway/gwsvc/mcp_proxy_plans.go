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
	"slices"
	"strings"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

const planStatusActive = "ACTIVE"

// offeredPlans checks the subscription plans an MCP proxy is asked to offer and returns the handles to store:
// trimmed, without repeats, in the order given. A nil request is returned as nil, which on update keeps the
// stored list; an empty list offers none.
//   - Each added plan must exist in the organization, be active and not be deprecated.
//   - A list sent back unchanged is kept as it is, even with a plan since deactivated or deprecated, so saving
//     another part of the proxy works (also while the plans module is off).
//   - A changed list can't keep a deprecated plan: it must be removed before the offer changes otherwise.
//
// Plans come from the module that owns them (subscription.plans/v1), so without it no plan can be added.
func (s *MCPProxyService) offeredPlans(ctx context.Context, orgUUID string, requested *[]string, current []string) ([]string, error) {
	if requested == nil {
		return nil, nil
	}
	handles := make([]string, 0, len(*requested))
	for _, h := range *requested {
		if h = strings.TrimSpace(h); h != "" && !slices.Contains(handles, h) {
			handles = append(handles, h)
		}
	}
	if len(handles) == 0 {
		if len(*requested) > 0 {
			return nil, fmt.Errorf("%w: subscriptionPlans must name a plan; send an empty list to offer none",
				httpx.ErrInvalidInput)
		}
		return handles, nil
	}
	if sameHandles(handles, current) {
		return handles, nil
	}
	added := slices.DeleteFunc(slices.Clone(handles), func(h string) bool { return slices.Contains(current, h) })
	source, ok := s.planSource()
	if !ok {
		if len(added) == 0 {
			// Only removals, which need no plan: allowed while the plans module is off.
			return handles, nil
		}
		return nil, fmt.Errorf("%w: subscription plans are not available, as the AI Hub module is not installed",
			httpx.ErrInvalidInput)
	}
	plans, err := source.ListPlans(ctx, orgUUID)
	if err != nil {
		return nil, fmt.Errorf("failed to list subscription plans: %w", err)
	}
	byHandle := make(map[string]subplanv1.Plan, len(plans))
	for _, p := range plans {
		byHandle[p.Handle] = p
	}
	for _, h := range handles {
		if p, found := byHandle[h]; found && p.Deprecated {
			if slices.Contains(added, h) {
				return nil, fmt.Errorf("%w: subscription plan %q is deprecated and can't be offered", httpx.ErrInvalidInput, h)
			}
			return nil, fmt.Errorf("%w: subscription plan %q is deprecated; remove it before changing the offered plans",
				httpx.ErrInvalidInput, h)
		}
	}
	for _, h := range added {
		if p, found := byHandle[h]; !found || p.Status != planStatusActive {
			return nil, fmt.Errorf("%w: subscription plan %q was not found or is not active", httpx.ErrInvalidInput, h)
		}
	}
	return handles, nil
}

// planSource is the module that owns plans, when one is active.
func (s *MCPProxyService) planSource() (subplanv1.Source, bool) {
	if s.plans == nil {
		return nil, false
	}
	return s.plans.Source()
}

// sameHandles reports whether two lists name the same plans, in any order.
func sameHandles(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for _, h := range a {
		if !slices.Contains(b, h) {
			return false
		}
	}
	return true
}

// offeredPlansDTO is the list a response shows: never nil, so a proxy that offers no plan shows [].
func offeredPlansDTO(handles []string) *[]string {
	out := append([]string{}, handles...)
	return &out
}

// PlanOffers counts the MCP proxies that offer a plan. The AI Gateway hands it to the module that owns plans
// (subplanv1.Source.CountOffersWith), which reports it as the plan's usage.
type PlanOffers struct{ db *gorm.DB }

// NewPlanOffers returns the counter over the module's MCP proxy table.
func NewPlanOffers(db *gorm.DB) PlanOffers { return PlanOffers{db: db} }

// ProxiesOffering counts the organization's MCP proxies whose configuration lists the plan. The list is in a
// JSON column, read the same way on every database.
func (o PlanOffers) ProxiesOffering(ctx context.Context, orgID, handle string) (int, error) {
	var proxies []gwmodels.MCPProxy
	if err := o.db.WithContext(ctx).Select("id", "configuration").
		Where("org_id = ?", orgID).Find(&proxies).Error; err != nil {
		return 0, fmt.Errorf("failed to read MCP proxy plans: %w", err)
	}
	n := 0
	for _, p := range proxies {
		if slices.Contains(p.Configuration.SubscriptionPlans, handle) {
			n++
		}
	}
	return n, nil
}
