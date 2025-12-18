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

type EnvironmentResponse struct {
	UUID         string    `json:"uuid"`
	Name         string    `json:"name"`
	DataplaneRef string    `json:"dataplaneRef"`
	DisplayName  string    `json:"displayName,omitempty"`
	IsProduction bool      `json:"isProduction"`
	DNSPrefix    string    `json:"dnsPrefix,omitempty"`
	CreatedAt    time.Time `json:"createdAt"`
}

type DataPlaneResponse struct {
	Name        string    `json:"name"`
	OrgName     string    `json:"orgName"`
	DisplayName string    `json:"displayName,omitempty"`
	Description string    `json:"description,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
}

type DeploymentPipelineResponse struct {
	Name           string          `json:"name"`
	DisplayName    string          `json:"displayName,omitempty"`
	Description    string          `json:"description,omitempty"`
	OrgName        string          `json:"orgName"`
	CreatedAt      time.Time       `json:"createdAt"`
	PromotionPaths []PromotionPath `json:"promotionPaths,omitempty"`
}

type PromotionPath struct {
	SourceEnvironmentRef  string                 `json:"sourceEnvironmentRef"`
	TargetEnvironmentRefs []TargetEnvironmentRef `json:"targetEnvironmentRefs"`
}
type TargetEnvironmentRef struct {
	Name             string `json:"name"`
	RequiresApproval bool   `json:"requiresApproval,omitempty"`
}

type LogEntry struct {
	Timestamp     time.Time         `json:"timestamp"`
	Log           string            `json:"log"`
	LogLevel      string            `json:"logLevel"` // ERROR, WARN, INFO, DEBUG
	ComponentId   string            `json:"componentId"`
	EnvironmentId string            `json:"environmentId"`
	ProjectId     string            `json:"projectId"`
	Version       string            `json:"version"`
	VersionId     string            `json:"versionId"`
	Namespace     string            `json:"namespace"`
	PodId         string            `json:"podId"`
	ContainerName string            `json:"containerName"`
	Labels        map[string]string `json:"labels"`
}

type BuildLogsResponse struct {
	Logs       []LogEntry `json:"logs"`
	TotalCount int32      `json:"totalCount"`
	TookMs     float32    `json:"tookMs"`
}
