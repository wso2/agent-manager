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

func registerEvaluatorRoutes(rr *middleware.RouteRegistrar, controller controllers.EvaluatorController) {
	// GET /orgs/{orgName}/evaluators - List evaluators (built-in + custom merged)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/evaluators", rbac.EvaluatorRead, controller.ListEvaluators)

	// Custom evaluator CRUD — registered before the {evaluatorId} catch-all
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/evaluators/custom", rbac.EvaluatorCreate,
		growthanalytics.Track("amp.observability.custom-evaluator", actionDims("created-custom-evaluator"), controller.CreateCustomEvaluator))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/evaluators/custom/{identifier}", rbac.EvaluatorRead, controller.GetCustomEvaluator)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/evaluators/custom/{identifier}", rbac.EvaluatorUpdate,
		growthanalytics.Track("amp.observability.custom-evaluator", actionDims("updated-custom-evaluator"), controller.UpdateCustomEvaluator))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/evaluators/custom/{identifier}", rbac.EvaluatorDelete,
		growthanalytics.Track("amp.observability.custom-evaluator", actionDims("deleted-custom-evaluator"), controller.DeleteCustomEvaluator))

	// GET /orgs/{orgName}/evaluators/{evaluatorId} - Get evaluator details (built-in or custom)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/evaluators/{evaluatorId}", rbac.EvaluatorRead, controller.GetEvaluator)
}
