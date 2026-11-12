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

// Package agent holds e2e tests for the agent lifecycle domain. It is the
// canonical builder of the shared IT helpdesk agents: internal_agent_test.go
// builds the single-env agent and promote_agent_test.go builds the
// two-env agent, both as explicit lifecycle test steps. The
// configuration, llmprovider, monitors and traces domains reuse those agents
// by name via the idempotent helpers in the testsetup package.

package agent

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

// Client is the shared API client used by all agent tests.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

func TestAgent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agent Suite")
}

// The shared project the agent specs create agents in is created once here (on
// a single process) rather than by the specs themselves, so that under
// `ginkgo -p` the specs — distributed across parallel processes — do not race
// to create it. Each process still builds its own API client.
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
