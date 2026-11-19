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
	"fmt"
	"strings"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
)

// Gateway sync, as the gateways expect it from any control plane: a gateway pulls every plan of its
// organization when it connects (planSource, the "subscription.plans/v1" capability), and each plan change is
// pushed to the organization's gateways once it is committed (syncPlanChange, on the module's own
// plan-changed event, through the gateway's "gateway.plans/v1" capability).

// gatewayUnits are the throttle units a gateway takes for its single limit.
var gatewayUnits = map[string]bool{"MINUTE": true, "HOUR": true, "DAY": true, "MONTH": true}

// planSource lists an organization's plans for its gateways.
type planSource struct{ plans *planService }

// pageSize bounds one read of the plans; ListPlans reads them all, page by page.
const pageSize = 500

func (s planSource) ListPlans(ctx context.Context, orgID string) ([]subplanv1.Plan, error) {
	var out []subplanv1.Plan
	for offset := 0; ; offset += pageSize {
		page, _, err := s.plans.list(ctx, orgID, listOptions{Limit: pageSize, Offset: offset})
		if err != nil {
			return nil, err
		}
		for _, p := range page {
			out = append(out, gatewayPlan(p.UUID, p.Handle, p.Name, p.Status, p.Conditions, p))
		}
		if len(page) < pageSize {
			return out, nil
		}
	}
}

// gatewayPlan is a plan as gateways see it. full carries the dates when the caller has the stored plan.
func gatewayPlan(id, handle, name, status string, conditions []Condition, full *plan) subplanv1.Plan {
	out := subplanv1.Plan{ID: id, Handle: handle, Name: name, Status: status, StopOnQuotaReach: true}
	for _, c := range conditions {
		out.Conditions = append(out.Conditions, subplanv1.Condition{ID: c.ID, Type: c.Type, Params: c.Params})
	}
	if count, unit, stop, ok := throttleLimit(conditions); ok {
		out.ThrottleLimitCount, out.ThrottleLimitUnit, out.StopOnQuotaReach = &count, unit, stop
	}
	if full != nil {
		out.ExpiryTime, out.CreatedAt, out.UpdatedAt = full.ExpiryTime, full.CreatedAt, full.UpdatedAt
	}
	return out
}

// throttleLimit is the single limit of a gateway that reads only that: the first window of the request-count
// condition that lasts one unit the gateway knows. stop is false when the condition lets requests through.
func throttleLimit(conditions []Condition) (count int, unit string, stop bool, ok bool) {
	for _, c := range conditions {
		if c.Type != ConditionTypeRequestCount {
			continue
		}
		var params Limits
		if err := json.Unmarshal(c.Params, &params); err != nil {
			return 0, "", false, false
		}
		for _, w := range params.Limits {
			u := strings.ToUpper(w.TimeUnit)
			if (w.TimeAmount == 0 || w.TimeAmount == 1) && gatewayUnits[u] && w.Count > 0 {
				return w.Count, u, params.OnExceed != OnExceedAllow, true
			}
		}
		return 0, "", false, false
	}
	return 0, "", false, false
}

// syncPlanChange pushes a committed plan change to the organization's gateways. Without the gateway (a
// distribution or set of modules that has none) there is nothing to push to.
func syncPlanChange(rt modkit.Runtime) modkit.EventHandler {
	return func(ctx context.Context, e modkit.Event) error {
		impl, ok := rt.Capability(subplanv1.GatewayID)
		if !ok {
			return nil
		}
		gateways, ok := impl.(subplanv1.Gateways)
		if !ok {
			return fmt.Errorf("%s is not a subplanv1.Gateways", subplanv1.GatewayID)
		}
		var ch planChanged
		if err := json.Unmarshal(e.Data, &ch); err != nil {
			return fmt.Errorf("decoding %s: %w", e.Type, err)
		}
		action := subplanv1.Action(ch.Action)
		switch action {
		case subplanv1.Created, subplanv1.Updated, subplanv1.Deleted:
		default:
			return fmt.Errorf("plan change %q has an unknown action %q", ch.Handle, ch.Action)
		}
		p := gatewayPlan(ch.PlanID, ch.Handle, ch.PlanName, ch.Status, ch.Conditions, nil)
		p.ExpiryTime = ch.ExpiryTime
		orgID := ch.OrgID
		if orgID == "" {
			orgID = e.OrgID
		}
		return gateways.PlanChanged(ctx, orgID, action, p)
	}
}
