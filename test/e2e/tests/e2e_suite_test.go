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

package tests

import (
	"encoding/json"
	"io"
	"net/http"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Client is the shared API client used by all e2e tests.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AMP E2E Suite")
}

var _ = BeforeSuite(func() {
	Cfg = framework.LoadConfig()

	By("Waiting for API readiness")
	Eventually(func() int {
		resp, err := http.Get(Cfg.AMPBaseURL + "/healthz")
		if err != nil {
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}).WithTimeout(Cfg.ReadinessTimeout).WithPolling(2 * time.Second).Should(Equal(http.StatusOK))
	GinkgoWriter.Println("API is ready")

	By("Creating API client")
	var err error
	Client, err = framework.NewAMPClient(Cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")

	By("Verifying default organization")
	verifyDefaultOrg(Client, Cfg.DefaultOrg)

	By("Cleaning up stale e2e resources")
	cleanupStaleE2EResources(Client, Cfg.DefaultOrg)
})

func verifyDefaultOrg(c *framework.AMPClient, orgName string) {
	resp, err := c.Get("/api/v1/orgs")
	Expect(err).NotTo(HaveOccurred(), "list orgs")
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK), "list orgs status")

	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred(), "read orgs response")

	var list framework.OrganizationListResponse
	Expect(json.Unmarshal(body, &list)).To(Succeed(), "decode orgs response")

	found := false
	for _, org := range list.Organizations {
		if org.Name == orgName {
			found = true
			break
		}
	}
	Expect(found).To(BeTrue(), "default org %q not found in %d organizations", orgName, list.Total)
	GinkgoWriter.Printf("Default org %q verified\n", orgName)
}
