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

// RegisterLLMRoutes registers all LLM-related routes
func RegisterLLMRoutes(mux *http.ServeMux, ctrl controllers.LLMController) {
	// LLM Provider Templates
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/llm-provider-templates", ctrl.CreateLLMProviderTemplate)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-provider-templates", ctrl.ListLLMProviderTemplates)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-provider-templates/{id}", ctrl.GetLLMProviderTemplate)
	middleware.HandleFuncWithValidation(mux, "PUT /orgs/{orgName}/llm-provider-templates/{id}", ctrl.UpdateLLMProviderTemplate)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/llm-provider-templates/{id}", ctrl.DeleteLLMProviderTemplate)

	// LLM Providers
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/llm-providers", ctrl.CreateLLMProvider)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-providers", ctrl.ListLLMProviders)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-providers/{id}", ctrl.GetLLMProvider)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-providers/{id}/llm-proxies", ctrl.ListLLMProxiesByProvider)
	middleware.HandleFuncWithValidation(mux, "PUT /orgs/{orgName}/llm-providers/{id}", ctrl.UpdateLLMProvider)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/llm-providers/{id}", ctrl.DeleteLLMProvider)

	// LLM Proxies
	middleware.HandleFuncWithValidation(mux, "POST /orgs/{orgName}/llm-proxies", ctrl.CreateLLMProxy)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-proxies", ctrl.ListLLMProxies)
	middleware.HandleFuncWithValidation(mux, "GET /orgs/{orgName}/llm-proxies/{id}", ctrl.GetLLMProxy)
	middleware.HandleFuncWithValidation(mux, "PUT /orgs/{orgName}/llm-proxies/{id}", ctrl.UpdateLLMProxy)
	middleware.HandleFuncWithValidation(mux, "DELETE /orgs/{orgName}/llm-proxies/{id}", ctrl.DeleteLLMProxy)
}
