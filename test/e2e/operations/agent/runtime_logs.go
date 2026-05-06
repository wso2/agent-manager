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

package agent

import (
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// WaitForRuntimeLogParams holds parameters for waiting on a specific log line.
type WaitForRuntimeLogParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	Environment string
	SearchText  string        // text to search for in logs
	Timeout     time.Duration // default: 3 minutes
}

// WaitForRuntimeLog polls the runtime logs API until the specified text appears.
// Returns the matching log entry.
func WaitForRuntimeLog(client *framework.AMPClient, params *WaitForRuntimeLogParams) framework.LogEntry {
	Expect(params.SearchText).NotTo(BeEmpty(), "SearchText must not be empty")

	timeout := params.Timeout
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/runtime-logs",
		params.OrgName, params.ProjectName, params.AgentName)

	var result framework.LogEntry
	Eventually(func(g Gomega) {
		req := framework.LogFilterRequest{
			EnvironmentName: params.Environment,
			StartTime:       time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
			EndTime:         time.Now().Add(1 * time.Minute).UTC().Format(time.RFC3339),
			Limit:           100,
			SortOrder:       "desc",
		}

		resp, err := client.Post(path, req)
		g.Expect(err).NotTo(HaveOccurred(), "runtime logs request failed")
		defer resp.Body.Close()

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			StopTrying(fmt.Sprintf("runtime logs returned %d", resp.StatusCode)).Now()
		}
		g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "runtime logs returned %d", resp.StatusCode)

		logs := framework.DecodeBody[framework.LogsResponse](g, resp)
		found := false
		for _, entry := range logs.Logs {
			if strings.Contains(entry.Log, params.SearchText) {
				ginkgo.GinkgoWriter.Printf("Found: %s\n", entry.Log)
				result = entry
				found = true
				break
			}
		}
		g.Expect(found).To(BeTrue(), "log line %q not found yet", params.SearchText)
	}).WithTimeout(timeout).WithPolling(5 * time.Second).Should(Succeed())

	return result
}

// GetRuntimeLogs fetches runtime logs for an agent.
func GetRuntimeLogs(g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) framework.LogsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/runtime-logs",
		orgName, projName, agentName)

	req := framework.LogFilterRequest{
		EnvironmentName: environment,
		StartTime:       time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		EndTime:         time.Now().Add(1 * time.Minute).UTC().Format(time.RFC3339),
		Limit:           100,
		SortOrder:       "desc",
	}

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "runtime logs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LogsResponse](g, resp)
}
