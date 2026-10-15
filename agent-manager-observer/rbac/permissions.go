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

// Package rbac defines the permission catalog enforced by
// agent-manager-observer. It mirrors agent-manager-service's rbac package
// convention (the two are separate Go modules); wire-format scopes are
// amp:<resource>:<action>, registered on the Thunder "amp" resource server.
package rbac

// Permission is the <resource>:<action> part of an amp:<resource>:<action>
// OAuth scope.
type Permission string

// ResourceServer is the OAuth2 resource-server handle under which all AMP
// permissions are registered in Thunder.
const ResourceServer = "amp"

// Scope returns the wire-format OAuth scope string for the permission.
func (p Permission) Scope() string { return ResourceServer + ":" + string(p) }

// Observability data-read permissions, one per observer data endpoint family.
const (
	TraceRead      Permission = "observability:trace-read"
	LogRead        Permission = "observability:log-read"
	BuildLogRead   Permission = "observability:build-log-read"
	MonitorLogRead Permission = "observability:monitor-log-read"
	MetricRead     Permission = "observability:metric-read"
)
