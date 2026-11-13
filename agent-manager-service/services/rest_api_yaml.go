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

import "github.com/wso2/agent-manager/agent-manager-service/models"

// Operation is one method and path a RestApi serves.
type Operation struct {
	Method string `yaml:"method"`
	Path   string `yaml:"path"`
}

// RestAPIDeploymentYAML is the kind: RestApi resource agent-manager publishes to the gateway.
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
