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
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// WaitForBuildParams holds parameters for waiting on a build to complete.
type WaitForBuildParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	Timeout     time.Duration // default: 10 minutes
}

// WaitForBuildSuccess polls the builds API until a build reaches "BuildSucceeded" status.
// It first waits for a build to appear in the builds list, then polls the individual
// build until its status is "BuildSucceeded".
// Returns the build name of the successful build.
func WaitForBuildSuccess(t *testing.T, client *framework.AMPClient, params *WaitForBuildParams) string {
	t.Helper()

	timeout := params.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds",
		params.OrgName, params.ProjectName, params.AgentName)

	// Phase 1: Poll until at least one build appears in the list.
	buildName := framework.Poll(t, "build to appear", framework.PollConfig{
		Timeout:         timeout,
		InitialInterval: 5 * time.Second,
		MaxInterval:     15 * time.Second,
	}, func() (string, bool, error) {
		resp, err := client.Get(basePath)
		if err != nil {
			return "", false, fmt.Errorf("list builds request failed: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			framework.Log(t, "  List builds returned %d: %s", resp.StatusCode, string(body))
			return "", false, nil
		}

		list := framework.DecodeBody[framework.BuildsListResponse](t, resp)
		if len(list.Builds) == 0 {
			return "", false, nil
		}
		// Pick the latest build (last in the list).
		latest := list.Builds[len(list.Builds)-1]
		return latest.BuildName, true, nil
	})

	framework.Log(t, "build %q appeared, waiting for completion...", buildName)

	// Phase 2: Poll the individual build until status = "Completed".
	// Only log when status changes to reduce noise.
	lastStatus := ""
	framework.Poll(t, "build "+buildName+" to complete", framework.PollConfig{
		Timeout:         timeout,
		InitialInterval: 10 * time.Second,
		MaxInterval:     30 * time.Second,
	}, func() (struct{}, bool, error) {
		buildPath := fmt.Sprintf("%s/%s", basePath, buildName)
		resp, err := client.Get(buildPath)
		if err != nil {
			framework.Log(t, "  Build check failed: %v", err)
			return struct{}{}, false, nil
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			framework.Log(t, "  Build check returned %d: %s", resp.StatusCode, string(body))
			return struct{}{}, false, nil
		}

		detail := framework.DecodeBody[framework.BuildDetailsResponse](t, resp)
		status := ""
		if detail.Status != nil {
			status = *detail.Status
		}

		if status != lastStatus {
			framework.Log(t, "  Build: %s", status)
			lastStatus = status
		}

		switch status {
		case "Completed":
			return struct{}{}, true, nil
		case "Failed":
			return struct{}{}, false, fmt.Errorf("build %s failed", buildName)
		default:
			return struct{}{}, false, nil
		}
	})

	return buildName
}

// GetBuildLogs retrieves the build logs for a specific build.
func GetBuildLogs(t *testing.T, client *framework.AMPClient, orgName, projName, agentName, buildName string) framework.LogsResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/builds/%s/build-logs",
		orgName, projName, agentName, buildName)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "get build logs request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.LogsResponse](t, resp)
}

