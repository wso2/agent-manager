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

import "errors"

var (
	// Resource not found errors
	ErrProjectNotFound            = errors.New("project not found")
	ErrAgentAlreadyExists         = errors.New("agent already exists")
	ErrAgentNotFound              = errors.New("agent not found")
	ErrOrganizationNotFound       = errors.New("organization not found")
	ErrBuildNotFound              = errors.New("build not found")
	ErrEnvironmentNotFound        = errors.New("environment not found")
	ErrOrganizationAlreadyExists  = errors.New("organization already exists")
	ErrProjectAlreadyExists       = errors.New("project already exists")
	ErrDeploymentPipelineNotFound = errors.New("deployment pipeline not found")
	ErrProjectHasAssociatedAgents = errors.New("project has associated agents")
	ErrInvalidInput               = errors.New("invalid input")
	ErrImmutableFieldChange       = errors.New("cannot change immutable field")

	// Request errors
	ErrBadRequest = errors.New("bad request")

	// Authorization errors
	ErrUnauthorized = errors.New("unauthorized")
	ErrForbidden    = errors.New("forbidden")

	// Server errors
	ErrServiceUnavailable = errors.New("service unavailable")
	// Gateway-related errors
	ErrGatewayNotFound         = errors.New("gateway not found")
	ErrGatewayAlreadyExists    = errors.New("gateway already exists")
	ErrInvalidAdapterType      = errors.New("invalid adapter type")
	ErrGatewayUnreachable      = errors.New("gateway unreachable")
	ErrInvalidGatewayConfig    = errors.New("invalid gateway configuration")
	ErrEnvironmentAlreadyExists = errors.New("environment already exists")
	ErrEnvironmentHasGateways   = errors.New("environment has associated gateways")
)
