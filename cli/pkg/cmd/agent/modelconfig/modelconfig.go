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

// Package modelconfig holds helpers shared by the agent llm and mcp commands.
package modelconfig

import (
	aigateway "github.com/wso2/agent-manager/cli/pkg/clients/aigateway/gen"
)

// StringPtr returns a pointer to v.
func StringPtr(v string) *string {
	return &v
}

// OptionalString returns a pointer to s, or nil when s is empty.
func OptionalString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

// BuildEnvVars maps the url/apikey env-var flags to request config; nil when neither is set.
func BuildEnvVars(urlEnv, apiKeyEnv string) *[]aigateway.EnvironmentVariableConfig {
	var evs []aigateway.EnvironmentVariableConfig
	if urlEnv != "" {
		evs = append(evs, aigateway.EnvironmentVariableConfig{Key: "url", Name: urlEnv})
	}
	if apiKeyEnv != "" {
		evs = append(evs, aigateway.EnvironmentVariableConfig{Key: "apikey", Name: apiKeyEnv})
	}
	if len(evs) == 0 {
		return nil
	}
	return &evs
}

// MergeExistingEnvMappings translates every env mapping of an existing config into request shape.
func MergeExistingEnvMappings(
	resp *aigateway.AgentModelConfigResponse,
	toRequest func(aigateway.EnvProviderConfigMappings) aigateway.EnvModelConfigRequest,
) map[string]aigateway.EnvModelConfigRequest {
	out := make(map[string]aigateway.EnvModelConfigRequest, len(resp.EnvMappings))
	for env, m := range resp.EnvMappings {
		out[env] = toRequest(m)
	}
	return out
}
