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

package identity

import (
	"context"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/testsetup"
)

// Client is the full-privilege API client, the org admin of these journeys.
var Client *framework.AMPClient

// Cfg is the shared test configuration.
var Cfg *framework.Config

// Provisioner creates OAuth personas bound to a role, the stand-in for a user
// signing in: the identity API assigns roles to users and groups, and only a
// client can mint a token non-interactively.
var Provisioner *framework.PersonaProvisioner

func TestIdentity(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Identity Suite")
}

var _ = BeforeSuite(func(ctx context.Context) {
	Cfg = framework.LoadConfig()
	framework.WaitForAPIReady(Cfg)
	var err error
	Client, err = framework.NewAMPClientWithContext(ctx, Cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")
	Provisioner, err = framework.NewPersonaProvisioner(ctx, Cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to reach ThunderID's admin API")
	testsetup.EnsureProject(Client, Cfg, framework.E2ESharedProjectName, "E2E Shared Project", "Shared project for e2e tests")
})
