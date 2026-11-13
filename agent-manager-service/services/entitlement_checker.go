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

package services

import "context"

// EntitlementChecker asks whether the organization's subscription allows a
// create before Agent Manager makes it.
//
// In open-source deployments this is nil and every create is allowed. A
// deployment that sells plans with limits can inject an implementation (via
// app.Options) that asks its billing system.
//
// CheckCreate returns nil when the create may go ahead, and an error wrapping
// utils.ErrEntitlementDenied (usually a *utils.EntitlementDeniedError carrying
// the message to show) when the subscription refuses it. Any other error stops
// the create as a failure. An implementation that lets creates through while
// its billing system is unreachable returns nil in that case.
type EntitlementChecker interface {
	// CheckCreate checks creating quantity more resources of resourceType in the
	// organization ouID. resourceType is the component type the
	// create will use, such as "proxy/agent-api".
	CheckCreate(ctx context.Context, ouID, resourceType string, quantity int) error
}
