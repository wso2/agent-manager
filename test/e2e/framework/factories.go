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

package framework

// NewCreateProjectRequest returns a project creation request bound to the given
// deployment pipeline.
func NewCreateProjectRequest(name, displayName, description, deploymentPipeline string) CreateProjectRequest {
	return CreateProjectRequest{
		Name:               name,
		DisplayName:        displayName,
		Description:        &description,
		DeploymentPipeline: deploymentPipeline,
	}
}

// NewExternalAgentRequest returns a request for creating an external agent.
func NewExternalAgentRequest(name, description string) CreateAgentRequest {
	return CreateAgentRequest{
		Name:        name,
		DisplayName: name,
		Description: description,
		Provisioning: Provisioning{
			Type: "external",
		},
		AgentType: AgentType{
			Type:    "external-agent-api",
			SubType: "custom-api",
		},
	}
}
