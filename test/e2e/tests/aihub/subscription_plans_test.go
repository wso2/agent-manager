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

// Validates the AI Hub module's subscription plans: the module is on in the full distribution, and a
// plan is created with conditions, refused when duplicated or priced, found by search, updated (omitted
// fields kept) and deleted.

package aihub

import (
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	aihubop "github.com/wso2/agent-manager/test/e2e/operations/aihub"
)

var _ = Describe("Subscription Plans", Label("aihub", "subscription-plans"), Ordered, func() {
	var planID string

	BeforeAll(func() {
		planID = "e2e-test-plan-" + uuid.New().String()[:8]
	})

	AfterAll(func() {
		if planID != "" {
			aihubop.DeletePlan(Default, Client, planID)
		}
	})

	It("should be offered by the AI Hub module", func() {
		resp, err := Client.Get("/api/v1/ui-manifest")
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		manifest := framework.ExpectStatusAndDecode[struct {
			Modules []struct {
				Name string `json:"name"`
			} `json:"modules"`
			Pages []string `json:"pages"`
		}](Default, resp, http.StatusOK)
		var names []string
		for _, m := range manifest.Modules {
			names = append(names, m.Name)
		}
		Expect(names).To(ContainElement("aihub"), "the full distribution enables the AI Hub module by default")
		Expect(manifest.Pages).To(ContainElement("aihub/subscription-plans"))
	})

	It("should create a plan whose conditions are normalized", func() {
		description := "E2E plan"
		plan := aihubop.CreatePlan(Default, Client, framework.SubscriptionPlanRequest{
			ID: planID, DisplayName: "E2E Gold", Description: &description,
			Conditions: []framework.PlanCondition{
				{Type: "request-count", Params: framework.PlanConditionParams{
					Limits: []framework.PlanLimit{{Count: 1000, TimeUnit: "HOUR"}}}},
				{Type: "tool-call-count", Params: framework.PlanConditionParams{
					Limits: []framework.PlanLimit{{Count: 30, TimeAmount: 1, TimeUnit: "MINUTE"}}, OnExceed: "allow"}},
			},
		})
		Expect(plan.ID).To(Equal(planID))
		Expect(plan.Status).To(Equal("ACTIVE"), "a new plan is active by default")
		Expect(plan.Conditions).To(HaveLen(2))
		Expect(plan.Conditions[0].ID).To(Equal("requests"))
		Expect(plan.Conditions[0].Params.OnExceed).To(Equal("reject"), "onExceed defaults to reject")
		Expect(plan.Conditions[0].Params.Limits[0].TimeAmount).To(Equal(1), "timeAmount defaults to 1")
		Expect(plan.Conditions[1].ID).To(Equal("tool-calls"))
		Expect(plan.Conditions[1].Params.OnExceed).To(Equal("allow"))
	})

	It("should refuse a duplicate plan", func() {
		aihubop.CreatePlanExpectError(Default, Client,
			framework.SubscriptionPlanRequest{ID: planID, DisplayName: "Duplicate"},
			http.StatusConflict, "SUBSCRIPTION_PLAN_EXISTS")
	})

	It("should refuse a priced plan", func() {
		aihubop.CreatePlanExpectError(Default, Client,
			framework.SubscriptionPlanRequest{ID: planID + "-paid", DisplayName: "Paid",
				Pricing: map[string]any{"flatAmount": 10, "currency": "USD", "billingPeriod": "MONTH"}},
			http.StatusBadRequest, "VALIDATION_FAILED")
	})

	It("should find the plan by search", func() {
		list := aihubop.ListPlans(Default, Client, planID)
		Expect(list.List).To(HaveLen(1))
		Expect(list.List[0].ID).To(Equal(planID))
	})

	It("should update the plan, keeping omitted fields", func() {
		plan := aihubop.UpdatePlan(Default, Client, planID, framework.SubscriptionPlanRequest{
			DisplayName: "E2E Gold v2", Status: "INACTIVE"})
		Expect(plan.DisplayName).To(Equal("E2E Gold v2"))
		Expect(plan.Status).To(Equal("INACTIVE"))
		Expect(plan.Description).To(Equal("E2E plan"), "an omitted description is kept")
		Expect(plan.Conditions).To(HaveLen(2), "omitted conditions are kept")
		Expect(aihubop.GetPlan(Default, Client, planID).DisplayName).To(Equal("E2E Gold v2"))
	})

	It("should delete the plan", func() {
		aihubop.DeletePlan(Default, Client, planID)
		aihubop.GetPlanExpectStatus(Default, Client, planID, http.StatusNotFound)
	})
})
