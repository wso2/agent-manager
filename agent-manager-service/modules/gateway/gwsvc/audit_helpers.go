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

	"github.com/wso2/agent-manager/agent-manager-service/audit"
)

// apiKeyAuditTarget identifies the key an audit record is about.
//
// API-key lifecycle is spread over three services (agent, LLM provider, LLM
// proxy) that share one permission and one action name. Routing them through
// one helper keeps the records identical in shape, so a query for "every key
// rotated last week" cannot miss one service because its emit site drifted.
type apiKeyAuditTarget struct {
	OUID string
	// OwnerType is one of the audit.APIKeyOwner* constants. Without it the
	// trail cannot tell an agent key from an LLM-provider key, since both
	// carry the same action.
	OwnerType string
	OwnerID   string
	OwnerName string
	KeyName   string
	// Project and Environment scope the key where that applies; agent keys are
	// per-environment, provider keys are org-wide.
	Project     string
	Environment string
}

// beginAPIKeyAudit records the intent to change a key and reports whether the
// operation may proceed.
//
// Keys are minted and broadcast to gateways outside this service, so the change
// and its record cannot commit together. Recording intent first, and refusing
// when that fails, is what stops a live credential existing with no trace of
// who created it. The key value is never passed in.
func beginAPIKeyAudit(
	ctx context.Context,
	action audit.Action,
	target apiKeyAuditTarget,
	extra ...audit.Option,
) (*audit.Attempt, error) {
	opts := []audit.Option{
		audit.Org(target.OUID),
		audit.ResourceNamed(audit.ResourceAPIKey, target.OwnerID, target.KeyName),
		audit.Detail("ownerType", target.OwnerType),
		audit.Detail("ownerName", target.OwnerName),
		audit.Detail("keyName", target.KeyName),
	}
	if target.Project != "" {
		opts = append(opts, audit.Project(target.Project))
	}
	if target.Environment != "" {
		opts = append(opts, audit.Environment(target.Environment))
	}
	opts = append(opts, extra...)

	return audit.Begin(ctx, action, opts...)
}
