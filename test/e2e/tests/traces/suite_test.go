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

package traces

import (
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

var (
	Client *framework.AMPClient
	Cfg    *framework.Config
)

func TestTraces(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Traces Suite")
}

// The shared project is created once (on a single process); each process
// builds its own API client.
var _ = SynchronizedBeforeSuite(func() []byte {
	cfg := framework.LoadConfig()
	framework.WaitForAPIReady(cfg)
	client, err := framework.NewAMPClient(cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")
	framework.VerifyDefaultOrg(client, cfg.DefaultOrg)
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
