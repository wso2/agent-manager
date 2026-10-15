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

package observability

import (
	"fmt"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// SEC-OBS-001 — the observer's data routes each require their own scope.
//
// Unlike the agent-manager-service matrix, this one is a full cross-product:
// every route is driven with all five observability scopes. The diagonal must
// pass and every off-diagonal cell must 403. That matters because these five
// scopes are handed out unevenly by the predefined roles — AI Lead gets
// trace-read, monitor-log-read and metric-read but NOT log-read or
// build-log-read, so trace-read leaking access to logs would be a real
// privilege gain.
//
// Requests are side-effect free: these are all read-only endpoints. Most are
// driven with an absent id and no time range, so the handler answers with a 4xx
// rather than data; monitor-run-logs has no such guard, so a permitted call
// there runs a real (still read-only) 30-day upstream query.

// absentTraceID is a syntactically plausible trace id that will not exist.
const absentTraceID = "00000000000000000000000000000000"

type obsRoute struct {
	Name  string
	Path  string
	Query map[string]string
	Scope string
}

func obsRoutes(org string) []obsRoute {
	orgQuery := map[string]string{"organization": org}

	return []obsRoute{
		{"trace overviews", "/api/v1/traces", orgQuery, scopeTraceRead},
		{"trace export", "/api/v1/traces/export", orgQuery, scopeTraceRead},
		{
			"trace spans",
			"/api/v1/traces/" + absentTraceID + "/spans",
			orgQuery,
			scopeTraceRead,
		},
		{"runtime logs", "/api/v1/logs", orgQuery, scopeLogRead},
		{
			"build logs",
			"/api/v1/build-logs",
			map[string]string{"organization": org, "buildId": "e2e-test-sec-absent"},
			scopeBuildLogRead,
		},
		{
			"monitor run logs",
			"/api/v1/monitor-run-logs",
			map[string]string{"organization": org, "runName": "e2e-test-sec-absent"},
			scopeMonitorLogRead,
		},
		{"metrics", "/api/v1/metrics", orgQuery, scopeMetricRead},
	}
}

var _ = Describe("SEC-OBS-001: observability scope matrix", Label("security"), func() {
	for _, route := range obsRoutes(framework.LoadConfig().DefaultOrg) {
		route := route

		It(fmt.Sprintf("allows %s with %s", route.Name, route.Scope), func(ctx SpecContext) {
			resp := getObs(ctx, tokenWithScope(ctx, route.Scope), route.Path, route.Query)
			defer resp.Body.Close()
			framework.ExpectNotForbidden(Default, resp,
				fmt.Sprintf("%s (%s) with its own scope", route.Name, route.Path))
		})

		for _, wrong := range observabilityScopes {
			if wrong == route.Scope {
				continue
			}
			wrong := wrong

			It(fmt.Sprintf("denies %s to a token holding only %s", route.Name, wrong), func(ctx SpecContext) {
				resp := getObs(ctx, tokenWithScope(ctx, wrong), route.Path, route.Query)
				defer resp.Body.Close()
				framework.ExpectForbidden(Default, resp,
					fmt.Sprintf("%s (%s) with only %s — observability scopes must not be "+
						"interchangeable", route.Name, route.Path, wrong))
			})
		}

		It(fmt.Sprintf("denies %s to an unscoped token", route.Name), func(ctx SpecContext) {
			unscoped, err := framework.FetchTokenWithScopes(ctx, Cfg, nil)
			Expect(err).NotTo(HaveOccurred(), "failed to fetch an unscoped token")

			resp := getObs(ctx, unscoped, route.Path, route.Query)
			defer resp.Body.Close()
			framework.ExpectForbidden(Default, resp,
				fmt.Sprintf("%s (%s) with an unscoped token", route.Name, route.Path))
		})
	}

	It("rejects an unauthenticated request to every data route", func(ctx SpecContext) {
		for _, route := range obsRoutes(Cfg.DefaultOrg) {
			resp, err := getObsUnauthenticated(ctx, route.Path, route.Query)
			Expect(err).NotTo(HaveOccurred(), "request failed: %s", route.Path)

			framework.ExpectUnauthorized(Default, resp,
				fmt.Sprintf("%s (%s) with no token", route.Name, route.Path))
			resp.Body.Close()
		}
	})
})

// The publisher-audience carve-out is covered separately by security/publisher:
// it provisions a real amp-publisher-* OAuth client and verifies both Observer
// trace-only confinement and Agent Manager's publisher-only ingestion route.
