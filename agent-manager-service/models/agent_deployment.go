// Copyright (c) 2025, WSO2 LLC. (https://www.wso2.com).
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

package models

// DeploymentResponse is where an agent is reached in one environment.
type DeploymentResponse struct {
	AgentName              string     `json:"agentName"`
	ProjectName            string     `json:"projectName"`
	Status                 string     `json:"status"`
	Environment            string     `json:"environment"`
	EnvironmentDisplayName string     `json:"environmentDisplayName"`
	Endpoints              []Endpoint `json:"endpoints"`
	// EnvironmentUUID is the environment's id.
	EnvironmentUUID string `json:"-"`
	// GatewayStatus and GatewayStatusReason are the agent's deployment on its environment's ingress gateway; empty
	// for an agent the gateway does not serve.
	GatewayStatus       string `json:"gatewayStatus,omitempty"`
	GatewayStatusReason string `json:"gatewayStatusReason,omitempty"`
}

// Endpoint represents endpoint configuration
type Endpoint struct {
	URL        string `json:"url"`
	Name       string `json:"name,omitempty"`
	Visibility string `json:"visibility,omitempty"`
}

// EnvVars represents environment variables
type EnvVars struct {
	Key         string `json:"key"`
	Value       string `json:"value"`
	IsSensitive bool   `json:"isSensitive,omitempty"`
	SecretRef   string `json:"secretRef,omitempty"`
	SecretKey   string `json:"secretKey,omitempty"` // The key within the secret (e.g., "api-key")
	IsSystem    bool   `json:"isSystem,omitempty"`  // Platform-injected (e.g. LLM_PROVIDER_URL); read-only for users
}
