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
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/controllers"
	"github.com/wso2/agent-manager/agent-manager-service/middleware"
	"github.com/wso2/agent-manager/agent-manager-service/middleware/jwtassertion"
)

// RegisterBuildPublisherRoutes registers the route a source build's workload step
// calls with its organization-scoped build token, under the /build-publisher/ prefix.
func RegisterBuildPublisherRoutes(rr *middleware.RouteRegistrar, ctrl controllers.BuildPublisherController) {
	// POST /build-publisher/builds/{buildName}/workload - Publish a build's Workload
	handler := jwtassertion.BuildClientAuthMiddleware()(http.HandlerFunc(ctrl.PublishBuildWorkload))
	rr.HandleFuncWithValidation(route("POST", "/build-publisher/builds/{buildName}/workload"),
		handler.ServeHTTP)
}
