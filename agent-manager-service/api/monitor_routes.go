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
)

func registerMonitorRoutes(mux *http.ServeMux, controller controllers.MonitorController) {
	// GET /orgs/{orgName}/monitors - List all monitors
	mux.HandleFunc("GET /orgs/{orgName}/monitors", controller.ListMonitors)

	// POST /orgs/{orgName}/monitors - Create a new evaluation monitor
	mux.HandleFunc("POST /orgs/{orgName}/monitors", controller.CreateMonitor)

	// GET /orgs/{orgName}/monitors/{monitorName} - Get a specific monitor
	mux.HandleFunc("GET /orgs/{orgName}/monitors/{monitorName}", controller.GetMonitor)

	// DELETE /orgs/{orgName}/monitors/{monitorName} - Delete a monitor
	mux.HandleFunc("DELETE /orgs/{orgName}/monitors/{monitorName}", controller.DeleteMonitor)

	// PATCH /orgs/{orgName}/monitors/{monitorName} - Update a monitor
	mux.HandleFunc("PATCH /orgs/{orgName}/monitors/{monitorName}", controller.UpdateMonitor)

	// POST /orgs/{orgName}/monitors/{monitorName}/stop - Stop a monitor
	mux.HandleFunc("POST /orgs/{orgName}/monitors/{monitorName}/stop", controller.StopMonitor)

	// POST /orgs/{orgName}/monitors/{monitorName}/start - Start a monitor
	mux.HandleFunc("POST /orgs/{orgName}/monitors/{monitorName}/start", controller.StartMonitor)

	// GET /orgs/{orgName}/monitors/{monitorName}/runs - List monitor runs
	mux.HandleFunc("GET /orgs/{orgName}/monitors/{monitorName}/runs", controller.ListMonitorRuns)

	// POST /orgs/{orgName}/monitors/{monitorName}/runs/{runId}/rerun - Create a new run with same time parameters
	mux.HandleFunc("POST /orgs/{orgName}/monitors/{monitorName}/runs/{runId}/rerun", controller.RerunMonitor)

	// GET /orgs/{orgName}/monitors/{monitorName}/runs/{runId}/logs - Get monitor run logs
	mux.HandleFunc("GET /orgs/{orgName}/monitors/{monitorName}/runs/{runId}/logs", controller.GetMonitorRunLogs)
}
