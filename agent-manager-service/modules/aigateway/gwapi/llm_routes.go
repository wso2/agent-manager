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

package gwapi

import (
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// RegisterLLMRoutes registers all LLM-related routes
func RegisterLLMRoutes(rr Router, ctrl gwctl.LLMController) {
	// LLM Provider Templates
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/llm-provider-templates", gwperm.LLMProviderTemplateCreate,
		rr.Track("amp.connections.create-llm-provider.template", actionDims("created-provider-template"), ctrl.CreateLLMProviderTemplate))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-provider-templates", gwperm.LLMProviderTemplateRead, ctrl.ListLLMProviderTemplates)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-provider-templates/{templateId}", gwperm.LLMProviderTemplateRead, ctrl.GetLLMProviderTemplate)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/llm-provider-templates/{templateId}", gwperm.LLMProviderTemplateUpdate,
		rr.Track("amp.connections.create-llm-provider.template", actionDims("updated-provider-template"), ctrl.UpdateLLMProviderTemplate))
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/llm-provider-templates/{templateId}", gwperm.LLMProviderTemplateDelete,
		rr.Track("amp.connections.create-llm-provider.template", actionDims("deleted-provider-template"), ctrl.DeleteLLMProviderTemplate))

	registerLLMProviderRoutes(rr, ctrl)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/policies", gwperm.LLMProviderRead, ctrl.ListAvailableLLMPolicies)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/{providerId}/llm-proxies", gwperm.LLMProxyRead, ctrl.ListLLMProxiesByProvider)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/llm-providers/{providerId}/consumers", gwperm.LLMProviderRead, ctrl.ListLLMProviderConsumers)

	// LLM Proxies
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/projects/{projName}/llm-proxies", gwperm.LLMProxyCreate, ctrl.CreateLLMProxy)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/llm-proxies", gwperm.LLMProxyRead, ctrl.ListLLMProxies)
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", gwperm.LLMProxyRead, ctrl.GetLLMProxy)
	rr.HandleFuncWithValidationAndAuthz("PUT /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", gwperm.LLMProxyUpdate, ctrl.UpdateLLMProxy)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/projects/{projName}/llm-proxies/{proxyId}", gwperm.LLMProxyDelete, ctrl.DeleteLLMProxy)
}

// registerLLMProviderRoutes mounts the provider documents. Catalog membership is spec.inCatalog on the PUT.
func registerLLMProviderRoutes(rr Router, ctrl gwctl.LLMController) {
	rr.HandleFuncWithValidationAndAuthz("POST /llm-providers", gwperm.LLMProviderCreate,
		rr.Track("amp.connections.create-llm-provider", actionDims("created-llm-provider"), ctrl.CreateLLMProvider))
	rr.HandleFuncWithValidationAndAuthz("GET /llm-providers", gwperm.LLMProviderRead, ctrl.ListLLMProviders)
	rr.HandleFuncWithValidationAndAuthz("GET /llm-providers/{providerId}", gwperm.LLMProviderRead, ctrl.GetLLMProvider)
	rr.HandleFuncWithValidationAndAuthz("PUT /llm-providers/{providerId}", gwperm.LLMProviderUpdate,
		rr.Track("amp.connections.create-llm-provider", actionDims("updated-llm-provider"), ctrl.UpdateLLMProvider))
	rr.HandleFuncWithValidationAndAuthz("DELETE /llm-providers/{providerId}", gwperm.LLMProviderDelete,
		rr.Track("amp.connections.create-llm-provider", actionDims("deleted-llm-provider"), ctrl.DeleteLLMProvider))
}
