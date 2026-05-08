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

package evaluators

import (
	"path/filepath"
	"runtime"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

var (
	Client      *framework.AMPClient
	Cfg         *framework.Config
	TestDataDir string
)

func TestEvaluators(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Evaluators Suite")
}

var _ = BeforeSuite(func() {
	Cfg = framework.LoadConfig()

	By("Waiting for API readiness")
	framework.WaitForAPIReady(Cfg)

	By("Creating API client")
	var err error
	Client, err = framework.NewAMPClient(Cfg)
	Expect(err).NotTo(HaveOccurred(), "failed to create API client")

	By("Verifying default organization")
	framework.VerifyDefaultOrg(Client, Cfg.DefaultOrg)

	_, thisFile, _, _ := runtime.Caller(0)
	TestDataDir = filepath.Join(filepath.Dir(thisFile), "..", "testdata")
})
