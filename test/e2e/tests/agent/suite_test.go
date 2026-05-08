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

package agent

import (
	"os"
	"path/filepath"
	"runtime"
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

// Shared is the shared internal chat agent provisioned once in BeforeSuite.
var Shared *framework.SharedAgent

// TestDataDir is the absolute path to the testdata directory (tests/testdata).
var TestDataDir string

func TestAgent(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Agent Suite")
}

var _ = BeforeSuite(func() {
	// Resolve TestDataDir via runtime.Caller to ../testdata relative to this file.
	_, thisFile, _, _ := runtime.Caller(0)
	testsDir := filepath.Join(filepath.Dir(thisFile), "..")
	TestDataDir = filepath.Join(testsDir, "testdata")

	// Change working directory to the tests/ parent so that framework helpers
	// using relative "testdata/..." paths (e.g. ExpectJSONMatch) resolve correctly.
	Expect(os.Chdir(testsDir)).To(Succeed(), "chdir to tests directory")

	Cfg = framework.LoadConfig()

	By("Waiting for API readiness")
	framework.WaitForAPIReady(Cfg)

	By("Creating API client")
	var err error
	Client, err = framework.NewAMPClient(Cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")

	By("Verifying default organization")
	framework.VerifyDefaultOrg(Client, Cfg.DefaultOrg)

	By("Setting up shared internal chat agent")
	Shared = testsetup.SetupSharedAgent(Client, Cfg, TestDataDir)
})
