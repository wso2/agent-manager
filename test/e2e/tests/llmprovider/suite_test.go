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

// Package llmprovider holds e2e tests for the LLM provider domain: configuring
// a provider post-deploy on the shared single-env and two-env
// IT helpdesk agents, and a from-creation flow (Build C). The shared
// agents are reused rather than rebuilt; a single OpenAI-backed LLM provider is
// provisioned once for the post-deploy tests.

package llmprovider

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

// Client is the shared API client used by all llmprovider tests.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

// SharedITHelpdeskAgent is the shared single-environment IT helpdesk agent.
var SharedITHelpdeskAgent *framework.SharedITHelpdeskAgent

// LLMProviderID is an OpenAI-backed LLM provider, provisioned once on the
// default AI gateway and used by the post-deploy tests on the shared IT helpdesk agent and the shared promotable IT helpdesk agent.
var LLMProviderID string

func TestLLMProvider(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "LLM Provider Suite")
}

// The shared project is created once here (on a single process) because the
// internal-agent and external-agent containers both build agents into it and run
// on different parallel processes under `ginkgo -p`. The two-env promotion infra
// is provisioned lazily by the two-env container's BeforeAll (only that one
// container needs it). Each process still builds its own API client.
var _ = SynchronizedBeforeSuite(func() []byte {
	cfg := framework.LoadConfig()

	By("Waiting for API readiness")
	framework.WaitForAPIReady(cfg)

	By("Creating API client")
	client, err := framework.NewAMPClient(cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")

	By("Verifying default organization")
	framework.VerifyDefaultOrg(client, cfg.DefaultOrg)

	By("Ensuring shared project exists")
	testsetup.EnsureProject(client, cfg, framework.E2ESharedProjectName,
		"E2E Shared Project", "Shared project for e2e tests")
	return nil
}, func(_ []byte) {
	Cfg = framework.LoadConfig()
	framework.WaitForAPIReady(Cfg)
	var err error
	Client, err = framework.NewAMPClient(Cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")
})
