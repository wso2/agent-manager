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

package publisher

import (
	"fmt"
	"net/http"

	"github.com/google/uuid"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

func publisherPath() string {
	return fmt.Sprintf("/api/v1/publisher/monitors/%s/runs/%s/scores", uuid.NewString(), uuid.NewString())
}

var _ = Describe("SEC-PUB-001: score publisher audience", Label("security", "publisher"), func() {
	It("allows an amp-publisher-* token to reach the ingestion handler", func(ctx SpecContext) {
		resp, err := framework.NewAMPClientWithToken(cfg, publisherClient.Token).
			PostWithContext(ctx, publisherPath(), nil)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		Expect(resp.StatusCode).To(Equal(http.StatusBadRequest),
			"a valid publisher token should reach body validation; 401/403 means the audience boundary rejected it")
	})

	It("denies a normal AMP API token", func(ctx SpecContext) {
		resp, err := normalClient.PostWithContext(ctx, publisherPath(), nil)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		framework.ExpectForbidden(Default, resp, "normal API client on publisher-only score ingestion")
	})

	It("rejects a publisher lookalike audience", func(ctx SpecContext) {
		resp, err := framework.NewAMPClientWithToken(cfg, lookalikeClient.Token).
			PostWithContext(ctx, publisherPath(), nil)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		framework.ExpectUnauthorized(Default, resp, "lookalike publisher audience")
	})

	It("rejects a forged amp-publisher-* audience", func(ctx SpecContext) {
		forged, err := framework.TamperClaims(normalClient.Token(), func(claims map[string]any) {
			claims["aud"] = []string{"amp-publisher-attacker"}
		})
		Expect(err).NotTo(HaveOccurred())

		resp, err := framework.NewAMPClientWithToken(cfg, forged).PostWithContext(ctx, publisherPath(), nil)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		framework.ExpectUnauthorized(Default, resp, "valid token with attacker-edited publisher audience")
	})

	It("does not grant access to normal AMP routes", func(ctx SpecContext) {
		path := "/api/v1/orgs/" + cfg.DefaultOrg + "/agent-kinds"
		resp, err := framework.NewAMPClientWithToken(cfg, publisherClient.Token).GetWithContext(ctx, path)
		Expect(err).NotTo(HaveOccurred())
		defer resp.Body.Close()
		framework.ExpectForbidden(Default, resp, "publisher token on ordinary Agent Manager route")
	})
})

var _ = Describe("SEC-PUB-002: Observer publisher confinement", Label("security", "publisher"), func() {
	traceRoutes := []string{
		"/api/v1/traces",
		"/api/v1/traces/export",
		"/api/v1/traces/00000000000000000000000000000000/spans",
	}
	for _, path := range traceRoutes {
		path := path
		It("allows implicit trace-read on "+path, func(ctx SpecContext) {
			actualPath := path + "?organization=" + cfg.DefaultOrg
			resp := observerGet(ctx, publisherClient.Token, actualPath)
			defer resp.Body.Close()
			framework.ExpectNotForbidden(Default, resp, "publisher token on Observer trace route "+actualPath)
		})
	}

	confinedRoutes := []string{
		"/api/v1/logs",
		"/api/v1/build-logs",
		"/api/v1/monitor-run-logs",
		"/api/v1/metrics",
	}
	for _, path := range confinedRoutes {
		path := path
		It("denies publisher access to "+path, func(ctx SpecContext) {
			actualPath := path + "?organization=" + cfg.DefaultOrg
			if path == "/api/v1/build-logs" {
				actualPath += "&buildId=e2e-test-sec-absent"
			}
			if path == "/api/v1/monitor-run-logs" {
				actualPath += "&runName=e2e-test-sec-absent"
			}
			resp := observerGet(ctx, publisherClient.Token, actualPath)
			defer resp.Body.Close()
			framework.ExpectForbidden(Default, resp, "publisher confinement on Observer route "+actualPath)
		})
	}

	It("never treats the lookalike audience as an Observer publisher", func(ctx SpecContext) {
		resp := observerGet(ctx, lookalikeClient.Token, "/api/v1/traces?organization="+cfg.DefaultOrg)
		defer resp.Body.Close()
		// Production JWKS mode validates the audience and returns 401. The local
		// quick-start deliberately runs Observer with isLocalDevEnv=true, where
		// genuine tokens are parsed without issuer/audience validation; there the
		// lookalike proceeds as an ordinary zero-scope client and must get 403.
		// Either response proves the anchored amp-publisher-* matcher did not grant
		// implicit trace-read. Any handler response would fail this assertion.
		framework.ExpectStatusIn(Default, resp, http.StatusUnauthorized, http.StatusForbidden)
	})
})
