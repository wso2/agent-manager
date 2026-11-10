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

// NewITHelpdeskAgentRequest returns a request for creating the IT helpdesk agent
// from /samples/it-helpdesk-agent. The agent is expected to run with USE_LLM_PROVIDER=true,
// with LLM_PROVIDER_URL and LLM_PROVIDER_KEY injected via a model config rather than
// passed directly as env vars.
func NewITHelpdeskAgentRequest(name, description string, envVars map[string]string) CreateAgentRequest {
	autoInstr := true
	sensitiveKeys := map[string]bool{
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
				AppPath: "/samples/it-helpdesk-agent",
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

// NewA2AAgentRequest returns a request for creating the internal A2A sample
// agent (samples/a2a-agent), which serves its card on port 9099.
func NewA2AAgentRequest(name, description string, envVars map[string]string) CreateAgentRequest {
	req := NewITHelpdeskAgentRequest(name, description, envVars)
	req.Provisioning.Repository.AppPath = "/samples/a2a-agent"
	req.AgentType.SubType = "a2a-agent"
	req.InputInterface = &InputInterface{Type: "HTTP", Port: 9099}
	return req
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

// NewSecurityProbeAgentRequest returns the deterministic, non-LLM runtime
// fixture used by security/runtime. Its OpenAPI surface accepts only named
// probes; it is intentionally not a general-purpose command or URL executor.
func NewSecurityProbeAgentRequest(cfg *Config, name string) CreateAgentRequest {
	autoInstr := false
	return CreateAgentRequest{
		Name:        name,
		DisplayName: name,
		Description: "Disposable deterministic agent for sandbox and AgentID security tests",
		Provisioning: Provisioning{
			Type: "internal",
			Repository: &Repository{
				URL:     cfg.SecurityProbeRepository,
				Branch:  cfg.SecurityProbeBranch,
				AppPath: cfg.SecurityProbeAppPath,
			},
		},
		AgentType: AgentType{Type: "agent-api", SubType: "custom-api"},
		Build: &BuildConfig{
			Type: "buildpack",
			Buildpack: &BuildpackConfig{
				Language:        "python",
				LanguageVersion: "3.11",
				RunCommand:      "python main.py",
			},
		},
		Configurations: &Configurations{EnableAutoInstrumentation: &autoInstr},
		InputInterface: &InputInterface{
			Type:     "HTTP",
			Port:     8000,
			BasePath: "/",
			Schema:   &InputInterfaceSchema{Path: "/openapi.yaml"},
		},
	}
}

// NewAgentFromKindRequest returns a request to create an agent from a published
// catalog kind. The agent reuses the kind's pre-built image, so no build step
// is required. Runtime configuration (e.g. OPENAI_API_KEY) must be supplied via
// envVars; sensitive keys are automatically marked as such.
func NewAgentFromKindRequest(name, kindName, kindVersion string, envVars map[string]string) CreateAgentRequest {
	autoInstr := true
	sensitiveKeys := map[string]bool{
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
		Provisioning: Provisioning{
			Type: "internal",
			AgentKind: &AgentKindRef{
				Name:    kindName,
				Version: kindVersion,
			},
		},
		Configurations: &Configurations{
			Env:                       env,
			EnableAutoInstrumentation: &autoInstr,
		},
	}
}

// DefaultInvokeRequest returns the standard chat invocation payload.
func DefaultInvokeRequest() json.RawMessage {
	return json.RawMessage(`{"session_id":"session-44","message":"Hello, what can you do?","context":{}}`)
}
