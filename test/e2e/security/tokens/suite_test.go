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

// Package tokens holds the authentication security suite: every way a caller
// can present a token that is not a genuine, currently-valid, correctly-signed
// one must be rejected with 401 before authorization is even considered.
package tokens

import (
	"fmt"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Client is a full-privilege client, used only to mint the genuine token that
// the tampering specs start from and to establish the positive control.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

// guardedPath is the endpoint every spec drives. It needs a single scope, needs
// no fixtures, and is side-effect free, so the only variable is the token.
const guardedPath = "/api/v1/orgs/%s/projects"

func TestSecurityTokens(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security: Authentication Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	Cfg = framework.LoadConfig()

	By("Waiting for API readiness")
	framework.WaitForAPIReady(Cfg)

	By("Creating full-privilege API client")
	var err error
	Client, err = framework.NewAMPClientWithContext(ctx, Cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")

	By("Verifying the endpoint under test accepts a genuine token")
	verifyPositiveControl(ctx)
})

// verifyPositiveControl proves the endpoint every spec drives returns 200 for a
// genuine token. Without this, all the 401 assertions below would also hold
// against a broken endpoint, an unreachable service, or a wrong path — the
// suite would report that authentication is airtight while testing nothing.
func verifyPositiveControl(ctx SpecContext) {
	resp, err := Client.GetWithContext(ctx, fmt.Sprintf(guardedPath, Cfg.DefaultOrg))
	Expect(err).NotTo(HaveOccurred(), "positive control request failed")
	defer resp.Body.Close()

	Expect(resp.StatusCode).To(Equal(http.StatusOK),
		"ABORTING: %s returned %d for a genuine full-privilege token. Every 401 assertion in "+
			"this suite would pass against a broken or unreachable endpoint, so the suite would "+
			"be vacuous. Fix the endpoint or the path before trusting these results.",
		fmt.Sprintf(guardedPath, Cfg.DefaultOrg), resp.StatusCode)
}

// get drives guardedPath with the given token, returning the response.
func get(ctx SpecContext, token string) *http.Response {
	resp, err := framework.NewAMPClientWithToken(Cfg, token).
		GetWithContext(ctx, fmt.Sprintf(guardedPath, Cfg.DefaultOrg))
	Expect(err).NotTo(HaveOccurred(), "request failed")
	return resp
}
