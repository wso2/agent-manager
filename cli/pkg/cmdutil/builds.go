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

// GetBuild fetches a build of the agent, returning a CLIError (NOT_FOUND for an
// unknown build) when the server has no such build.
func GetBuild(ctx context.Context, client *amsvc.ClientWithResponses, org, proj, agentName, buildName string) (*amsvc.BuildDetailsResponse, error) {
	resp, err := client.GetBuildWithResponse(ctx, org, proj, agentName, buildName)
	if err != nil {
		return nil, clierr.Newf(clierr.Transport, "%v", err)
	}
	if resp.JSON200 == nil {
		return nil, ErrorFromServer(resp.HTTPResponse, FirstNonNil(resp.JSON404, resp.JSON500))
	}
	return resp.JSON200, nil
}
