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

package cmdutil

import (
	"context"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

// ValidateBuildable fetches the agent and verifies it supports build operations.
// Returns nil if the agent is buildable, or a CLIError explaining why not.
func ValidateBuildable(ctx context.Context, client *amsvc.ClientWithResponses, org, proj, agentName string) error {
	resp, err := client.GetAgentWithResponse(ctx, org, proj, agentName)
	if err != nil {
		return clierr.Newf(clierr.Transport, "%v", err)
	}
	if resp.JSON200 == nil {
		return ErrorFromServer(resp.HTTPResponse, FirstNonNil(resp.JSON404, resp.JSON500))
	}
	if !IsBuildable(*resp.JSON200) {
		return clierr.Newf(clierr.Validation,
			"agent %q does not support builds\n  Only internally-provisioned agents can be built.", agentName)
	}
	return nil
}
