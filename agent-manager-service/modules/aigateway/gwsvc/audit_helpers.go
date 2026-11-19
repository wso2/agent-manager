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

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwaudit"
)

// apiKeyAuditTarget identifies the key an audit record is about.
//
// API-key lifecycle is spread over three services (agent, LLM provider, LLM
// proxy) that share one action name. Routing them through one helper keeps
// the records identical in shape, so a query for "every key rotated last
// week" cannot miss one service because its emit site drifted.
type apiKeyAuditTarget struct {
	// OwnerType is one of the gwaudit.APIKeyOwner* constants. Without it the
	// trail cannot tell an agent key from an LLM-provider key, since both
	// carry the same action.
	OwnerType string
	OwnerID   string
	OwnerName string
	KeyName   string
}

// annotateAPIKey names the key the request's audit record is about. The key
// routes are fail-closed (modkit.AuditFailClosed): keys are minted and
// broadcast to gateways outside this service, so the core writes the intent
// record before the route runs and refuses it when that fails. The key value
// is never passed in.
func annotateAPIKey(ctx context.Context, target apiKeyAuditTarget) {
	modkit.AuditResource(ctx, gwaudit.ResourceAPIKey, target.OwnerID, target.KeyName)
	modkit.AuditDetail(ctx, "ownerType", target.OwnerType)
	modkit.AuditDetail(ctx, "ownerName", target.OwnerName)
	modkit.AuditDetail(ctx, "keyName", target.KeyName)
}
