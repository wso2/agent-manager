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

import "encoding/json"

// NewCreateProjectRequest returns a project creation request with default values.
func NewCreateProjectRequest(name, displayName, description string) CreateProjectRequest {
	return CreateProjectRequest{
		Name:               name,
		DisplayName:        displayName,
		Description:        &description,
		DeploymentPipeline: "default",
	}
}

// NewInternalChatAgentRequest returns a request for creating an internal chat agent
// with build config, environment variables, and input interface pre-configured.
// Keys "TAVILY_API_KEY" and "OPENAI_API_KEY" are automatically marked as sensitive.
func NewInternalChatAgentRequest(name, description string, envVars map[string]string) CreateAgentRequest {
	autoInstr := true
	sensitiveKeys := map[string]bool{
		"TAVILY_API_KEY": true,
		"OPENAI_API_KEY": true,
	}

	var env []EnvironmentVariable
	for key, val := range envVars {
		env = append(env, EnvironmentVariable{
			Key:         key,
			Value:       val,
			IsSensitive: sensitiveKeys[key],
		})
	}

	return CreateAgentRequest{
		Name:        name,
		DisplayName: name,
		Description: description,
		Provisioning: Provisioning{
			Type: "internal",
			Repository: &Repository{
				URL:     "https://github.com/wso2/agent-manager",
				Branch:  "main",
				AppPath: "/samples/customer-support-agent",
			},
		},
		AgentType: AgentType{
			Type:    "agent-api",
			SubType: "chat-api",
		},
		Build: &BuildConfig{
			Type: "buildpack",
			Buildpack: &BuildpackConfig{
				Language:        "python",
				LanguageVersion: "3.11",
				RunCommand:      "python main.py",
			},
		},
		Configurations: &Configurations{
			Env:                       env,
			EnableAutoInstrumentation: &autoInstr,
		},
		InputInterface: &InputInterface{
			Type: "HTTP",
		},
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

// DefaultInvokeRequest returns the standard chat invocation payload.
func DefaultInvokeRequest() json.RawMessage {
	return json.RawMessage(`{"session_id":"session-44","message":"Hello, what can you do?","context":{}}`)
}
