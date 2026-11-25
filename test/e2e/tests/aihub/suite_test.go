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

package aihub

import (
	"context"
	"net/http"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Client is the shared API client used by all aihub tests.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

func TestAIHub(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "AI Hub Suite")
}

var _ = BeforeSuite(func() {
	Cfg = framework.LoadConfig()

	By("Waiting for API readiness")
	framework.WaitForAPIReady(Cfg)

	By("Creating API client with the AI Hub module's scopes")
	// The shared scope catalog carries the AI Hub's scopes, as it does every linked module's.
	token, err := framework.FetchTokenWithScopes(context.Background(), Cfg, framework.AllScopes())
	Expect(err).NotTo(HaveOccurred(), "failed to fetch a token with the AI Hub scopes")
	Client = framework.NewAMPClientWithToken(Cfg, token)

	By("Verifying default organization")
	framework.VerifyDefaultOrg(Client, Cfg.DefaultOrg)
})

// skipWithoutAIHub skips the calling container's specs on a stack without the AI Hub, an optional module an
// operator installs with `amp module install ai-hub`.
func skipWithoutAIHub() {
	resp, err := Client.Get("/api/v1/ui-manifest")
	Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	manifest := framework.ExpectStatusAndDecode[struct {
		Modules []struct {
			Name string `json:"name"`
		} `json:"modules"`
		Pages []string `json:"pages"`
	}](Default, resp, http.StatusOK)
	installed := false
	for _, m := range manifest.Modules {
		installed = installed || m.Name == "ai-hub"
	}
	if !installed {
		Skip("the AI Hub module is not installed (amp module install ai-hub)")
	}
	Expect(manifest.Pages).To(ContainElement("ai-hub/subscription-plans"))
}
