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
	"sync/atomic"

	"gorm.io/gorm"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/notifyv1"
)

// Notifications is the gateway's way to notifications/v1, which the core provides.
// Activate points it at the module's Runtime; until then, or when the Runtime has none,
// nothing is sent and every channel name is accepted.
type Notifications struct {
	lookup atomic.Pointer[func() (notifyv1.Notifier, bool)]
}

// NewNotifications returns a notifier with nothing to look up yet.
func NewNotifications() *Notifications { return &Notifications{} }

// Use sets how the notifier is looked up.
func (n *Notifications) Use(lookup func() (notifyv1.Notifier, bool)) { n.lookup.Store(&lookup) }

func (n *Notifications) get() (notifyv1.Notifier, bool) {
	if lookup := n.lookup.Load(); lookup != nil {
		return (*lookup)()
	}
	return nil, false
}

// Notify sends through the capability, or does nothing without one.
func (n *Notifications) Notify(ctx context.Context, tx *gorm.DB, in notifyv1.Notification) error {
	if notifier, ok := n.get(); ok {
		return notifier.Notify(ctx, tx, in)
	}
	return nil
}

// Unusable checks through the capability, or accepts every name without one.
func (n *Notifications) Unusable(ctx context.Context, orgID, project string, names []string) ([]notifyv1.Unusable, error) {
	if notifier, ok := n.get(); ok {
		return notifier.Unusable(ctx, orgID, project, names)
	}
	return nil, nil
}

var _ notifyv1.Notifier = (*Notifications)(nil)
