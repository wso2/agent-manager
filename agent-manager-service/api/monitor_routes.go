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
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

func route(method, path string) string {
	return method + " " + path
}

func registerMonitorScoreRoutes(rr *middleware.RouteRegistrar, controller controllers.MonitorScoresController) {
	agentBase := "/orgs/{orgName}/projects/{projName}/agents/{agentName}"
	monitorBase := agentBase + "/monitors/{monitorName}"

	// GET .../monitors/{monitorName}/scores - Get scores for a monitor (time-range based)
	// Query params: startTime, endTime, evaluator (optional), level (optional), span_type (optional)
	rr.HandleFuncWithValidationAndAuthz(route("GET", monitorBase+"/scores"), rbac.MonitorScoreRead, controller.GetMonitorScores)

	// GET .../monitors/{monitorName}/runs/{runId}/scores - Get per-run aggregated scores
	rr.HandleFuncWithValidationAndAuthz(route("GET", monitorBase+"/runs/{runId}/scores"), rbac.MonitorScoreRead, controller.GetMonitorRunScores)

	// GET .../monitors/{monitorName}/scores/breakdown - Get scores grouped by span label (agent/LLM breakdown)
	// Query params: startTime, endTime, level (required: "agent" or "llm")
	rr.HandleFuncWithValidationAndAuthz(route("GET", monitorBase+"/scores/breakdown"), rbac.MonitorScoreRead, controller.GetGroupedScores)

	// GET .../monitors/{monitorName}/scores/timeseries - Get time-series data for an evaluator
	// Query params: startTime, endTime, evaluator (required), granularity (optional: hour/day/week)
	rr.HandleFuncWithValidationAndAuthz(route("GET", monitorBase+"/scores/timeseries"), rbac.MonitorScoreRead, controller.GetScoresTimeSeries)

	// GET .../agents/{agentName}/scores - Aggregated scores per trace for an agent
	// Query params: startTime, endTime
	rr.HandleFuncWithValidationAndAuthz(route("GET", agentBase+"/scores"), rbac.MonitorScoreRead, controller.GetAgentTraceScores)

	// GET .../agents/{agentName}/traces/{traceId}/scores - Get all evaluation scores for a trace across all monitors
	rr.HandleFuncWithValidationAndAuthz(route("GET", agentBase+"/traces/{traceId}/scores"), rbac.MonitorScoreRead, controller.GetTraceScores)
}

func registerMonitorRoutes(rr *middleware.RouteRegistrar, controller controllers.MonitorController) {
	base := "/orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors"

	// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors - List all monitors
	rr.HandleFuncWithValidationAndAuthz(route("GET", base), rbac.MonitorRead, controller.ListMonitors)

	// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors - Create a new evaluation monitor
	rr.HandleFuncWithValidationAndAuthz(route("POST", base), rbac.MonitorCreate,
		growthanalytics.Track("amp.observability.create-monitor", actionDims("created-monitor"), controller.CreateMonitor))

	// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName} - Get a specific monitor
	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/{monitorName}"), rbac.MonitorRead, controller.GetMonitor)

	// DELETE /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName} - Delete a monitor
	rr.HandleFuncWithValidationAndAuthz(route("DELETE", base+"/{monitorName}"), rbac.MonitorDelete,
		growthanalytics.Track("amp.observability.create-monitor", actionDims("deleted-monitor"), controller.DeleteMonitor))

	// PATCH /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName} - Update a monitor
	rr.HandleFuncWithValidationAndAuthz(route("PATCH", base+"/{monitorName}"), rbac.MonitorUpdate,
		growthanalytics.Track("amp.observability.create-monitor", actionDims("updated-monitor"), controller.UpdateMonitor))

	// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/stop - Stop a monitor
	rr.HandleFuncWithValidationAndAuthz(route("POST", base+"/{monitorName}/stop"), rbac.MonitorExecute,
		growthanalytics.Track("amp.observability.create-monitor.start-stop", actionDims("stopped-monitor"), controller.StopMonitor))

	// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/start - Start a monitor
	rr.HandleFuncWithValidationAndAuthz(route("POST", base+"/{monitorName}/start"), rbac.MonitorExecute,
		growthanalytics.Track("amp.observability.create-monitor.start-stop", actionDims("started-monitor"), controller.StartMonitor))

	// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs - List monitor runs
	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/{monitorName}/runs"), rbac.MonitorRead, controller.ListMonitorRuns)

	// POST /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs/{runId}/rerun - Create a new run with same time parameters
	rr.HandleFuncWithValidationAndAuthz(route("POST", base+"/{monitorName}/runs/{runId}/rerun"), rbac.MonitorExecute,
		growthanalytics.Track("amp.observability.create-monitor.rerun", actionDims("reran-monitor"), controller.RerunMonitor))

	// GET /orgs/{orgName}/projects/{projName}/agents/{agentName}/monitors/{monitorName}/runs/{runId}/logs - Get monitor run logs
	rr.HandleFuncWithValidationAndAuthz(route("GET", base+"/{monitorName}/runs/{runId}/logs"), rbac.MonitorRead, controller.GetMonitorRunLogs)
}
