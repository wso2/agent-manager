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
	"net/url"
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

	// Since, when set, searches every log line from this time on, oldest first,
	// so a line logged once at startup stays findable however long ago the
	// agent started and however much it has logged since. When zero, only the
	// newest lines from the last 10 minutes are searched.
	Since time.Time
}

// WaitForRuntimeLog polls the runtime logs API until the specified text appears.
// Returns the matching log entry.
func WaitForRuntimeLog(client *framework.AMPClient, params *WaitForRuntimeLogParams) framework.LogEntry {
	Expect(params.SearchText).NotTo(BeEmpty(), "SearchText must not be empty")

	timeout := params.Timeout
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	scope := fmt.Sprintf("org=%s project=%s agent=%s env=%s search=%q",
		params.OrgName, params.ProjectName, params.AgentName, params.Environment, params.SearchText)

	var lastDiag string
	framework.AttachOnFailure("runtime-log: last poll result", func() string { return lastDiag })

	var result framework.LogEntry
	attempt := 0
	Eventually(func(g Gomega) {
		attempt++
		q := url.Values{}
		q.Set("organization", params.OrgName)
		q.Set("project", params.ProjectName)
		q.Set("agent", params.AgentName)
		q.Set("environment", params.Environment)
		startTime, sortOrder := time.Now().Add(-10*time.Minute), "desc"
		if !params.Since.IsZero() {
			startTime, sortOrder = params.Since, "asc"
		}
		q.Set("startTime", startTime.UTC().Format(time.RFC3339))
		q.Set("endTime", time.Now().Add(1*time.Minute).UTC().Format(time.RFC3339))
		q.Set("limit", "100")
		q.Set("sortOrder", sortOrder)

		logsURL := fmt.Sprintf("%s/api/v1/logs?%s", client.Cfg().ObserverBaseURL, q.Encode())

		resp, err := client.DoRaw("GET", logsURL)
		g.Expect(err).NotTo(HaveOccurred(), "runtime logs request failed (%s)", scope)
		defer resp.Body.Close()

		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			lastDiag = fmt.Sprintf("%s | attempt %d | runtime-logs returned %d (non-retryable)", scope, attempt, resp.StatusCode)
			StopTrying(fmt.Sprintf("runtime logs returned %d (%s)", resp.StatusCode, scope)).Now()
		}
		logs := framework.ExpectStatusAndDecode[framework.LogsResponse](g, resp, http.StatusOK)
		found := false
		for _, entry := range logs.Logs {
			if strings.Contains(entry.Log, params.SearchText) {
				ginkgo.GinkgoWriter.Printf("Found: %s\n", entry.Log)
				result = entry
				found = true
				break
			}
		}
		lastDiag = fmt.Sprintf("%s | attempt %d | 200 OK, %d log line(s) returned, search text not present",
			scope, attempt, len(logs.Logs))
		g.Expect(found).To(BeTrue(),
			"log line %q not found yet for %s — agent is %d log lines deep; if it never appears the runtime "+
				"may not be logging it, or logs are not being ingested for this environment", params.SearchText, scope, len(logs.Logs))
	}).WithTimeout(timeout).WithPolling(15 * time.Second).Should(Succeed())

	return result
}

// GetRuntimeLogs fetches runtime logs for an agent from the observer.
func GetRuntimeLogs(g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) framework.LogsResponse {
	q := url.Values{}
	q.Set("organization", orgName)
	q.Set("project", projName)
	q.Set("agent", agentName)
	q.Set("environment", environment)
	q.Set("startTime", time.Now().Add(-10*time.Minute).UTC().Format(time.RFC3339))
	q.Set("endTime", time.Now().Add(1*time.Minute).UTC().Format(time.RFC3339))
	q.Set("limit", "100")
	q.Set("sortOrder", "desc")

	logsURL := fmt.Sprintf("%s/api/v1/logs?%s", client.Cfg().ObserverBaseURL, q.Encode())

	resp, err := client.DoRaw("GET", logsURL)
	g.Expect(err).NotTo(HaveOccurred(), "runtime logs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LogsResponse](g, resp)
}
