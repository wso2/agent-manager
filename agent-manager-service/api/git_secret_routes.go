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

package api

import (
	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/growthanalytics"
	"github.com/wso2/agent-manager/agent-manager-service/rbac"
)

// RegisterGitSecretRoutes registers all git secret routes
func RegisterGitSecretRoutes(rr *middleware.RouteRegistrar, ctrl controllers.GitSecretController) {
	// Git Secrets (org-level)
	rr.HandleFuncWithValidationAndAuthz("POST /orgs/{orgName}/git-secrets", rbac.GitSecretCreate,
		growthanalytics.Track("amp.source-control.manage-git-secret", actionDims("created-git-secret"), ctrl.CreateGitSecret))
	rr.HandleFuncWithValidationAndAuthz("GET /orgs/{orgName}/git-secrets", rbac.GitSecretRead, ctrl.ListGitSecrets)
	rr.HandleFuncWithValidationAndAuthz("DELETE /orgs/{orgName}/git-secrets/{secretName}", rbac.GitSecretDelete,
		growthanalytics.Track("amp.source-control.manage-git-secret", actionDims("deleted-git-secret"), ctrl.DeleteGitSecret))
}
