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

package gwapi

import (
	"net/http"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// RegisterBudgetRoutes registers the budget routes. can checks a permission the route
// itself does not require.
func RegisterBudgetRoutes(rr Router, controller gwctl.BudgetController, can func(r *http.Request, perm string) bool) {
	base := "/orgs/{orgName}/budgets"
	rr.HandleFuncWithValidationAndAuthz("GET "+base, gwperm.BudgetRead, controller.ListBudgets)
	rr.HandleFuncWithValidationAndAuthz("POST "+base, gwperm.BudgetCreate, controller.CreateBudget)
	rr.HandleFuncWithValidationAndAuthz("POST "+base+"/import", gwperm.BudgetCreate, importBudgets(controller, can))
	rr.HandleFuncWithValidationAndAuthz("GET "+base+"/{budgetName}", gwperm.BudgetRead, controller.GetBudget)
	rr.HandleFuncWithValidationAndAuthz("GET "+base+"/{budgetName}/budget-usage", gwperm.BudgetRead, controller.GetBudgetUsage)
	rr.HandleFuncWithValidationAndAuthz("PUT "+base+"/{budgetName}", gwperm.BudgetUpdate, controller.UpdateBudget)
	rr.HandleFuncWithValidationAndAuthz("DELETE "+base+"/{budgetName}", gwperm.BudgetDelete, controller.DeleteBudget)
}

// importBudgets is the import handler: an import that may replace existing budgets
// (onExisting=update) also needs budgets.update, dry run or not, so creating budgets
// never lets a caller change ones that exist.
func importBudgets(controller gwctl.BudgetController, can func(r *http.Request, perm string) bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("onExisting") == gwsvc.OnExistingUpdate && !can(r, short(gwperm.BudgetUpdate)) {
			httpx.WriteErrorResponseWithReason(w, http.StatusForbidden, "Forbidden",
				"Replacing existing budgets needs the "+gwperm.BudgetUpdate+" permission", httpx.ErrCodeForbidden)
			return
		}
		controller.ImportBudgets(w, r)
	}
}
