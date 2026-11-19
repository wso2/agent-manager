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
	"fmt"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
)

// PlanChangeHandler pushes a committed plan change (subplanv1.EventPlanChanged) to the organization's
// gateways. A created or updated plan is read back from the module that owns plans, so the gateways get its
// current state with every field they take; while that module is inactive nothing is pushed, and gateways
// pull every plan when they reconnect. A deleted plan is sent by its id, handle and name.
func PlanChangeHandler(plans func() (subplanv1.Source, bool), gateways subplanv1.Gateways) modkit.EventHandler {
	return func(ctx context.Context, e modkit.Event) error {
		var ch subplanv1.PlanChange
		if err := json.Unmarshal(e.Data, &ch); err != nil {
			return fmt.Errorf("decoding %s: %w", e.Type, err)
		}
		orgID := ch.OrgID
		if orgID == "" {
			orgID = e.OrgID
		}
		switch ch.Action {
		case subplanv1.Deleted:
			return gateways.PlanChanged(ctx, orgID, ch.Action,
				subplanv1.Plan{ID: ch.PlanID, Handle: ch.Handle, Name: ch.PlanName})
		case subplanv1.Created, subplanv1.Updated:
		default:
			return fmt.Errorf("plan change %q has an unknown action %q", ch.Handle, ch.Action)
		}
		source, ok := plans()
		if !ok {
			return nil
		}
		list, err := source.ListPlans(ctx, orgID)
		if err != nil {
			return fmt.Errorf("reading plan %s: %w", ch.Handle, err)
		}
		for _, p := range list {
			if p.ID == ch.PlanID {
				return gateways.PlanChanged(ctx, orgID, ch.Action, p)
			}
		}
		// Deleted since: its own deletion event follows.
		return nil
	}
}
