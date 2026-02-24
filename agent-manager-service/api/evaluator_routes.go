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

func registerEvaluatorRoutes(mux *http.ServeMux, controller controllers.EvaluatorController) {
	// GET /orgs/{orgName}/evaluators - List evaluators
	mux.HandleFunc("GET /orgs/{orgName}/evaluators", controller.ListEvaluators)

	// GET /orgs/{orgName}/evaluators/llm-providers - List supported LLM providers
	mux.HandleFunc("GET /orgs/{orgName}/evaluators/llm-providers", controller.ListLLMProviders)

	// GET /orgs/{orgName}/evaluators/{evaluatorId} - Get evaluator details
	mux.HandleFunc("GET /orgs/{orgName}/evaluators/{evaluatorId}", controller.GetEvaluator)
}
