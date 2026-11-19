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

package gwctl

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/logger"
)

// GetPlatformGatewayFailureSummary serves the platform-wide gateway failure
// summary: how many gateways exist across every organization, how many have been
// disconnected past the configured staleness threshold, and what share of the
// fleet that is.
//
// The status code carries the verdict, so the route can be pointed at a monitor
// directly: 200 while the failure percentage is under the configured threshold,
// 503 at or above it. Two things about that are deliberate:
//
//   - 503 here describes the gateway fleet, not this service. Anything that
//     alerts on non-2xx responses from agent-manager-service will see these and
//     should exclude this route, or it will read a degraded fleet as a broken
//     control plane.
//   - A detailed request always answers 200. Details are for a human or a
//     dashboard reading the response body, and an error status there would make
//     the payload look like the error rather than the diagnosis. The verdict is
//     still in the body: "healthy" is present on every response, in both forms,
//     and is the field a caller should branch on.
//
// Unlike every other handler here it does not read an org from the request —
// there is no org to read: the route carries no {orgName}, and the host lets
// only a platform admin reach it (modkit.PlatformAdmin, set by its registrar).
func (c *gatewayController) GetPlatformGatewayFailureSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	failure := c.gatewayService.FailureSettings()
	includeDetails := r.URL.Query().Get("includeDetails") == "true"

	summary, err := c.gatewayService.GetCrossOrgGatewayFailureSummary(ctx, gwsvc.GatewayFailureSummaryQuery{
		StalenessThreshold:         failure.Threshold,
		MaxAge:                     failure.MaxAge,
		FailurePercentageThreshold: failure.PercentageThreshold,
		IncludeDetails:             includeDetails,
	})
	if err != nil {
		log.Error("GetPlatformGatewayFailureSummary: failed to build summary", "error", err)
		handleGatewayErrors(w, err, "Failed to get gateway failure summary")
		return
	}

	// Logged on the verdict, not on the status. A dashboard polling with
	// includeDetails=true gets 200 by design, and gating this warning on the
	// status too would mean a degraded fleet produced no log line at all for
	// exactly the caller most likely to be watching it.
	if !summary.Healthy {
		log.Warn("gateway fleet is over the failure threshold",
			"failed", summary.Failed, "considered", summary.Considered,
			"abandoned", summary.Abandoned, "total", summary.Total,
			"failurePercentage", summary.FailurePercentage,
			"thresholdPercentage", summary.FailurePercentageThreshold)
	}

	status := http.StatusOK
	if !summary.Healthy && !includeDetails {
		status = http.StatusServiceUnavailable
	}

	// WriteSuccessResponse writes the payload under the status it is given; the
	// name distinguishes a plain body from WriteErrorResponse's error envelope,
	// not 2xx from 5xx. A degraded fleet is reported as the same document either
	// way, because the caller wants the numbers whichever side of the threshold
	// they fall on.
	httpx.WriteSuccessResponse(w, status, summary)
}
