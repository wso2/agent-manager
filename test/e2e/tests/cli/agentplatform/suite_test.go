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

package cliagentplatformtests

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

// Suite-wide shared fixture for the platform-agent suite: the loaded config, an
// authenticated API client, and the dedicated CLI-owned agent. The agent is
// provisioned exactly once on parallel process 1 and its handle is broadcast to
// every process (see WithSharedSetup below), so the observability, llm, and mcp
// specs all share one built/deployed instance instead of each parallel process
// racing to create it. Specs that change the shared agent are Serial: a
// redeploy resends the env it just read, so a concurrent config unset fails it.
var (
	cfg       *framework.Config
	apiClient *framework.AMPClient
	owned     *framework.CLILifecycleAgent
)

// H is the shared CLI harness: binary built once, logged in per parallel
// process. It also carries the once-only provisioning of the CLI-owned agent,
// folded into the harness's single SynchronizedBeforeSuite (Ginkgo allows only
// one). Provisioning over the API client racing concurrently across processes is
// exactly what produced the "already exists" creates and wedged deployments.
var H = amctl.RegisterSuite(
	amctl.WithSharedSetup(
		testsetup.SynchronizedCLILifecycleAgent(&cfg, &apiClient, &owned),
	),
	// Tear the CLI-owned agent + project down when the suite ends.
	amctl.WithSharedTeardown(
		testsetup.CleanupCLILifecycleAgent(&apiClient, &cfg),
	),
)

func TestCLIAgentPlatform(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "CLI Platform Agent Suite")
}
