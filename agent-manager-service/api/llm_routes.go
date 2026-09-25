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
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RegisterLLMRoutes registers all LLM-related routes
func RegisterLLMRoutes(rr *middleware.RouteRegistrar, ctrl controllers.LLMController) {
	// LLM Provider Templates
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/llm-provider-templates", rbac.LLMProviderTemplateCreate, ctrl.CreateLLMProviderTemplate)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-provider-templates", rbac.LLMProviderTemplateRead, ctrl.ListLLMProviderTemplates)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-provider-templates/{templateId}", rbac.LLMProviderTemplateRead, ctrl.GetLLMProviderTemplate)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/llm-provider-templates/{templateId}", rbac.LLMProviderTemplateUpdate, ctrl.UpdateLLMProviderTemplate)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/llm-provider-templates/{templateId}", rbac.LLMProviderTemplateDelete, ctrl.DeleteLLMProviderTemplate)

	// LLM Providers
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/llm-providers", rbac.LLMProviderCreate, ctrl.CreateLLMProvider)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers", rbac.LLMProviderRead, ctrl.ListLLMProviders)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/policies", rbac.LLMProviderRead, ctrl.ListAvailableLLMPolicies)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/{providerId}", rbac.LLMProviderRead, ctrl.GetLLMProvider)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/{providerId}/llm-proxies", rbac.LLMProxyRead, ctrl.ListLLMProxiesByProvider)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/{providerId}/consumers", rbac.LLMProviderRead, ctrl.ListLLMProviderConsumers)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/llm-providers/{providerId}", rbac.LLMProviderUpdate, ctrl.UpdateLLMProvider)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/llm-providers/{providerId}/catalog", rbac.LLMProviderUpdate, ctrl.UpdateLLMProviderCatalogStatus)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/llm-providers/{providerId}", rbac.LLMProviderDelete, ctrl.DeleteLLMProvider)
	// Gated on EvaluatorCreate, not an llm-provider permission: this route exists to
	// author an evaluator, and whoever may create one may use a provider to draft it.
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/llm-providers/{providerId}/generate-evaluator", rbac.EvaluatorCreate, ctrl.GenerateEvaluatorCode)

	// LLM Proxies
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/llm-proxies", rbac.LLMProxyCreate, ctrl.CreateLLMProxy)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/llm-proxies", rbac.LLMProxyRead, ctrl.ListLLMProxies)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", rbac.LLMProxyRead, ctrl.GetLLMProxy)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", rbac.LLMProxyUpdate, ctrl.UpdateLLMProxy)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", rbac.LLMProxyDelete, ctrl.DeleteLLMProxy)
}
