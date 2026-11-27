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

	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
	"github.com/wso2/agent-manager/cli/pkg/clierr"
)

// gatewayPageLimit is the page size ListAllGateways requests: the contract's maximum.
const gatewayPageLimit = int32(100)

// ListAllGateways returns every gateway in the org, following the response's
// pagination metadata rather than trusting one page. A single unpaginated request
// reads as the whole set but is capped at the server's default page size, which
// made a gateway past that boundary invisible: resolving it by name failed as
// "unknown gateway", and completion offered a silently truncated list.
//
// Returns a clierr on transport failure or a non-2xx response, along with the pages
// gathered before it. Callers that need the complete set must treat a non-nil error
// as fatal; a best-effort caller such as shell completion can still use the partial
// result, which beats offering nothing when a later page times out.
func ListAllGateways(ctx context.Context, client *aigateway.ClientWithResponses, org string) ([]aigateway.GatewayDocument, error) {
	var all []aigateway.GatewayDocument
	for {
		limit, offset := gatewayPageLimit, int32(len(all))
		resp, err := client.ListGatewaysWithResponse(ctx, &aigateway.ListGatewaysParams{
			Limit:  &limit,
			Offset: &offset,
		})
		if err != nil {
			return all, clierr.Newf(clierr.Transport, "list gateways: %v", err)
		}
		if resp.JSON200 == nil {
			return all, ErrorFromServer(resp.HTTPResponse,
				FirstNonNil(resp.JSON400, resp.JSON401, resp.JSON500))
		}
		page := resp.JSON200.List
		all = append(all, page...)
		// An empty page terminates regardless of Total: a Total the server
		// over-reports would otherwise loop forever.
		if len(page) == 0 || resp.JSON200.Pagination == nil || resp.JSON200.Pagination.Total == nil ||
			int32(len(all)) >= *resp.JSON200.Pagination.Total {
			return all, nil
		}
	}
}
