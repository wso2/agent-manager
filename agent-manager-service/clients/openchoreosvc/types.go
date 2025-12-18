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

package openchoreosvc

import "time"

type ComponentType string

const (
	ComponentTypeInternalAgentAPI ComponentType = "deployment/agent-api"
	ComponentTypeExternalAgentAPI ComponentType = "proxy/external-agent-api"
)

type TraitType string

const (
	TraitTypeOTELInstrumentation TraitType = "python-otel-instrumentation-trait"
)

type ComponentWorkflow string

const (
	ComponentWorkflowGCB       ComponentWorkflow = "google-cloud-buildpacks"
	ComponentWorkflowBallerina ComponentWorkflow = "ballerina-buildpack"
)

type AgentComponent struct {
	UUID         string       `json:"uuid"`
	Name         string       `json:"name"`
	DisplayName  string       `json:"displayName,omitempty"`
	Description  string       `json:"description,omitempty"`
	ProjectName  string       `json:"projectName"`
	CreatedAt    time.Time    `json:"createdAt"`
	Status       string       `json:"status,omitempty"`
	Provisioning Provisioning `json:"provisioning"`
	Type         AgentType    `json:"agentType,omitempty"`
	Language     string       `json:"language,omitempty"`
}

type AgentType struct {
	Type    string `json:"type"`
	SubType string `json:"subType,omitempty"`
}
type Provisioning struct {
	Type       string     `json:"type"`
	Repository Repository `json:"repository,omitempty"`
}
type Repository struct {
	RepoURL string `json:"repoURL"`
	Branch  string `json:"branch,omitempty"`
	AppPath string `json:"appPath,omitempty"`
}
