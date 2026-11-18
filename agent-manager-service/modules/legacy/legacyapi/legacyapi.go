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

// Package legacyapi registers the routes of features this version does not
// serve (hosted agents, agent kinds, repositories, git secrets, monitors),
// which answer 501, and the evaluator catalog.
package legacyapi

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/modules/legacy/evaluators"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// RegisterRoutes registers the legacy routes on the authenticated /api/v1
// router.
func RegisterRoutes(rr *middleware.RouteRegistrar, evaluatorCtrl evaluators.EvaluatorController) {
	registerHostedRoutes(rr)
	registerAgentKindRoutes(rr)
	registerRepositoryRoutes(rr)
	RegisterGitSecretRoutes(rr)
	registerMonitorRoutes(rr)
	registerMonitorScoreRoutes(rr)
	registerMonitorAlertRoutes(rr)
	RegisterMonitorPublisherRoutes(rr)
	registerEvaluatorRoutes(rr, evaluatorCtrl)
}

// notImplemented answers 501 for a route this version does not serve.
func notImplemented(w http.ResponseWriter, _ *http.Request) {
	utils.WriteErrorResponse(w, http.StatusNotImplemented, "This operation is not available in this version")
}
