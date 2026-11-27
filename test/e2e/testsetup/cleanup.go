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

package testsetup

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/onsi/ginkgo/v2"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CleanupStaleE2EResources finds and deletes e2e projects (with the
// E2EProjectPrefix) that were created more than 1 hour ago. It deletes all
// agents within those projects first, then deletes the projects themselves.
// This is intended to be called from BeforeSuite before any tests execute.
const cutoff = 1 * time.Hour

func CleanupStaleE2EResources(client *framework.AMPClient, orgName string) {
	path := "/api/v1/projects"
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

	for _, proj := range projects.List {
		name := proj.Metadata.Name
		// e2e-cli-* is the CLI suite's own project, swept here as a backstop
		// when a run dies before its teardown.
		if !strings.HasPrefix(name, framework.E2EProjectPrefix) &&
			!strings.HasPrefix(name, framework.E2ECLIProjectPrefix) {
			continue
		}
		if proj.Metadata.CreationTimestamp == nil || time.Since(*proj.Metadata.CreationTimestamp) < cutoff {
			continue
		}

		ginkgo.GinkgoWriter.Printf("stale cleanup: removing stale project %q (created %s)\n",
			name, proj.Metadata.CreationTimestamp.Format(time.RFC3339))

		deleteAgentsInProject(client, orgName, name)

		// Retry project deletion — agent cleanup is async, project may still
		// report associated agents briefly after agent DELETE returns 204.
		projPath := "/api/v1/projects/" + name
		for attempt := 0; attempt < 5; attempt++ {
			if attempt > 0 {
				time.Sleep(3 * time.Second)
			}
			delResp, err := client.Delete(projPath)
			if err != nil {
				ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete project %s: %v\n", name, err)
				break
			}
			delResp.Body.Close()
			if delResp.StatusCode == http.StatusNoContent || delResp.StatusCode == http.StatusNotFound {
				ginkgo.GinkgoWriter.Printf("stale cleanup: deleted project %s (status %d)\n",
					name, delResp.StatusCode)
				break
			}
			if delResp.StatusCode == http.StatusConflict && attempt < 4 {
				ginkgo.GinkgoWriter.Printf("stale cleanup: project %s still has resources, retrying...\n", name)
				continue
			}
			ginkgo.GinkgoWriter.Printf("stale cleanup: delete project %s returned status %d\n",
				name, delResp.StatusCode)
			break
		}
	}

	// Order matters and mirrors the platform's referential deletion guards:
	// projects (done above) → deployment pipelines (a pipeline refuses deletion while a
	// project references it) → LLM providers → environments (an environment refuses
	// deletion while a pipeline references it).
	cleanupStaleDeploymentPipelines(client, orgName)
	cleanupStaleLLMProviders(client)
	cleanupStaleMCPProxies(client)
}

// logDeleteResult reports a stale-cleanup DELETE outcome without misreporting a
// non-2xx response as a successful deletion: 204/404 are success, 409 means the
// resource is still referenced (guard) and is skipped, anything else is a failure.
func logDeleteResult(kind, name string, status int) {
	switch status {
	case http.StatusNoContent, http.StatusNotFound:
		ginkgo.GinkgoWriter.Printf("stale cleanup: deleted %s %s (status %d)\n", kind, name, status)
	case http.StatusConflict:
		ginkgo.GinkgoWriter.Printf("stale cleanup: %s %s still in use (status %d), skipping\n", kind, name, status)
	default:
		ginkgo.GinkgoWriter.Printf("stale cleanup: delete %s %s returned status %d (not removed)\n", kind, name, status)
	}
}

// cleanupStaleDeploymentPipelines deletes e2e-created deployment pipelines (name
// prefixed with "e2e-") that were created more than the cutoff ago. The "default"
// pipeline is never touched. These are org-scoped and are not cascaded by project
// deletion, so they orphan unless removed explicitly. Runs after project cleanup so
// the platform's "pipeline referenced by a project" guard does not block deletion.
func cleanupStaleDeploymentPipelines(client *framework.AMPClient, orgName string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines", orgName)
	resp, err := client.Get(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list deployment pipelines: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list deployment pipelines returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read deployment pipelines response: %v\n", err)
		return
	}

	var pipelines framework.DeploymentPipelineListResponse
	if err := json.Unmarshal(body, &pipelines); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode deployment pipelines: %v\n", err)
		return
	}

	for _, p := range pipelines.DeploymentPipelines {
		if p.Name == "default" || !strings.HasPrefix(p.Name, "e2e-") {
			continue
		}
		if time.Since(p.CreatedAt) < cutoff {
			continue
		}

		pipelinePath := fmt.Sprintf("/api/v1/orgs/%s/deployment-pipelines/%s", orgName, p.Name)
		delResp, err := client.Delete(pipelinePath)
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete deployment pipeline %s: %v\n", p.Name, err)
			continue
		}
		delResp.Body.Close()
		logDeleteResult("deployment pipeline", p.Name, delResp.StatusCode)
	}
}

