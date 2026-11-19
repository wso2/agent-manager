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
)

// Permissions answers whether the caller of the current request holds one of
// the module's permissions. Activate points it at the module's Runtime; until
// then every check fails.
type Permissions struct {
	can atomic.Pointer[func(ctx context.Context, perm string) bool]
}

// NewPermissions returns permissions that hold nothing yet.
func NewPermissions() *Permissions { return &Permissions{} }

// Use sets how a permission is checked: Runtime.Can, which takes the
// permission's manifest name.
func (p *Permissions) Use(can func(ctx context.Context, perm string) bool) { p.can.Store(&can) }

// Can reports whether the caller holds perm, by its manifest name.
func (p *Permissions) Can(ctx context.Context, perm string) bool {
	if can := p.can.Load(); can != nil {
		return (*can)(ctx, perm)
	}
	return false
}
