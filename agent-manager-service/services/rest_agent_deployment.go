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

package services

import (
	"fmt"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/wso2/agent-manager/agent-manager-service/clients/openchoreosvc/client"
	"github.com/wso2/agent-manager/agent-manager-service/models"
	"github.com/wso2/agent-manager/agent-manager-service/utils"
)

// restAgentVersion is the version the api-configuration trait gives a REST agent.
const restAgentVersion = "v1.0"

// Operation is one method and path a RestApi serves.
type Operation struct {
	Method string `yaml:"method"`
	Path   string `yaml:"path"`
}

// restAgentOperations are every method on every path: the agent itself decides what it serves.
var restAgentOperations = []Operation{
	{Method: "GET", Path: "/*"},
	{Method: "POST", Path: "/*"},
	{Method: "PUT", Path: "/*"},
	{Method: "DELETE", Path: "/*"},
	{Method: "PATCH", Path: "/*"},
	{Method: "OPTIONS", Path: "/*"},
}

// RestAPIDeploymentYAML is the kind: RestApi resource agent-manager publishes to the gateway for a REST agent.
type RestAPIDeploymentYAML struct {
	ApiVersion string                `yaml:"apiVersion" json:"apiVersion"`
	Kind       string                `yaml:"kind" json:"kind"`
	Metadata   DeploymentMetadata    `yaml:"metadata" json:"metadata"`
	Spec       RestAPIDeploymentSpec `yaml:"spec" json:"spec"`
}

// RestAPIDeploymentSpec is the spec section of the RestApi resource.
type RestAPIDeploymentSpec struct {
	DisplayName string                   `yaml:"displayName" json:"displayName"`
	Version     string                   `yaml:"version" json:"version"`
	Context     string                   `yaml:"context" json:"context"`
	Upstream    RestAPIUpstream          `yaml:"upstream" json:"upstream"`
	Policies    []map[string]interface{} `yaml:"policies" json:"policies"`
	Operations  []Operation              `yaml:"operations" json:"operations"`
	Resilience  *models.Resilience       `yaml:"resilience,omitempty" json:"resilience,omitempty"`
}

// RestAPIUpstream is the address the gateway dials.
type RestAPIUpstream struct {
	Main RestAPIUpstreamTarget `yaml:"main" json:"main"`
}

// RestAPIUpstreamTarget is one upstream URL.
type RestAPIUpstreamTarget struct {
	URL string `yaml:"url" json:"url"`
}

// RestAPIDeploymentInput is everything the builder needs, already resolved.
type RestAPIDeploymentInput struct {
	ProjectName     string
	AgentName       string
	EnvironmentUUID string
	// UpstreamURL is the release binding's service URL. Empty is an error.
	UpstreamURL string
	// Policies is the output of buildPolicies.
	Policies                 []map[string]interface{}
	ResilienceTimeoutSeconds int32
}

// buildRestAPIDeploymentYAML assembles the RestApi resource. An empty upstream is refused rather than emitted: a
// RestApi published before the release binding serves would route nowhere while looking healthy.
func buildRestAPIDeploymentYAML(in RestAPIDeploymentInput) (*RestAPIDeploymentYAML, error) {
	upstreamURL := strings.TrimSpace(in.UpstreamURL)
	if upstreamURL == "" {
		return nil, fmt.Errorf("refusing to publish agent %q: upstream url is not available yet", in.AgentName)
	}
	policies := in.Policies
	if policies == nil {
		policies = []map[string]interface{}{}
	}
	return &RestAPIDeploymentYAML{
		ApiVersion: apiVersionA2AAgent,
		Kind:       models.KindRestAPI,
		Metadata:   DeploymentMetadata{Name: agentGatewayName(in.ProjectName, in.AgentName, in.EnvironmentUUID)},
		Spec: RestAPIDeploymentSpec{
			DisplayName: in.AgentName,
			Version:     restAgentVersion,
			Context:     agentGatewayContext(in.AgentName),
			Upstream:    RestAPIUpstream{Main: RestAPIUpstreamTarget{URL: upstreamURL}},
			Policies:    policies,
			Operations:  restAgentOperations,
			Resilience: &models.Resilience{
				Timeout:     utils.StrAsStrPointer(client.FormatResilienceTimeout(in.ResilienceTimeoutSeconds)),
				IdleTimeout: utils.StrAsStrPointer("0s"),
			},
		},
	}, nil
}

// generateRestAPIDeploymentYAML renders the RestApi resource as YAML.
func generateRestAPIDeploymentYAML(in RestAPIDeploymentInput) ([]byte, error) {
	deployment, err := buildRestAPIDeploymentYAML(in)
	if err != nil {
		return nil, err
	}
	content, err := yaml.Marshal(deployment)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal REST agent deployment YAML: %w", err)
	}
	return content, nil
}
