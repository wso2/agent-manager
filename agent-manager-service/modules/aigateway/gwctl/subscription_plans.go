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

package gwctl

import (
	"sync/atomic"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/subplanv1"
)

// planSource finds the module that owns subscription plans ("subscription.plans/v1"). Activate sets it on
// every activation, so a lookup sees the current set of active modules.
var planSource atomic.Pointer[func() (subplanv1.Source, bool)]

// UseSubscriptionPlans sets how the controller API finds the plans its gateways pull.
func UseSubscriptionPlans(lookup func() (subplanv1.Source, bool)) { planSource.Store(&lookup) }

func subscriptionPlanSource() (subplanv1.Source, bool) {
	if f := planSource.Load(); f != nil {
		return (*f)()
	}
	return nil, false
}
