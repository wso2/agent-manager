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
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

const e2eProjectPrefix = "e2e-test-"

// cleanupStaleE2EResources finds and deletes e2e projects (with prefix "e2e-test-")
// that were created more than 1 hour ago. It deletes all agents within those
// projects first, then deletes the projects themselves.
// This runs from BeforeSuite before any tests execute.
func cleanupStaleE2EResources(client *framework.AMPClient, orgName string) {
	cutoff := time.Now().Add(-1 * time.Hour)

	path := fmt.Sprintf("/api/v1/orgs/%s/projects", orgName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list projects: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list projects returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read projects response: %v\n", err)
		return
	}

	var projects framework.ProjectListResponse
	if err := json.Unmarshal(body, &projects); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode projects: %v\n", err)
		return
	}

	for _, proj := range projects.Projects {
		if !strings.HasPrefix(proj.Name, e2eProjectPrefix) {
			continue
		}
		if proj.CreatedAt.After(cutoff) {
			continue
		}

		ginkgo.GinkgoWriter.Printf("stale cleanup: removing stale project %q (created %s)\n", proj.Name, proj.CreatedAt.Format(time.RFC3339))

		deleteAgentsInProject(client, orgName, proj.Name)

		// Retry project deletion — agent cleanup is async, project may still
		// report associated agents briefly after agent DELETE returns 204.
		projPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", orgName, proj.Name)
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(3 * time.Second)
			}
			delResp, err := client.Delete(projPath)
			if err != nil {
				ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete project %s: %v\n", proj.Name, err)
				break
			}
			delResp.Body.Close()
			if delResp.StatusCode == http.StatusNoContent || delResp.StatusCode == http.StatusNotFound {
				ginkgo.GinkgoWriter.Printf("stale cleanup: deleted project %s (status %d)\n", proj.Name, delResp.StatusCode)
				break
			}
			if delResp.StatusCode == http.StatusConflict && attempt < 4 {
				ginkgo.GinkgoWriter.Printf("stale cleanup: project %s still has resources, retrying...\n", proj.Name)
				continue
			}
			ginkgo.GinkgoWriter.Printf("stale cleanup: delete project %s returned status %d\n", proj.Name, delResp.StatusCode)
			break
		}
	}
}

// deleteAgentsInProject deletes all agents within a project.
func deleteAgentsInProject(client *framework.AMPClient, orgName, projName string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents", orgName, projName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list agents in %s: %v\n", projName, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return
	}

	var agents framework.AgentListResponse
	if err := json.Unmarshal(body, &agents); err != nil {
		return
	}

	for _, ag := range agents.Agents {
		agentPath := fmt.Sprintf("%s/%s", path, ag.Name)
		delResp, err := client.Delete(agentPath)
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete agent %s: %v\n", ag.Name, err)
			continue
		}
		delResp.Body.Close()
		ginkgo.GinkgoWriter.Printf("stale cleanup: deleted agent %s/%s (status %d)\n", projName, ag.Name, delResp.StatusCode)
	}
}
