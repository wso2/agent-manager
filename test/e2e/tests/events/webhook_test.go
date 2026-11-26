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

// J10: an org admin sends platform events to a webhook. Deliveries are signed
// with the destination's secret, retried when the receiver fails, follow a
// rotated secret, and stop when the subscription is deleted. Project and agent
// owners subscribe to their own events through destinations made available
// to them.

package events

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
)

var _ = Describe("Event delivery to a webhook", Label("events"), Ordered, func() {
	var (
		s        *sink
		suffix   string
		destID   string
		secret   string
		subID    string
		projects []string
		agent    string
	)
	ev := func(format string, args ...any) string { return framework.API(Cfg, "/events"+format, args...) }
	shared := framework.E2ESharedProjectName

	createProject := func(name string) {
		projects = append(projects, name)
		framework.Call(Default, Client, http.MethodPost, framework.Projects(""),
			framework.NewProjectDocument(name, "Events "+name, ""), http.StatusAccepted)
	}

	BeforeAll(func() {
		suffix = uuid.NewString()[:8]
		agent = framework.E2EAgentPrefix + suffix
		s = startSink()
		DeferCleanup(func() {
			s.stop()
			if subID != "" {
				framework.Send(Default, Client, http.MethodDelete, ev("/subscriptions/%s", subID), nil)
			}
			agentops.DeleteAgentBestEffort(Client, Cfg.DefaultOrg, shared, agent)
			for _, p := range projects {
				framework.Send(Default, Client, http.MethodDelete, framework.Projects("/%s", p), nil)
			}
			if destID != "" {
				framework.Send(Default, Client, http.MethodDelete, ev("/destinations/%s", destID), nil)
			}
		})
	})

	It("lists the event types and destination kinds a destination can be built from", func() {
		types := framework.CallObj(Default, Client, http.MethodGet, ev("/event-types"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(types, "eventTypes"), "type")).To(ContainElements("project.created", "agent.updated"))
		kinds := framework.CallObj(Default, Client, http.MethodGet, ev("/destination-kinds"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(kinds, "kinds"), "kind")).To(ContainElements("webhook", "notify"))
	})

	It("registers a webhook destination and returns its signing secret once", func() {
		status, body := framework.Send(Default, Client, http.MethodPost, ev("/destinations"), framework.Obj{
			"name": "e2e-test-hook-" + suffix, "kind": "webhook", "availability": "projects",
			"projects": []string{shared}, "config": framework.Obj{"url": s.URL},
		})
		if status == http.StatusBadRequest {
			Skip("the server refuses a private webhook URL; start it with EVENTS_ALLOW_PRIVATE_DESTINATIONS=true: " + string(body))
		}
		Expect(status).To(Equal(http.StatusCreated), string(body))
		var d framework.Obj
		Expect(json.Unmarshal(body, &d)).To(Succeed())
		destID, secret = framework.Str(d, "id"), framework.Str(d, "secret")
		Expect(secret).To(HavePrefix("whsec_"))

		got := framework.CallObj(Default, Client, http.MethodGet, ev("/destinations/%s", destID), nil, http.StatusOK)
		Expect(got).NotTo(HaveKey("secret"), "the secret is returned only on create")
		Expect(framework.Str(got, "config", "url")).To(Equal(s.URL))
		list := framework.CallObj(Default, Client, http.MethodGet, ev("/destinations"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(list, "destinations"), "id")).To(ContainElement(destID))

		framework.CallError(Default, Client, http.MethodPost, ev("/destinations"), framework.Obj{
			"name": "e2e-test-bad-" + suffix, "kind": "webhook", "availability": "all", "config": framework.Obj{},
		}, http.StatusBadRequest)
	})

	It("sends a signed test event the receiver can verify", func() {
		res := framework.CallObj(Default, Client, http.MethodPost, ev("/destinations/%s/test", destID), nil, http.StatusOK)
		if res["delivered"] != true {
			Skip("amp-api can't reach the sink at " + s.URL + " (set E2E_SINK_HOST): " + framework.Str(res, "error"))
		}
		Expect(s.matching("")).NotTo(BeEmpty())
		Expect(verifySignature(secret, s.matching("")[0])).To(BeTrue(), "the test event's signature doesn't verify")
	})

	It("delivers a subscribed event, signed, and retries when the receiver fails", func() {
		sub := framework.CallObj(Default, Client, http.MethodPost, ev("/subscriptions"), framework.Obj{
			"name": "e2e-test-sub-" + suffix, "destinationId": destID, "eventTypes": []string{"project.created"},
		}, http.StatusCreated)
		subID = framework.Str(sub, "id")
		Expect(sub["enabled"]).To(BeTrue())

		proj := framework.E2EProjectPrefix + "ev-" + suffix
		s.failNextN(proj, 1)
		createProject(proj)

		Eventually(func() []delivery { return s.matching(proj) }).
			WithTimeout(90*time.Second).WithPolling(time.Second).Should(HaveLen(2), "one failed attempt, then a retry")
		got := s.matching(proj)
		Expect(got[1].ID).To(Equal(got[0].ID), "a retry carries the same webhook-id")
		Expect(verifySignature(secret, got[1])).To(BeTrue())
		var ce framework.Obj
		Expect(json.Unmarshal(got[1].Body, &ce)).To(Succeed())
		Expect(ce["type"]).To(Equal("com.wso2.agentmanager.project.created"))

		Eventually(func(g Gomega) {
			list := framework.CallObj(g, Client, http.MethodGet, ev("/deliveries?subscriptionId=%s", subID), nil, http.StatusOK)
			ds := framework.Items(list, "deliveries")
			g.Expect(ds).NotTo(BeEmpty())
			g.Expect(ds[0]["status"]).To(Equal("delivered"))
			g.Expect(ds[0]["attempts"]).To(BeNumerically("==", 2))
		}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
	})

	It("signs with the new secret after a rotation", func() {
		rotated := framework.CallObj(Default, Client, http.MethodPost, ev("/destinations/%s/rotate-secret", destID), nil, http.StatusOK)
		newSecret := framework.Str(rotated, "secret")
		Expect(newSecret).To(HavePrefix("whsec_"))
		Expect(newSecret).NotTo(Equal(secret))

		proj := framework.E2EProjectPrefix + "ev2-" + suffix
		createProject(proj)
		Eventually(func() []delivery { return s.matching(proj) }).WithTimeout(60 * time.Second).WithPolling(time.Second).ShouldNot(BeEmpty())
		d := s.matching(proj)[0]
		Expect(verifySignature(newSecret, d)).To(BeTrue())
		Expect(verifySignature(secret, d)).To(BeFalse(), "the old secret must no longer verify")
		secret = newSecret
	})

	It("updates a subscription and a destination", func() {
		framework.CallObj(Default, Client, http.MethodPut, ev("/subscriptions/%s", subID), framework.Obj{
			"name": "e2e-test-sub-" + suffix, "destinationId": destID,
			"eventTypes": []string{"project.created", "project.deleted"},
		}, http.StatusOK)
		sub := framework.CallObj(Default, Client, http.MethodGet, ev("/subscriptions/%s", subID), nil, http.StatusOK)
		Expect(sub["eventTypes"]).To(ConsistOf("project.created", "project.deleted"))
		subs := framework.CallObj(Default, Client, http.MethodGet, ev("/subscriptions"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(subs, "subscriptions"), "id")).To(ContainElement(subID))

		framework.CallObj(Default, Client, http.MethodPut, ev("/destinations/%s", destID), framework.Obj{
			"name": "e2e-test-hook-" + suffix, "description": "renamed", "kind": "webhook", "availability": "projects",
			"projects": []string{shared}, "config": framework.Obj{"url": s.URL},
		}, http.StatusOK)
		got := framework.CallObj(Default, Client, http.MethodGet, ev("/destinations/%s", destID), nil, http.StatusOK)
		Expect(got["description"]).To(Equal("renamed"))
	})

	It("lets the project and its agent subscribe through the destination made available to them", func() {
		agentops.CreateAgent(Default, Client, &agentops.CreateAgentParams{
			OrgName: Cfg.DefaultOrg, ProjectName: shared,
			Request: framework.NewExternalAgentRequest(agent, "events journey"),
		})
		proj := func(f string, a ...any) string {
			return framework.Projects("/%s/events"+f, append([]any{shared}, a...)...)
		}
		ag := func(f string, a ...any) string {
			return framework.Projects("/%s/agents/%s/events"+f, append([]any{shared, agent}, a...)...)
		}

		dests := framework.CallObj(Default, Client, http.MethodGet, proj("/destinations"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(dests, "destinations"), "id")).To(ContainElement(destID))
		agDests := framework.CallObj(Default, Client, http.MethodGet, ag("/destinations"), nil, http.StatusOK)
		Expect(framework.Field(framework.Items(agDests, "destinations"), "id")).To(ContainElement(destID))

		ps := framework.CallObj(Default, Client, http.MethodPost, proj("/subscriptions"), framework.Obj{
			"name": "e2e-test-psub-" + suffix, "destinationId": destID, "eventTypes": []string{"agent.updated"},
		}, http.StatusCreated)
		psID := framework.Str(ps, "id")
		Expect(ps["scope"]).To(Equal("project"))
		as := framework.CallObj(Default, Client, http.MethodPost, ag("/subscriptions"), framework.Obj{
			"name": "e2e-test-asub-" + suffix, "destinationId": destID, "eventTypes": []string{"agent.updated"},
		}, http.StatusCreated)
		asID := framework.Str(as, "id")
		Expect(as["agentName"]).To(Equal(agent))

		framework.CallObj(Default, Client, http.MethodPut, proj("/subscriptions/%s", psID), framework.Obj{
			"name": "e2e-test-psub-" + suffix, "destinationId": destID, "eventTypes": []string{"agent.updated"}, "enabled": false,
		}, http.StatusOK)
		Expect(framework.CallObj(Default, Client, http.MethodGet, proj("/subscriptions/%s", psID), nil, http.StatusOK)["enabled"]).To(BeFalse())
		framework.CallObj(Default, Client, http.MethodPut, ag("/subscriptions/%s", asID), framework.Obj{
			"name": "e2e-test-asub-" + suffix + "-b", "destinationId": destID, "eventTypes": []string{"agent.updated"},
		}, http.StatusOK)
		Expect(framework.CallObj(Default, Client, http.MethodGet, ag("/subscriptions/%s", asID), nil, http.StatusOK)["name"]).
			To(Equal("e2e-test-asub-" + suffix + "-b"))
		Expect(framework.Field(framework.Items(framework.CallObj(Default, Client, http.MethodGet, proj("/subscriptions"), nil, http.StatusOK), "subscriptions"), "id")).
			To(ContainElement(psID))
		Expect(framework.Field(framework.Items(framework.CallObj(Default, Client, http.MethodGet, ag("/subscriptions"), nil, http.StatusOK), "subscriptions"), "id")).
			To(ContainElement(asID))

		// The agent's subscription delivers its own update.
		renamed := agentops.GetAgent(Default, Client, Cfg.DefaultOrg, shared, agent)
		renamed.Status = nil
		renamed.Spec.DisplayName, renamed.Spec.Description = "Renamed "+suffix, "events"
		framework.Call(Default, Client, http.MethodPut, agentops.AgentPath(shared, agent), renamed, http.StatusOK)
		Eventually(func() []delivery { return s.matching(agent) }).WithTimeout(60 * time.Second).WithPolling(time.Second).ShouldNot(BeEmpty())
		Eventually(func(g Gomega) {
			g.Expect(framework.Items(framework.CallObj(g, Client, http.MethodGet, ag("/deliveries"), nil, http.StatusOK), "deliveries")).NotTo(BeEmpty())
		}).WithTimeout(30 * time.Second).WithPolling(time.Second).Should(Succeed())
		framework.Call(Default, Client, http.MethodGet, proj("/deliveries"), nil, http.StatusOK)

		framework.Call(Default, Client, http.MethodDelete, ag("/subscriptions/%s", asID), nil, http.StatusNoContent)
		framework.Call(Default, Client, http.MethodDelete, proj("/subscriptions/%s", psID), nil, http.StatusNoContent)
		framework.CallError(Default, Client, http.MethodGet, ag("/subscriptions/%s", asID), nil, http.StatusNotFound)
	})

	It("refuses to delete a destination a subscription still uses", func() {
		framework.CallError(Default, Client, http.MethodDelete, ev("/destinations/%s", destID), nil, http.StatusConflict)
		framework.Call(Default, Client, http.MethodGet, ev("/destinations/%s", destID), nil, http.StatusOK)
	})

	It("stops delivering once the subscription is deleted", func() {
		framework.Call(Default, Client, http.MethodDelete, ev("/subscriptions/%s", subID), nil, http.StatusNoContent)
		framework.CallError(Default, Client, http.MethodGet, ev("/subscriptions/%s", subID), nil, http.StatusNotFound)
		subID = ""

		proj := framework.E2EProjectPrefix + "ev3-" + suffix
		createProject(proj)
		Consistently(func() []delivery { return s.matching(proj) }).WithTimeout(15 * time.Second).WithPolling(time.Second).Should(BeEmpty())

		framework.Call(Default, Client, http.MethodDelete, ev("/destinations/%s", destID), nil, http.StatusNoContent)
		framework.CallError(Default, Client, http.MethodGet, ev("/destinations/%s", destID), nil, http.StatusNotFound)
		destID = ""
	})
})
