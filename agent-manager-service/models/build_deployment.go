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

import "time"

// DeploymentResponse represents deployment information
type DeploymentResponse struct {
	AgentName                  string                      `json:"agentName"`
	ProjectName                string                      `json:"projectName"`
	ImageId                    string                      `json:"imageId"`
	Status                     string                      `json:"status"`
	Environment                string                      `json:"environment"`
	EnvironmentDisplayName     string                      `json:"environmentDisplayName"`
	PromotionTargetEnvironment *PromotionTargetEnvironment `json:"promotionTargetEnvironment,omitempty"`
	LastDeployedAt             time.Time                   `json:"lastDeployedAt"`
	Endpoints                  []Endpoint                  `json:"endpoints"`
}

// PromotionTargetEnvironment represents environment promotion targets
type PromotionTargetEnvironment struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
}

// EndpointsResponse represents detailed endpoint information
type EndpointsResponse struct {
	Endpoint
	Schema EndpointSchema `json:"schema"`
}

// EndpointSchema represents the schema for an endpoint
type EndpointSchema struct {
	Content string `json:"content"`
}

// Endpoint represents endpoint configuration
type Endpoint struct {
	URL        string `json:"url"`
}

// EnvVars represents environment variables
type EnvVars struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Build represents a build instance
type BuildResponse struct {
	UUID        string     `json:"uuid"`
	Name        string     `json:"name"`
	AgentName   string     `json:"agentName"`
	ProjectName string     `json:"projectName"`
	CommitID    string     `json:"commitId"`
	Status      string     `json:"status"`
	StartedAt   time.Time  `json:"startedAt"`
	Image       string     `json:"image,omitempty"`
	Branch      string     `json:"branch,omitempty"`
	EndedAt     *time.Time `json:"endedAt,omitempty"`
}

// BuildStep represents a step in the build process
type BuildStep struct {
	Type    string    `json:"type"`
	Status  string    `json:"status"`
	Message string    `json:"message"`
	At      time.Time `json:"at"`
}

// BuildDetails represents detailed build information
type BuildDetailsResponse struct {
	BuildResponse
	Percent         float32     `json:"percent,omitempty"`
	Steps           []BuildStep `json:"steps,omitempty"`
	DurationSeconds int32       `json:"durationSeconds,omitempty"`
}
