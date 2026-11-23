//go:build integration

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

package gwctl_test

import (
	"net/http"
	"testing"

	"github.com/google/uuid"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
)

// llmProviderDoc is a provider document on the built-in openai template.
func llmProviderDoc(handle string, gateways ...string) map[string]any {
	spec := map[string]any{
		"displayName": "Provider " + handle,
		"version":     "v1.0",
		"context":     "/" + handle,
		"template":    "openai",
		"upstream": map[string]any{
			"main": map[string]any{
				"url":  "https://api.openai.com/v1",
				"auth": map[string]any{"type": "api-key", "header": "Authorization", "value": "Bearer sk-test"},
			},
		},
	}
	if gateways != nil {
		spec["gateways"] = gateways
	}
	return map[string]any{
		"apiVersion": "ai-control-plane.choreolabs.com/v1",
		"kind":       "LlmProvider",
		"metadata":   map[string]any{"name": handle},
		"spec":       spec,
	}
}

// llmCreateProvider creates a provider through the API and returns its document.
func llmCreateProvider(t *testing.T, a *routeAPI, handle string, gateways ...string) gwspec.LlmProviderDocument {
	t.Helper()
	doc := expectInto[gwspec.LlmProviderDocument](t, a.do(http.MethodPost, "/llm-providers", llmProviderDoc(handle, gateways...)), http.StatusCreated)
	require.NotNil(t, doc.Metadata.Uid)
	return doc
}

// llmProxyBody is a create request for a proxy on providerUUID.
func llmProxyBody(providerUUID, name string) map[string]any {
	return map[string]any{
		"providerUuid": providerUUID,
		"description":  "proxy " + name,
		"configuration": map[string]any{
			"name":     name,
			"version":  "v1.0",
			"context":  "/" + name,
			"provider": providerUUID,
		},
	}
}

// llmCreateProxy creates a proxy in project through the API and returns it.
func llmCreateProxy(t *testing.T, a *routeAPI, project, providerUUID, name string) gwspec.LLMProxyResponse {
	t.Helper()
	return expectInto[gwspec.LLMProxyResponse](t,
		a.do(http.MethodPost, "/orgs/{org}/projects/"+project+"/llm-proxies", llmProxyBody(providerUUID, name)), http.StatusCreated)
}

// llmProxyUUID is the proxy's id as a UUID.
func llmProxyUUID(t *testing.T, p gwspec.LLMProxyResponse) uuid.UUID {
	t.Helper()
	id, err := uuid.Parse(p.Uuid)
	require.NoError(t, err)
	return id
}
