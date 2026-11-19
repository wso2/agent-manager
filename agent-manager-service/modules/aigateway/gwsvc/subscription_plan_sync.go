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
	"sync"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
)

// PlanGateways sends a plan change to every gateway of the organization (SubscriptionPlanBroadcaster). For a
// deleted plan only its ID, Handle and Name are read.
type PlanGateways interface {
	PlanChanged(ctx context.Context, orgID string, action subplanv1.Action, plan subplanv1.Plan) error
}

// PlanChangeHandler pushes a committed plan change (the event the plans module names with
// subplanv1.Source.ChangeEvent) to the organization's gateways. A created or updated plan is read back from the
// module that owns plans, so the gateways get its current state with every field they take; while that module
// is inactive nothing is pushed, and gateways pull every plan when they reconnect. A deleted plan is sent by its
// id, handle and name.
//
// Deliveries can overlap, so a delivery that read an older state of a plan than one already sent from this
// replica sends nothing: the newer state is on the gateways. The check holds no lock across I/O. Every pushed
// plan carries its updatedAt, so a gateway can also drop an older state sent by another replica.
func PlanChangeHandler(plans func() (subplanv1.Source, bool), gateways PlanGateways) modkit.EventHandler {
	var sent sentVersions
	return func(ctx context.Context, e modkit.Event) error {
		var ch subplanv1.PlanChange
		if err := json.Unmarshal(e.Data, &ch); err != nil {
			return fmt.Errorf("decoding %s: %w", e.Type, err)
		}
		orgID := ch.OrgID
		if orgID == "" {
			orgID = e.OrgID
		}
		key := orgID + "/" + ch.PlanID
		switch ch.Action {
		case subplanv1.Deleted:
			sent.forget(key)
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
			if p.ID != ch.PlanID {
				continue
			}
			if !sent.claim(key, p.UpdatedAt) {
				return nil
			}
			return gateways.PlanChanged(ctx, orgID, ch.Action, p)
		}
		// Deleted since: its own deletion event follows.
		return nil
	}
}

// sentVersions remembers, per plan, the newest state this replica has sent (its updatedAt).
type sentVersions struct {
	mu   sync.Mutex
	last map[string]time.Time
}

// claim records at as the plan's newest sent state and reports true, unless a newer state was sent already.
// An equal state is sent again, so a redelivered event still reaches the gateways.
func (v *sentVersions) claim(key string, at time.Time) bool {
	v.mu.Lock()
	defer v.mu.Unlock()
	if v.last == nil {
		v.last = map[string]time.Time{}
	}
	if prev, ok := v.last[key]; ok && at.Before(prev) {
		return false
	}
	v.last[key] = at
	return true
}

// forget drops a deleted plan, so the map holds only plans that exist.
func (v *sentVersions) forget(key string) {
	v.mu.Lock()
	defer v.mu.Unlock()
	delete(v.last, key)
}
