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

// Package gwperm holds the gateway module's permissions. The module declares
// them in its manifest by their action names; the host names them
// gateway:<action>, which ThunderID computes as the amp:gateway:<action>
// scopes they have always had.
package gwperm

import "github.com/wso2/agent-manager/agent-manager-service/rbac"

// Module is the name of the module that owns these permissions.
const Module = "gateway"

// The gateway permissions.
const (
	Create      rbac.Permission = "gateway:create"
	Read        rbac.Permission = "gateway:read"
	Update      rbac.Permission = "gateway:update"
	Delete      rbac.Permission = "gateway:delete"
	TokenManage rbac.Permission = "gateway:token-manage"
)

// Actions are the permissions' names within the module, with what each lets
// a caller do.
var Actions = []struct{ Name, Description string }{
	{"create", "Register gateways"},
	{"read", "View gateways"},
	{"update", "Update gateways and their environments"},
	{"delete", "Delete gateways"},
	{"token-manage", "Manage gateway registration tokens"},
}

// RoleGrants is what each predefined role holds, by action name.
var RoleGrants = map[string][]string{
	rbac.RoleAdmin:            {"create", "read", "update", "delete", "token-manage"},
	rbac.RoleDeveloper:        {"read"},
	rbac.RolePlatformEngineer: {"create", "read", "update", "delete", "token-manage"},
}
