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

// Package cliagent holds amctl agent commands as thin, assertion-backed
// operations over the amctl harness, mirroring operations/agent (HTTP).
package cliagent

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
)

// Agent is the subset of the CLI's agent envelope data we assert on
// (matches the server's AgentResponse shape).
type Agent struct {
	Name        string `json:"name"`
	DisplayName string `json:"displayName"`
	Description string `json:"description"`
	AgentType   struct {
		Type    string `json:"type"`
		SubType string `json:"subType"`
	} `json:"agentType"`
	Provisioning struct {
		Type string `json:"type"`
	} `json:"provisioning"`
	Status      string `json:"status"`
	ProjectName string `json:"projectName"`
	UUID        string `json:"uuid"`
}

// CreateExternalResult is the data shape of `amctl agent create --provisioning
// external --json`: {agent, token, ...}. token is empty if the post-create
// token mint failed (the CLI still exits 0).
type CreateExternalResult struct {
	Agent Agent  `json:"agent"`
	Token string `json:"token"`
}

// AgentList is the data shape of `amctl agent list --json`.
type AgentList struct {
	Agents []Agent `json:"agents"`
	Limit  int     `json:"limit"`
	Offset int     `json:"offset"`
	Total  int     `json:"total"`
}

// Names returns the agent names in the list, for membership assertions.
func (l AgentList) Names() []string {
	names := make([]string, 0, len(l.Agents))
	for _, a := range l.Agents {
		names = append(names, a.Name)
	}
	return names
}

// DeleteResult is the data shape of `amctl agent delete --json`.
type DeleteResult struct {
	Name    string `json:"name"`
	Deleted bool   `json:"deleted"`
}

// AgentStatusResult is the data shape of `amctl agent status --json`. The
// embedded environment entries decode only name+status (a subset of the CLI's
// EnvStatus); extend them if a spec ever asserts on a populated environment
// (displayName, lastDeployed, endpoints).
type AgentStatusResult struct {
	Agent        string `json:"agent"`
	Environments []struct {
		Name   string `json:"name"`
		Status string `json:"status"`
	} `json:"environments"`
}

// CreateExternalAgent runs `amctl agent create --provisioning external`.
func CreateExternalAgent(g Gomega, h *amctl.Harness, org, project, name, displayName, description string) CreateExternalResult {
	args := []string{"agent", "create", name, "--provisioning", "external",
		"--display-name", displayName, "--org", org, "--project", project, "--json"}
	if description != "" {
		args = append(args, "--description", description)
	}
	return amctl.DecodeData[CreateExternalResult](g, h.Run(args...))
}

// GetAgent runs `amctl agent get`.
func GetAgent(g Gomega, h *amctl.Harness, org, project, name string) Agent {
	return amctl.DecodeData[Agent](g, h.Run("agent", "get", name, "--org", org, "--project", project, "--json"))
}

// GetAgentExpectError runs `amctl agent get` expecting a non-zero exit.
func GetAgentExpectError(g Gomega, h *amctl.Harness, org, project, name string) amctl.EnvelopeError {
	return h.Run("agent", "get", name, "--org", org, "--project", project, "--json").ExpectError(g)
}

// ListAgents runs `amctl agent list`.
func ListAgents(g Gomega, h *amctl.Harness, org, project string) AgentList {
	return amctl.DecodeData[AgentList](g, h.Run("agent", "list", "--org", org, "--project", project, "--json"))
}

// DeleteAgent runs `amctl agent delete --yes`.
func DeleteAgent(g Gomega, h *amctl.Harness, org, project, name string) DeleteResult {
	return amctl.DecodeData[DeleteResult](g, h.Run("agent", "delete", name, "--org", org, "--project", project, "--yes", "--json"))
}

// AgentStatus runs `amctl agent status`.
func AgentStatus(g Gomega, h *amctl.Harness, org, project, name string) AgentStatusResult {
	return amctl.DecodeData[AgentStatusResult](g, h.Run("agent", "status", name, "--org", org, "--project", project, "--json"))
}

// DeployResult is the data shape of `amctl agent deploy --json`.
type DeployResult struct {
	Agent             string   `json:"agent"`
	Build             string   `json:"build"`
	ImageId           string   `json:"imageId"`
	TargetEnvironment string   `json:"targetEnvironment"`
	ConflictsResolved []string `json:"conflictsResolved"`
}

// DeployAgent runs `amctl agent deploy --yes` (a redeploy of the latest build
// to the pipeline's lowest environment) and returns the deploy result.
func DeployAgent(g Gomega, h *amctl.Harness, org, project, name string) DeployResult {
	return amctl.DecodeData[DeployResult](g, h.Run("agent", "deploy", name, "--org", org, "--project", project, "--yes", "--json"))
}

// DeployAgentEventually redeploys the agent, tolerating a transient 409
// conflict. The server rejects a redeploy while the target environment's
// release binding still reports its Ready condition as Progressing, which can
// briefly happen right after a fresh deployment settles (see the
// IsDeploymentInProgress guard in agent_manager.go). That surfaces as a 409;
// this retries on it and fails fast on any other error.
func DeployAgentEventually(h *amctl.Harness, org, project, name string, timeout time.Duration) DeployResult {
	var result DeployResult
	Eventually(func(g Gomega) {
		res := h.Run("agent", "deploy", name, "--org", org, "--project", project, "--yes", "--json")
		if res.ExitCode != 0 {
			env := res.Envelope(g)
			g.Expect(env.Error).NotTo(BeNil(), "expected error envelope on failed deploy: %s", res.Combined())
			if env.Error.Status != 409 {
				StopTrying(fmt.Sprintf("agent deploy failed (non-retryable): %s", res.Combined())).Now()
			}
			ginkgo.GinkgoWriter.Printf("agent deploy: 409 deployment still in progress, retrying (%s)\n", name)
			// Fail this attempt so Eventually retries the redeploy.
			g.Expect(res.ExitCode).To(Equal(0), "deployment still in progress (409): %s", res.Combined())
			return
		}
		result = amctl.DecodeData[DeployResult](g, res)
	}).WithTimeout(timeout).WithPolling(10 * time.Second).Should(Succeed())
	return result
}