// cleanupStaleLLMProviders deletes e2e-created LLM providers (name prefixed with
// "e2e", case-insensitive) created more than the cutoff ago. These are org-scoped and
// are not cascaded by project or environment deletion.
func cleanupStaleLLMProviders(client *framework.AMPClient) {
	resp, err := client.Get("/m/ai-gateway/llm-providers?limit=100")
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list LLM providers: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list LLM providers returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read LLM providers response: %v\n", err)
		return
	}

	var providers framework.LLMProviderListResponse
	if err := json.Unmarshal(body, &providers); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode LLM providers: %v\n", err)
		return
	}

	for _, p := range providers.List {
		// e2e providers are named/identified "E2E ..." / "e2e-..."; match either handle or display name.
		handle := p.Metadata.Name
		if !strings.HasPrefix(strings.ToLower(p.Spec.DisplayName), "e2e") && !strings.HasPrefix(strings.ToLower(handle), "e2e") {
			continue
		}
		if p.Metadata.CreationTimestamp != nil && time.Since(*p.Metadata.CreationTimestamp) < cutoff {
			continue
		}

		delResp, err := client.Delete(fmt.Sprintf("/m/ai-gateway/llm-providers/%s", handle))
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete LLM provider %s: %v\n", handle, err)
			continue
		}
		delResp.Body.Close()
		logDeleteResult("LLM provider", handle, delResp.StatusCode)
	}
}

// cleanupStaleMCPProxies deletes e2e-created MCP proxies (handle prefixed with
// E2EMCPProxyPrefix) created more than the cutoff ago. These are org-scoped and
// are not cascaded by project or environment deletion.
func cleanupStaleMCPProxies(client *framework.AMPClient) {
	resp, err := client.Get("/m/ai-gateway/mcp-proxies")
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to list MCP proxies: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		ginkgo.GinkgoWriter.Printf("stale cleanup: list MCP proxies returned %d, skipping\n", resp.StatusCode)
		return
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to read MCP proxies response: %v\n", err)
		return
	}

	var proxies framework.MCPProxyListResponse
	if err := json.Unmarshal(body, &proxies); err != nil {
		ginkgo.GinkgoWriter.Printf("stale cleanup: failed to decode MCP proxies: %v\n", err)
		return
	}

	for _, p := range proxies.List {
		handle := p.Metadata.Name
		if !strings.HasPrefix(handle, framework.E2EMCPProxyPrefix) {
			continue
		}
		// A row whose age cannot be read is left alone rather than deleted out from
		// under a concurrent run.
		created := p.Metadata.CreationTimestamp
		if created == nil || time.Since(*created) < cutoff {
			continue
		}

		delResp, err := client.Delete(fmt.Sprintf("/m/ai-gateway/mcp-proxies/%s", handle))
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete MCP proxy %s: %v\n", handle, err)
			continue
		}
		delResp.Body.Close()
		logDeleteResult("MCP proxy", handle, delResp.StatusCode)
	}
}

// deleteAgentsInProject deletes all agents within a project.
func deleteAgentsInProject(client *framework.AMPClient, orgName, projName string) {
	resp, err := client.Get("/api/v1/agents?" + url.Values{"project": {projName}, "limit": {"500"}}.Encode())
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

	for _, ag := range agents.List {
		name := ag.Metadata.Name
		delResp, err := client.Delete(fmt.Sprintf("/api/v1/agents/%s?%s", url.PathEscape(name), url.Values{"project": {projName}}.Encode()))
		if err != nil {
			ginkgo.GinkgoWriter.Printf("stale cleanup: failed to delete agent %s: %v\n", name, err)
			continue
		}
		delResp.Body.Close()
		ginkgo.GinkgoWriter.Printf("stale cleanup: deleted agent %s/%s (status %d)\n",
			projName, name, delResp.StatusCode)
	}
}
