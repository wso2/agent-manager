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

// E2EEnvPrefix is the naming prefix for all e2e test environments. It is kept
// short because environment names are length-constrained:
const E2EEnvPrefix = "e2e-"

// TestMCPServerURL is a public, always-on upstream MCP "everything" server used
// as the proxy upstream by the MCP e2e tests (proxy lifecycle, invocation, and
// the CLI agent-mcp suite). Proxy creation validates this URL via SSRF checks
// but does not require a live MCP handshake, so it is safe to reuse for
// config-only coverage.
const TestMCPServerURL = "https://db720294-98fd-40f4-85a1-cc6a3b65bc9a-prod.e1-us-east-azure.choreoapis.dev/godzilla/mcp-everything-server/v1.0/mcp"

// Shared Projects and Agent
const E2ESharedProjectName = "e2e-test-shared"
const SharedITHelpdeskAgentName = "e2e-it-helpdesk"
const E2ESharedProjectWithMultiEnvDepPipeline = "e2e-test-multienv"
const SharedA2AAgentName = "e2e-a2a-agent"
const SharedPromotableITHelpdeskAgentName = "e2e-it-helpdesk-multi"

// E2ESharedSecondEnv is the fixed promotion-target environment for the shared promotable IT helpdesk agent. It
// carries the E2EEnvPrefix so the stale-resource sweep tears it down between
// runs, and is kept short to respect the gateway Service name length limit.
const E2ESharedSecondEnv = E2EEnvPrefix + "shared-stg"

// E2ESharedKindName is the fixed kind name published by the promotion tests and
// consumed by the catalog tests. Using a stable name avoids needing cross-suite
// coordination of a randomly-generated kind name.
const E2ESharedKindName = "e2e-it-helpdesk-kind"

// E2ESharedKindVersion is the version used when publishing the shared kind.
const E2ESharedKindVersion = "1.0.0"

const (
	// E2ECLIProjectPrefix is the prefix for projects owned solely by the amctl
	// CLI e2e suite. Distinct from E2EProjectPrefix so the suite can mutate its
	// own agent freely; the stale sweep reaps it as a teardown backstop.
	E2ECLIProjectPrefix = "e2e-cli-"
	// E2ECLIProjectName is the project owning the CLI suite's agent. Torn down on
	// exit (amctl.WithSharedTeardown) and rebuilt fresh next run.
	E2ECLIProjectName = E2ECLIProjectPrefix + "shared"
	// CLILifecycleAgentName is the agent the amctl CLI suite builds, redeploys,
	// and observes.
	CLILifecycleAgentName = "e2e-cli-it-helpdesk"
)

// CLILifecycleAgent is a handle to the dedicated CLI-owned IT-helpdesk agent:
// a built/deployed/ready agent the amctl CLI suite drives mutating and
// observability commands against. Same shape as SharedITHelpdeskAgent but
// semantically owned by the CLI suite alone. It is provisioned once on parallel
// process 1 and serialized to every process via the suite's
// SynchronizedBeforeSuite, so its fields carry JSON tags.
type CLILifecycleAgent struct {
	ProjectName string          `json:"projectName"`
	AgentName   string          `json:"agentName"`
	BuildName   string          `json:"buildName"`
	EndpointURL string          `json:"endpointURL"`
	APIKey      string          `json:"apiKey"`
	InvokeReq   json.RawMessage `json:"invokeReq"`
}

// SharedITHelpdeskAgent holds details of the shared internal chat agent that is
// provisioned once in BeforeSuite and reused by multiple test suites.
type SharedITHelpdeskAgent struct {
	ProjectName string          `json:"projectName"`
	AgentName   string          `json:"agentName"`
	BuildName   string          `json:"buildName"`
	EndpointURL string          `json:"endpointURL"`
	APIKey      string          `json:"apiKey"`
	InvokeReq   json.RawMessage `json:"invokeReq"`
}

// SharedA2AAgent holds details of the shared A2A sample agent, provisioned once
// and reused by the A2A specs.
type SharedA2AAgent struct {
	ProjectName string `json:"projectName"`
	AgentName   string `json:"agentName"`
	APIKey      string `json:"apiKey"`
	CardURL     string `json:"cardURL"` // sourceUrl of the fetched platform card
}

// SharedPromotableITHelpdeskAgent holds details of the shared two-environment IT helpdesk agent
// . It is provisioned once (built, deployed to the default
// environment, and published as a kind) and reused by name across the agent,
// configuration and llmprovider domains. EndpointURL/APIKey refer to the
// default environment.
type SharedPromotableITHelpdeskAgent struct {
	ProjectName string          `json:"projectName"`
	AgentName   string          `json:"agentName"`
	BuildName   string          `json:"buildName"`
	SecondEnv   string          `json:"secondEnv"`
	EndpointURL string          `json:"endpointURL"`
	APIKey      string          `json:"apiKey"`
	InvokeReq   json.RawMessage `json:"invokeReq"`
}

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
