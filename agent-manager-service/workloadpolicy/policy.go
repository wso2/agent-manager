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

// Package workloadpolicy carries a deployment-specific workload admission check.
// The default is unrestricted; cloud deployments install a subscription check.
package workloadpolicy

import (
	"context"
	"errors"
)

// ErrSuspended identifies attempts to start work for a suspended subscription.
var ErrSuspended = errors.New("SUBSCRIPTION_SUSPENDED")

// Guard checks the trusted organization identity before starting new work.
type Guard func(context.Context, string) error

type guardKey struct{}

// WithGuard installs a guard for request and background operations.
func WithGuard(ctx context.Context, guard Guard) context.Context {
	return context.WithValue(ctx, guardKey{}, guard)
}

// Check applies the deployment's guard. A nil guard preserves standalone behavior.
func Check(ctx context.Context, orgID string) error {
	if guard, ok := ctx.Value(guardKey{}).(Guard); ok && guard != nil {
		return guard(ctx, orgID)
	}
	return nil
}
