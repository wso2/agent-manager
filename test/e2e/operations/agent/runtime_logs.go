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
	"io"
	"strings"
	"testing"
	"time"

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
func WaitForRuntimeLog(t *testing.T, client *framework.AMPClient, params *WaitForRuntimeLogParams) framework.LogEntry {
	t.Helper()

	timeout := params.Timeout
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/runtime-logs",
		params.OrgName, params.ProjectName, params.AgentName)

	result := framework.Poll(t, "runtime log: "+params.SearchText, framework.PollConfig{
		Timeout:         timeout,
		InitialInterval: 5 * time.Second,
		MaxInterval:     10 * time.Second,
	}, func() (framework.LogEntry, bool, error) {
		req := framework.LogFilterRequest{
			EnvironmentName: params.Environment,
			StartTime:       time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
			EndTime:         time.Now().Add(1 * time.Minute).UTC().Format(time.RFC3339),
			Limit:           100,
			SortOrder:       "desc",
		}

		resp, err := client.Post(path, req)
		if err != nil {
			return framework.LogEntry{}, false, fmt.Errorf("runtime logs request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != 200 {
			body, _ := io.ReadAll(resp.Body)
			framework.Log(t, "  Runtime logs returned %d: %s", resp.StatusCode, string(body))
			return framework.LogEntry{}, false, nil
		}

		logs := framework.DecodeBody[framework.LogsResponse](t, resp)
		for _, entry := range logs.Logs {
			if strings.Contains(entry.Log, params.SearchText) {
				framework.Log(t, "  Found: %s", entry.Log)
				return entry, true, nil
			}
		}
		return framework.LogEntry{}, false, nil
	})

	return result
}

// GetRuntimeLogs fetches runtime logs for an agent.
func GetRuntimeLogs(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, environment string) framework.LogsResponse {
	t.Helper()

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
	if err != nil {
		framework.Fatalf(t, "runtime logs request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.LogsResponse](t, resp)
}
