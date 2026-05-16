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

package traces

import (
	"context"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
)

// requireEnvironment verifies that env exists in org via the agent-manager
// service, so that callers can surface "Environment not found" the same way
// `agent metrics` does, instead of the generic 500 from traces-observer.
func requireEnvironment(ctx context.Context, client *amsvc.ClientWithResponses, org, env string) error {
	if env == "" {
		return nil
	}
	resp, err := client.GetEnvironmentWithResponse(ctx, org, env)
	if err != nil {
		return clierr.Newf(clierr.Transport, "%v", err)
	}
	if resp.JSON200 != nil {
		return nil
	}
	return cmdutil.ErrorFromServer(resp.HTTPResponse,
		cmdutil.FirstNonNil(resp.JSON404, resp.JSON400, resp.JSON401, resp.JSON500))
}

func preflightEnv(ctx context.Context, amClient func(context.Context) (*amsvc.ClientWithResponses, error), org, env string) error {
	if amClient == nil {
		return nil
	}
	client, err := amClient(ctx)
	if err != nil {
		return err
	}
	return requireEnvironment(ctx, client, org, env)
}
