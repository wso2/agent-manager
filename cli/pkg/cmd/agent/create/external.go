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

package create

import (
	"context"
	"fmt"
	"strings"

	amsvc "github.com/wso2/agent-manager/cli/pkg/clients/amsvc/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
	"github.com/wso2/agent-manager/cli/pkg/cmdutil"
	"github.com/wso2/agent-manager/cli/pkg/render"
)

// 1 year — matches the MCP create_agent flow.
const externalTokenExpiresIn = "8760h"

func runExternalPostCreate(ctx context.Context, opts *CreateOptions, agent *amsvc.AgentResponse, client *amsvc.ClientWithResponses) error {
	tc, err := opts.TraceObserver(ctx)
	if err != nil {
		return err
	}
	traceObsURL := tc.URL()

	expires := externalTokenExpiresIn
	body := amsvc.TokenRequest{ExpiresIn: &expires}

	tokenResp, err := client.GenerateAgentTokenWithResponse(ctx, opts.Org, opts.Proj, agent.Name, nil, body)
	if err != nil {
		return clierr.Newf(clierr.Transport, "generate agent token: %v", err)
	}
	if tokenResp.JSON200 == nil {
		return cmdutil.ErrorFromServer(tokenResp.HTTPResponse, cmdutil.FirstNonNil(tokenResp.JSON400, tokenResp.JSON401, tokenResp.JSON404))
	}

	endpoint := otelIngestEndpoint(traceObsURL)
	instructions := buildPythonInstructions(endpoint, tokenResp.JSON200.Token)

	if opts.IO.JSON {
		return render.JSONSuccess(opts.IO, opts.Scope, map[string]any{
			"agent":                       agent,
			"token":                       tokenResp.JSON200.Token,
			"tokenExpiresAt":              tokenResp.JSON200.ExpiresAt,
			"otelEndpoint":                endpoint,
			"instrumentationInstructions": instructions,
		})
	}

	fmt.Fprintln(opts.IO.ErrOut)
	fmt.Fprintln(opts.IO.ErrOut, instructions)
	return nil
}

func otelIngestEndpoint(base string) string {
	return strings.TrimRight(base, "/") + "/v1/traces"
}

// The MCP server keeps its own copy at agent-manager-service/mcp/tools/agents.go;
// the duplication is intentional and avoids a cli -> agent-manager-service Go dependency.
func buildPythonInstructions(otelEndpoint, token string) string {
	return fmt.Sprintf(`Follow these steps to enable instrumentation:

  1. Install the AMP instrumentation package:
     pip install amp-instrumentation

  2. Export the following environment variables in the agent's runtime environment:
     export AMP_OTEL_ENDPOINT=%q
     export AMP_AGENT_API_KEY=%q

  3. Run the agent with instrumentation enabled:
     amp-instrument <your_existing_start_command>`, otelEndpoint, token)
}
