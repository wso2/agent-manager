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

package gwctl

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// CostController serves the costs API (/orgs/{orgName}/costs).
type CostController interface {
	GetCosts(w http.ResponseWriter, r *http.Request)
}

type costController struct {
	costs gwsvc.CostService
	core  modkit.CoreAPI
}

// NewCostController creates a CostController; core gives the organization's handle.
func NewCostController(costs gwsvc.CostService, core modkit.CoreAPI) CostController {
	return &costController{costs: costs, core: core}
}

// GetCosts handles GET /orgs/{orgName}/costs: the kept months' cost per groupBy value.
func (c *costController) GetCosts(w http.ResponseWriter, r *http.Request) {
	query := r.URL.Query()
	ouID := modkit.OrgID(r.Context())
	org, err := c.core.GetOrg(r.Context(), ouID)
	if err != nil {
		writeBudgetError(w, r, err, "Failed to read costs")
		return
	}
	resp, err := c.costs.Costs(r.Context(), ouID, org.Handle, query.Get("groupBy"), query["filter"])
	if err != nil {
		writeBudgetError(w, r, err, "Failed to read costs")
		return
	}
	httpx.WriteSuccessResponse(w, http.StatusOK, resp)
}
