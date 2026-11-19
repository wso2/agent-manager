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
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// Client is the shared API client used by all aihub tests.
var Client *framework.AMPClient

// aihubScopes are the AI Hub module's subscription plan scopes.
var aihubScopes = []string{"amp:aihub:plan-read", "amp:aihub:plan-create", "amp:aihub:plan-update", "amp:aihub:plan-delete"}

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
	// A module's scopes are not in the core catalog the shared client requests (the route invariants keep
	// that catalog to core permissions), so this suite asks for them on top.
	token, err := framework.FetchTokenWithScopes(context.Background(), Cfg, append(framework.AllScopes(), aihubScopes...))
	Expect(err).NotTo(HaveOccurred(), "failed to fetch a token with the AI Hub scopes")
	Client = framework.NewAMPClientWithToken(Cfg, token)

	By("Verifying default organization")
	framework.VerifyDefaultOrg(Client, Cfg.DefaultOrg)
})
