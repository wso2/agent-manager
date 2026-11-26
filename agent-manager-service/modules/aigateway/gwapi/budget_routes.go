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
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwperm"
)

// RegisterBudgetRoutes registers the budget routes: costs.read reads budgets, costs.write
// creates, updates and imports them, and costs.delete deletes them.
func RegisterBudgetRoutes(rr Router, controller gwctl.BudgetController) {
	base := "/orgs/{orgName}/budgets"
	rr.HandleFuncWithValidationAndAuthz("GET "+base, gwperm.CostRead, controller.ListBudgets)
	rr.HandleFuncWithValidationAndAuthz("POST "+base, gwperm.CostWrite, controller.CreateBudget)
	rr.HandleFuncWithValidationAndAuthz("POST "+base+"/import", gwperm.CostWrite, controller.ImportBudgets)
	rr.HandleFuncWithValidationAndAuthz("GET "+base+"/{budgetName}", gwperm.CostRead, controller.GetBudget)
	rr.HandleFuncWithValidationAndAuthz("GET "+base+"/{budgetName}/usage", gwperm.CostRead, controller.GetBudgetUsage)
	rr.HandleFuncWithValidationAndAuthz("PUT "+base+"/{budgetName}", gwperm.CostWrite, controller.UpdateBudget)
	rr.HandleFuncWithValidationAndAuthz("DELETE "+base+"/{budgetName}", gwperm.CostDelete, controller.DeleteBudget)
}
