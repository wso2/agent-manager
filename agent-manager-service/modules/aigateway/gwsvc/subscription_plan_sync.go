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
// Deliveries for one plan run one at a time on a replica: each reads the plan and sends it under the plan's
// lock, so of two overlapping deliveries the later one reads, and sends, the later state.
func PlanChangeHandler(plans func() (subplanv1.Source, bool), gateways PlanGateways) modkit.EventHandler {
	var locks planLocks
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
		case subplanv1.Deleted, subplanv1.Created, subplanv1.Updated:
		default:
			return fmt.Errorf("plan change %q has an unknown action %q", ch.Handle, ch.Action)
		}
		unlock := locks.lock(orgID + "/" + ch.Handle)
		defer unlock()
		if ch.Action == subplanv1.Deleted {
			return gateways.PlanChanged(ctx, orgID, ch.Action,
				subplanv1.Plan{ID: ch.PlanID, Handle: ch.Handle, Name: ch.PlanName})
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

// planLocks holds a lock per plan while a delivery for it runs.
type planLocks struct {
	mu    sync.Mutex
	locks map[string]*planLock
}

type planLock struct {
	sync.Mutex
	users int
}

func (l *planLocks) lock(key string) (unlock func()) {
	l.mu.Lock()
	if l.locks == nil {
		l.locks = map[string]*planLock{}
	}
	pl := l.locks[key]
	if pl == nil {
		pl = &planLock{}
		l.locks[key] = pl
	}
	pl.users++
	l.mu.Unlock()
	pl.Lock()
	return func() {
		pl.Unlock()
		l.mu.Lock()
		if pl.users--; pl.users == 0 {
			delete(l.locks, key)
		}
		l.mu.Unlock()
	}
}
