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
	"sync/atomic"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
)

// SubscriptionPlans is the gateway's way to subscription.plans/v1, which the module that owns plans provides.
// Activate points it at the module's Runtime on every activation, so a lookup sees the current set of active
// modules; until then, or while no module provides it, there are no plans.
type SubscriptionPlans struct {
	lookup atomic.Pointer[func() (subplanv1.Source, bool)]
}

// NewSubscriptionPlans returns a holder with nothing to look up yet.
func NewSubscriptionPlans() *SubscriptionPlans { return &SubscriptionPlans{} }

// Use sets how the plans are looked up.
func (p *SubscriptionPlans) Use(lookup func() (subplanv1.Source, bool)) { p.lookup.Store(&lookup) }

// Source returns the module that owns the plans, when one is active.
func (p *SubscriptionPlans) Source() (subplanv1.Source, bool) {
	if lookup := p.lookup.Load(); lookup != nil {
		return (*lookup)()
	}
	return nil, false
}
