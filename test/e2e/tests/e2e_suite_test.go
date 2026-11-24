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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

func TestE2E(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AMP E2E Root Suite")
}

var _ = Describe("Stale E2E resource cleanup", Label("cleanup"), func() {
	It("removes stale e2e resources left by prior runs", func() {
		cfg := framework.LoadConfig()

		By("Waiting for API readiness")
		framework.WaitForAPIReady(cfg)

		By("Creating API client")
		client, err := framework.NewAMPClient(cfg)
		Expect(err).NotTo(HaveOccurred(), "failed to create API client")

		By("Verifying default organization")
		framework.VerifyDefaultOrg(client, cfg.DefaultOrg)

		By("Cleaning up stale e2e resources")
		testsetup.CleanupStaleE2EResources(client, cfg.DefaultOrg)
	})
})
