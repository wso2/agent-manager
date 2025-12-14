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

package utils

type EndpointType string

const (
	EndpointTypeDefault EndpointType = "DEFAULT"
	EndpointTypeCustom  EndpointType = "CUSTOM"
)

type ResourceType string

const (
	ResourceTypeAgent   ResourceType = "agent"
	ResourceTypeProject ResourceType = "project"
)

// Name generation constants
const (
	MaxResourceNameLength     = 25
	RandomSuffixLength        = 2
	ValidCandidateLength      = MaxResourceNameLength - RandomSuffixLength - 1 // 1 for hyphen
	MaxNameGenerationAttempts = 10                                             // Prevent infinite loop
	NameGenerationAlphabet    = "abcdefghijklmnopqrstuvwxyz"
)

// Path parameter names used in HTTP routes
const (
	PathParamOrgName   = "orgName"
	PathParamProjName  = "projName"
	PathParamAgentName = "agentName"
	PathParamBuildName = "buildName"
)

// Pagination constants
const (
	DefaultLimit  = 10
	MinLimit      = 1
	MaxLimit      = 50
	DefaultOffset = 0
	MinOffset     = 0
)

