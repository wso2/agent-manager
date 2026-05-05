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

package build

import (
	"fmt"
	"net/http"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// WaitForBuildParams holds parameters for waiting on a build to complete.
type WaitForBuildParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	Timeout     time.Duration // default: 10 minutes
}

// WaitForBuildSuccess polls the builds API until a build reaches "Completed" status.
// It first waits for a build to appear in the builds list, then polls the individual
// build until its status is "Completed".
// Returns the build name of the successful build.
func WaitForBuildSuccess(client *framework.AMPClient, params *WaitForBuildParams) string {
	timeout := params.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds",
		params.OrgName, params.ProjectName, params.AgentName)

	// Phase 1: Wait for at least one build to appear in the list.
	var buildName string
	Eventually(func(g Gomega) {
		resp, err := client.Get(basePath)
		g.Expect(err).NotTo(HaveOccurred(), "list builds request failed")
		defer resp.Body.Close()

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			StopTrying(fmt.Sprintf("list builds returned %d", resp.StatusCode)).Now()
		}
		g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "list builds returned %d", resp.StatusCode)
		list := framework.DecodeBody[framework.BuildsListResponse](g, resp)
		g.Expect(list.Builds).NotTo(BeEmpty(), "no builds found yet")

		buildName = list.Builds[len(list.Builds)-1].BuildName
	}).WithTimeout(timeout).WithPolling(5 * time.Second).Should(Succeed())

	ginkgo.GinkgoWriter.Printf("Build %q appeared, waiting for completion...\n", buildName)

	// Phase 2: Poll the individual build until status = "Completed".
	Eventually(func(g Gomega) {
		buildPath := fmt.Sprintf("%s/%s", basePath, buildName)
		resp, err := client.Get(buildPath)
		g.Expect(err).NotTo(HaveOccurred(), "build check failed")
		defer resp.Body.Close()

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			StopTrying(fmt.Sprintf("build check returned %d", resp.StatusCode)).Now()
		}
		g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "build check returned %d", resp.StatusCode)
		detail := framework.DecodeBody[framework.BuildDetailsResponse](g, resp)

		status := ""
		if detail.Status != nil {
			status = *detail.Status
		}

		ginkgo.GinkgoWriter.Printf("Build status: %s\n", status)

		if status == "Failed" {
			StopTrying(fmt.Sprintf("build %s failed", buildName)).Now()
		}

		g.Expect(status).To(Equal("Completed"), "build %s not yet completed", buildName)
	}).WithTimeout(timeout).WithPolling(10 * time.Second).Should(Succeed())

	return buildName
}

// GetBuildLogs retrieves the build logs for a specific build.
func GetBuildLogs(g Gomega, client *framework.AMPClient, orgName, projName, agentName, buildName string) framework.LogsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds/%s/build-logs",
		orgName, projName, agentName, buildName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get build logs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LogsResponse](g, resp)
}
