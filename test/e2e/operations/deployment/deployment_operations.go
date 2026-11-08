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

package deployment

import (
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	agentops "github.com/wso2/agent-manager/test/e2e/operations/agent"
)

// WaitForDeploymentParams holds parameters for waiting on a deployment.
type WaitForDeploymentParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	Environment string
	Timeout     time.Duration // default: 10 minutes

	// DeployedAfter, when set, ensures the wait only succeeds when the
	// deployment's LastDeployed timestamp is strictly after this time.
	// This prevents falsely passing when a previous deployment is still
	// "active" and the new one hasn't started yet.
	DeployedAfter time.Time
}

// clockSkew covers the gap between the service's clock, which stamps
// lastDeployed, and the cluster's, which stamps log lines.
const clockSkew = time.Minute

// WaitForReadiness verifies that the agent's runtime is up and serving in the
// given environment. The deployment must already be active (WaitForDeployed).
//
// Temporarily hack added to verify that agent is up and running, replace this
// with readiness probe check once its available.
//
// The startup line is searched for from the current deployment on, not from a
// fixed window: an agent reused from an earlier run started long ago and has
// logged nothing since.
func WaitForReadiness(client *framework.AMPClient, orgName, projName, agentName, envName string, timeout time.Duration) {
	dep := GetDeployment(Default, client, orgName, projName, agentName, envName)
	agentops.WaitForRuntimeLog(client, &agentops.WaitForRuntimeLogParams{
		OrgName:     orgName,
		ProjectName: projName,
		AgentName:   agentName,
		Environment: envName,
		SearchText:  "Uvicorn running on",
		Timeout:     timeout,
		Since:       dep.LastDeployed.Add(-clockSkew),
	})
}

// GetDeployment returns the agent's deployment in the given environment.
func GetDeployment(g Gomega, client *framework.AMPClient, orgName, projName, agentName, envName string) framework.DeploymentDetailsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments", orgName, projName, agentName)
	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get deployments request failed")
	defer resp.Body.Close()
	deployments := framework.ExpectStatusAndDecode[map[string]framework.DeploymentDetailsResponse](g, resp, http.StatusOK)
	dep, exists := deployments[envName]
	g.Expect(exists).To(BeTrue(), "environment %q not found in deployments", envName)
	g.Expect(dep.LastDeployed.IsZero()).To(BeFalse(), "deployment in %q has no lastDeployed time", envName)
	return dep
}

// WaitForDeployed polls the deployments API until the agent is "active" in
// the specified environment. If DeployedAfter is set, it also waits until
// the LastDeployed timestamp is newer than that time.
func WaitForDeployed(client *framework.AMPClient, params *WaitForDeploymentParams) {
	timeout := params.Timeout
	if timeout == 0 {
		timeout = 5 * time.Minute
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		params.OrgName, params.ProjectName, params.AgentName)

	Eventually(func(g Gomega) {
		resp, err := client.Get(path)
		g.Expect(err).NotTo(HaveOccurred(), "get deployments request failed")
		defer resp.Body.Close()

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			StopTrying(fmt.Sprintf("deployments check returned %d", resp.StatusCode)).Now()
		}
		deploymentsMap := framework.ExpectStatusAndDecode[map[string]framework.DeploymentDetailsResponse](g, resp, http.StatusOK)

		dep, exists := deploymentsMap[params.Environment]
		g.Expect(exists).To(BeTrue(), "environment %q not found in deployments", params.Environment)

		ginkgo.GinkgoWriter.Printf("Deployment status: %s, lastDeployed: %s\n", dep.Status, dep.LastDeployed.Format(time.RFC3339))

		// Fail fast on a terminal failure status rather than polling until timeout.
		switch strings.ToLower(dep.Status) {
		case "error", "failed":
			StopTrying(fmt.Sprintf("deployment for env %q is in terminal %q state", params.Environment, dep.Status)).Now()
		}

		g.Expect(dep.Status).To(Equal("active"), "deployment not yet active")

		if !params.DeployedAfter.IsZero() {
			g.Expect(dep.LastDeployed.After(params.DeployedAfter)).To(BeTrue(),
				"deployment lastDeployed (%s) is not after %s, still seeing previous deployment",
				dep.LastDeployed.Format(time.RFC3339), params.DeployedAfter.Format(time.RFC3339))
		}
	}).WithTimeout(timeout).WithPolling(10 * time.Second).Should(Succeed())
}

// GetEndpoints retrieves the endpoints for an agent in a given environment.
func GetEndpoints(g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) map[string]framework.EndpointConfiguration {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/endpoints?environment=%s",
		orgName, projName, agentName, environment)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get endpoints request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[map[string]framework.EndpointConfiguration](g, resp)
}

// FirstEndpointURL returns the first non-empty endpoint URL using a deterministic
// order (endpoint keys sorted ascending). Ranging over the endpoints map directly
// is non-deterministic in Go, which can make invocation specs flaky and, worse,
// pick a different endpoint across environments. Returns "" when no endpoint has a
// URL, so callers always overwrite their target (no stale value from a prior env).
func FirstEndpointURL(endpoints map[string]framework.EndpointConfiguration) string {
	keys := make([]string, 0, len(endpoints))
	for k := range endpoints {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if ep := endpoints[k]; ep.URL != "" {
			return ep.URL
		}
	}
	return ""
}
