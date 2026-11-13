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

package testsetup

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
	"github.com/wso2/agent-manager/test/e2e/operations/project"
)

// EnsureProject creates the named project in the default org unless it
// already exists, and waits until it is visible.
func EnsureProject(client *framework.AMPClient, cfg *framework.Config, name, displayName, description string) {
	projPath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", cfg.DefaultOrg, name)
	if framework.ResourceExists(client, projPath) {
		return
	}
	ginkgo.By(fmt.Sprintf("Creating project %q", name))
	project.CreateProject(Default, client, &project.CreateProjectParams{
		OrgName: cfg.DefaultOrg,
		Request: framework.NewCreateProjectRequest(name, displayName, description, "default"),
	})
	Eventually(func() bool {
		return framework.ResourceExists(client, projPath)
	}).WithTimeout(2*time.Minute).WithPolling(2*time.Second).Should(BeTrue(),
		"project %q was not visible in time after creation", name)
}
