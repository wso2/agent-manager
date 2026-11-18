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
	"sync/atomic"

	"github.com/wso2/agent-manager/agent-manager-service/modkit/contracts/identityv1"
)

// IdentityDirectory is the gateway's way to identity.directory/v1, which the core provides. Activate points it
// at the module's Runtime; until then, or when the Runtime has none, every environment has no directory.
type IdentityDirectory struct {
	lookup atomic.Pointer[func() (identityv1.Directory, bool)]
}

// NewIdentityDirectory returns a directory with nothing to look up yet.
func NewIdentityDirectory() *IdentityDirectory { return &IdentityDirectory{} }

// Use sets how the directory is looked up.
func (d *IdentityDirectory) Use(lookup func() (identityv1.Directory, bool)) { d.lookup.Store(&lookup) }

// For opens the environment's directory through the capability.
func (d *IdentityDirectory) For(ctx context.Context, orgID, environment string) (identityv1.Client, error) {
	if lookup := d.lookup.Load(); lookup != nil {
		if dir, ok := (*lookup)(); ok {
			return dir.For(ctx, orgID, environment)
		}
	}
	return nil, fmt.Errorf("%w: %s is not available", identityv1.ErrNotProvisioned, identityv1.ID)
}

var _ identityv1.Directory = (*IdentityDirectory)(nil)
