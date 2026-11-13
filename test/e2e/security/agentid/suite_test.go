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

// Package agentid verifies the credential lifecycle for the per-environment
// OAuth2 identities assigned to agents.
package agentid

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	environmentops "github.com/wso2/agent-manager/test/e2e/operations/environment"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

var (
	cfg           *framework.Config
	adminClient   *framework.AMPClient
	reducedClient *framework.AMPClient
)

func TestSecurityAgentID(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Security: AgentID Credential Lifecycle Suite")
}

var _ = BeforeSuite(func(ctx SpecContext) {
	cfg = framework.LoadConfig()

	By("Waiting for Agent Manager readiness")
	framework.WaitForAPIReady(cfg)

	By("Creating the full-privilege fixture client")
	var err error
	adminClient, err = framework.NewAMPClientWithContext(ctx, cfg)
	Expect(err).NotTo(HaveOccurred())
	framework.VerifyDefaultOrg(adminClient, cfg.DefaultOrg)

	By("Ensuring the project the agent is registered in exists")
	testsetup.EnsureProject(adminClient, cfg, cfg.DefaultProject, "Default", "Project for the AgentID security suite")

	if cfg.AgentIDTokenURL == "" {
		By("Resolving the Thunder token endpoint that issues AgentID credentials")
		// An environment with its own Thunder registers it; otherwise the
		// platform Thunder issues AgentID credentials.
		instances := environmentops.ListThunderInstances(ctx, Default, adminClient, cfg.DefaultOrg)
		for _, instance := range instances.ThunderInstances {
			if instance.EnvName == cfg.DefaultEnv && instance.TokenURL != "" {
				cfg.AgentIDTokenURL = instance.TokenURL
				break
			}
		}
		if cfg.AgentIDTokenURL == "" {
			cfg.AgentIDTokenURL = cfg.IDPTokenURL
		}
	}

	By("Creating a valid AMP token without amp:agent:update")
	reducedToken, err := framework.FetchTokenWithScopes(ctx, cfg, []string{"amp:agent:read"})
	Expect(err).NotTo(HaveOccurred())
	scopes, err := framework.TokenScopes(reducedToken)
	Expect(err).NotTo(HaveOccurred())
	Expect(scopes).To(HaveKey("amp:agent:read"),
		"the reduced token needs an AMP permission so denial is measured at authorization, not audience validation")
	Expect(scopes).NotTo(HaveKey("amp:agent:update"),
		"the AgentID negative control unexpectedly received the permission under test")
	reducedClient = framework.NewAMPClientWithToken(cfg, reducedToken)
})
