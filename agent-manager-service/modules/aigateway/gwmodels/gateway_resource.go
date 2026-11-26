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

package gwmodels

import (
	"encoding/json"
	"time"
)

// A gateway pushes the resources it created through its own API — kinds that are not
// deployable artifacts — the way it pushes artifacts (import-gateway-artifacts): one item
// after each create or update, a deletion when one is deleted, and its complete list on
// connect. A resource is matched by its kind and name, and the control plane mints its own
// ID, which the gateway stores as cp_artifact_id.

// GatewayResourceKindBudget is the Budget kind a gateway syncs.
const GatewayResourceKindBudget = "Budget"

// Per-resource outcomes of a gateway resource push.
const (
	GatewayResourceStatusSynced = "synced"
	GatewayResourceStatusFailed = "failed"
)

// GatewayResourceItem is one resource in a push: the document as the gateway stores it
// (its kind is configuration.kind), keyed by the gateway's ID for it.
type GatewayResourceItem struct {
	DPID          string          `json:"dpid"`
	Configuration json.RawMessage `json:"configuration"`
	CreatedAt     time.Time       `json:"createdAt"`
	UpdatedAt     time.Time       `json:"updatedAt"`
}

// GatewayResourcesRequest is the body of POST /api/internal/v1/gateways/{gatewayId}/resources.
// Resources are created or updated and Deleted are removed. Complete marks Resources as
// everything the gateway created: the gateway's other resources are then removed too.
type GatewayResourcesRequest struct {
	Complete  bool                  `json:"complete"`
	Resources []GatewayResourceItem `json:"resources"`
	Deleted   []GatewayResourceItem `json:"deleted"`
}

// GatewayResourceResult is the outcome for one resource. ID is the control plane's ID for
// it, set once it is stored.
type GatewayResourceResult struct {
	ID     string `json:"id,omitempty"`
	Status string `json:"status"`
	Error  string `json:"error,omitempty"`
}

// GatewayResourcesResponse holds a result per resource, keyed by dpid, with counts.
type GatewayResourcesResponse struct {
	Total     int                              `json:"total"`
	Success   int                              `json:"success"`
	Failed    int                              `json:"failed"`
	Resources map[string]GatewayResourceResult `json:"resources"`
}
