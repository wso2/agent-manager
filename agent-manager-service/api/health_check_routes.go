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

package api

import (
	"context"
	"net/http"
	"time"

	"github.com/wso2/ai-agent-management-platform/agent-manager-service/config"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/db"
	"github.com/wso2/ai-agent-management-platform/agent-manager-service/utils"
)

func registerHealthCheck(mux *http.ServeMux) {
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), time.Duration(config.GetConfig().HealthCheckTimeoutSeconds)*time.Second)
		defer cancel()

		var dbRes *int
		if result := db.DB(ctx).Raw("SELECT 1").Scan(&dbRes); result.Error != nil {
			utils.WriteErrorResponse(w, http.StatusInternalServerError, "database connection error")
			return
		}
		response := map[string]interface{}{
			"message":   "agent-manager-service is healthy",
			"timestamp": time.Now(),
		}
		utils.WriteSuccessResponse(w, http.StatusOK, response)
	})
}
