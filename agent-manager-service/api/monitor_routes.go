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

package api

import (
	"net/http"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/controllers"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/middleware"
)

func route(method, path string) string {
	return method + " " + path
}

func registerMonitorScoreRoutes(mux *http.ServeMux, controller controllers.MonitorScoresController) {
	agentBase := "/orgs/{orgName}/projects/{projName}/agents/{agentName}"
	monitorBase := agentBase + "/monitors/{monitorName}"

	// GET .../monitors/{monitorName}/scores - Get scores for a monitor (time-range based)
	// Query params: startTime, endTime, evaluator (optional), level (optional), span_type (optional)
	middleware.HandleFuncWithValidation(mux, route("GET", monitorBase+"/scores"), controller.GetMonitorScores)

	// GET .../monitors/{monitorName}/runs/{runId}/scores - Get per-run aggregated scores
	middleware.HandleFuncWithValidation(mux, route("GET", monitorBase+"/runs/{runId}/scores"), controller.GetMonitorRunScores)

	// GET .../monitors/{monitorName}/scores/timeseries - Get time-series data for an evaluator
	// Query params: startTime, endTime, evaluator (required), granularity (optional: hour/day/week)
	middleware.HandleFuncWithValidation(mux, route("GET", monitorBase+"/scores/timeseries"), controller.GetScoresTimeSeries)

	// GET .../agents/{agentName}/traces/{traceId}/scores - Get all evaluation scores for a trace across all monitors
	middleware.HandleFuncWithValidation(mux, route("GET", agentBase+"/traces/{traceId}/scores"), controller.GetTraceScores)
}

func registerMonitorRoutes(mux *http.ServeMux, controller controllers.MonitorController) {
	base := "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors"

	// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors - List all monitors
	middleware.HandleFuncWithValidation(mux, route("GET", base), controller.ListMonitors)

	// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors - Create a new evaluation monitor
	middleware.HandleFuncWithValidation(mux, route("POST", base), controller.CreateMonitor)

	// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName} - Get a specific monitor
	middleware.HandleFuncWithValidation(mux, route("GET", base+"/{monitorName}"), controller.GetMonitor)

	// DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName} - Delete a monitor
	middleware.HandleFuncWithValidation(mux, route("DELETE", base+"/{monitorName}"), controller.DeleteMonitor)

	// PATCH /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName} - Update a monitor
	middleware.HandleFuncWithValidation(mux, route("PATCH", base+"/{monitorName}"), controller.UpdateMonitor)

	// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/stop - Stop a monitor
	middleware.HandleFuncWithValidation(mux, route("POST", base+"/{monitorName}/stop"), controller.StopMonitor)

	// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/start - Start a monitor
	middleware.HandleFuncWithValidation(mux, route("POST", base+"/{monitorName}/start"), controller.StartMonitor)

	// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs - List monitor runs
	middleware.HandleFuncWithValidation(mux, route("GET", base+"/{monitorName}/runs"), controller.ListMonitorRuns)

	// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs/{runId}/rerun - Create a new run with same time parameters
	middleware.HandleFuncWithValidation(mux, route("POST", base+"/{monitorName}/runs/{runId}/rerun"), controller.RerunMonitor)

	// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs/{runId}/logs - Get monitor run logs
	middleware.HandleFuncWithValidation(mux, route("GET", base+"/{monitorName}/runs/{runId}/logs"), controller.GetMonitorRunLogs)
}
