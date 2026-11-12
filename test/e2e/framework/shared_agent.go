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

package framework

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
)

// E2EPrefix list for naming resources.
const E2EProjectPrefix = "e2e-test-"
const E2EAgentPrefix = "e2e-test-agent-"
const E2ELLMProviderPrefix = "e2e-test-llm-provider-"
const E2EMCPProxyPrefix = "e2e-test-mcp-proxy-"
const E2EMonitorPrefix = "e2e-test-mon-monitor-"

// TestMCPServerURL is a public, always-on upstream MCP "everything" server used
// as the proxy upstream by the MCP e2e tests (proxy lifecycle, invocation, and
// the CLI agent-mcp suite). Proxy creation validates this URL via SSRF checks
// but does not require a live MCP handshake, so it is safe to reuse for
// config-only coverage.
const TestMCPServerURL = "https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/godzilla/mcp-everything-server/v1.0/mcp"

// Shared project
const E2ESharedProjectName = "e2e-test-shared"

const (
	// E2ECLIProjectPrefix is the prefix for projects owned solely by the amctl
	// CLI e2e suite. Distinct from E2EProjectPrefix so the suite can mutate its
	// own agent freely; the stale sweep reaps it as a teardown backstop.
	E2ECLIProjectPrefix = "e2e-cli-"
)

// WaitForAPIReady polls the health endpoint until the API is ready.
func WaitForAPIReady(cfg *Config) {
	healthClient := &http.Client{Timeout: 5 * time.Second}
	Eventually(func() int {
		resp, err := healthClient.Get(cfg.AMPBaseURL + "/healthz")
		if err != nil {
			return 0
		}
		defer resp.Body.Close()
		return resp.StatusCode
	}).WithTimeout(cfg.ReadinessTimeout).WithPolling(2 * time.Second).Should(Equal(http.StatusOK))
	ginkgo.GinkgoWriter.Println("API is ready")
}

// VerifyDefaultOrg verifies the default organization exists.
func VerifyDefaultOrg(client *AMPClient, orgName string) {
	resp, err := client.Get("/api/v1/orgs")
	Expect(err).NotTo(HaveOccurred(), "list orgs")
	defer resp.Body.Close()
	Expect(resp.StatusCode).To(Equal(http.StatusOK), "list orgs status")

	body, err := io.ReadAll(resp.Body)
	Expect(err).NotTo(HaveOccurred(), "read orgs response")

	var list OrganizationListResponse
	Expect(json.Unmarshal(body, &list)).To(Succeed(), "decode orgs response")

	found := false
	for _, org := range list.Organizations {
		if org.Name == orgName {
			found = true
			break
		}
	}
	Expect(found).To(BeTrue(), "default org %q not found in %d organizations", orgName, list.Total)
	ginkgo.GinkgoWriter.Printf("Default org %q verified\n", orgName)
}

// ResourceExists checks if an API resource exists (returns 200).
func ResourceExists(client *AMPClient, path string) bool {
	resp, err := client.Get(path)
	if err != nil {
		return false
	}
	defer resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}
