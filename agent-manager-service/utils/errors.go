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
	ErrProjectNotFound           = errors.New("project not found")
	ErrAgentAlreadyExists        = errors.New("agent already exists")
	ErrAgentNotFound             = errors.New("agent not found")
	ErrOrganizationNotFound      = errors.New("organization not found")
	ErrBuildNotFound             = errors.New("build not found")
	ErrEnvironmentNotFound       = errors.New("environment not found")
	ErrOrganizationAlreadyExists = errors.New("organization already exists")
	ErrProjectAlreadyExists      = errors.New("project already exists")
)
