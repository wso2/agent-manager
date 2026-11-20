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

// Package aihub provides E2E operation helpers for the AI Hub module's subscription plans API.
package aihub

import (
	"fmt"
	"net/http"
	"net/url"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// PlansPath is the plans API; the organization comes from the caller's token.
const PlansPath = "/m/ai-hub/subscription-plans"

// CreatePlan creates a subscription plan and expects 201.
func CreatePlan(g Gomega, client *framework.AMPClient, req framework.SubscriptionPlanRequest) framework.SubscriptionPlanResponse {
	resp, err := client.Post(PlansPath, req)
	g.Expect(err).NotTo(HaveOccurred(), "create subscription plan request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.SubscriptionPlanResponse](g, resp, http.StatusCreated)
}

// CreatePlanExpectError creates a plan that must be refused with the status and error code.
func CreatePlanExpectError(g Gomega, client *framework.AMPClient, req framework.SubscriptionPlanRequest, status int, code string) {
	resp, err := client.Post(PlansPath, req)
	g.Expect(err).NotTo(HaveOccurred(), "create subscription plan request failed")
	defer resp.Body.Close()
	body := framework.ExpectStatusAndDecode[framework.APIErrorResponse](g, resp, status)
	g.Expect(body.Code).To(Equal(code))
}

// GetPlan reads a subscription plan and expects 200.
func GetPlan(g Gomega, client *framework.AMPClient, id string) framework.SubscriptionPlanResponse {
	resp, err := client.Get(PlansPath + "/" + url.PathEscape(id))
	g.Expect(err).NotTo(HaveOccurred(), "get subscription plan request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.SubscriptionPlanResponse](g, resp, http.StatusOK)
}

// GetPlanExpectStatus reads a subscription plan and asserts only the status.
func GetPlanExpectStatus(g Gomega, client *framework.AMPClient, id string, status int) {
	resp, err := client.Get(PlansPath + "/" + url.PathEscape(id))
	g.Expect(err).NotTo(HaveOccurred(), "get subscription plan request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, status)
}

// ListPlans lists subscription plans matching query (name or description).
func ListPlans(g Gomega, client *framework.AMPClient, query string) framework.SubscriptionPlanListResponse {
	resp, err := client.Get(fmt.Sprintf("%s?limit=100&query=%s", PlansPath, url.QueryEscape(query)))
	g.Expect(err).NotTo(HaveOccurred(), "list subscription plans request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.SubscriptionPlanListResponse](g, resp, http.StatusOK)
}

// UpdatePlan updates a subscription plan and expects 200.
func UpdatePlan(g Gomega, client *framework.AMPClient, id string, req framework.SubscriptionPlanRequest) framework.SubscriptionPlanResponse {
	resp, err := client.Put(PlansPath+"/"+url.PathEscape(id), req)
	g.Expect(err).NotTo(HaveOccurred(), "update subscription plan request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.SubscriptionPlanResponse](g, resp, http.StatusOK)
}

// DeletePlan deletes a subscription plan, accepting 204 or (already gone) 404.
func DeletePlan(g Gomega, client *framework.AMPClient, id string) {
	resp, err := client.Delete(PlansPath + "/" + url.PathEscape(id))
	g.Expect(err).NotTo(HaveOccurred(), "delete subscription plan request failed")
	defer resp.Body.Close()
	framework.ExpectStatusIn(g, resp, http.StatusNoContent, http.StatusNotFound)
}
