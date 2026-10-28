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

package controllers

import (
	"net/http"
	"time"

	"github.com/wso2/agent-manager/agent-manager-service/config"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/logger"
	"github.com/wso2/agent-manager/agent-manager-service/services"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
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
// there is no org to read. It deliberately does not call
// middleware.OUIDFromRequest either: this route carries no {orgName}, so the
// registrar never ran RequireOrgMatch and that value would be empty. Reading it
// and passing it as a filter would silently return an empty fleet.
func (c *gatewayController) GetPlatformGatewayFailureSummary(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	log := logger.GetLogger(ctx)

	// The platform-admin gate is what stands between an ordinary tenant token
	// and every tenant's rows, and it is applied at registration where a future
	// refactor could drop it without breaking the build. Confirming it ran
	// turns that mistake into a refusal instead of a cross-tenant disclosure.
	if !middleware.IsPlatformAdminRequest(ctx) {
		log.Error("GetPlatformGatewayFailureSummary: reached without the platform-admin guard; " +
			"the route is registered without HandleFuncWithValidationAndPlatformAdminAuthz")
		utils.WriteErrorResponse(w, http.StatusForbidden, "forbidden")
		return
	}

	cfg := config.GetConfig()
	includeDetails := r.URL.Query().Get("includeDetails") == "true"

	summary, err := c.gatewayService.GetCrossOrgGatewayFailureSummary(ctx, services.GatewayFailureSummaryQuery{
		StalenessThreshold:         time.Duration(cfg.GatewayFailureThresholdSeconds) * time.Second,
		MaxAge:                     time.Duration(cfg.GatewayFailureMaxAgeSeconds) * time.Second,
		FailurePercentageThreshold: cfg.GatewayFailurePercentageThreshold,
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
	utils.WriteSuccessResponse(w, status, summary)
}
