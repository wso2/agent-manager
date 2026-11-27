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

package environment

import (
	"context"
	"fmt"
	"net/http"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetEnvironment fetches an environment by name and asserts a 200 response.
func GetEnvironment(g Gomega, client *framework.AMPClient, _, envName string) framework.EnvironmentResponse {
	path := fmt.Sprintf("/api/v1/environments/%s", envName)
	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get environment request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.EnvironmentResponse](g, resp, http.StatusOK)
}

// ListThunderInstances returns the externally reachable per-environment
// Thunder endpoints registered with Agent Manager. Their host labels are
// opaque handles and must never be reconstructed from org/environment names.
func ListThunderInstances(ctx context.Context, g Gomega, client *framework.AMPClient, _ string) framework.ThunderInstanceListResponse {
	resp, err := client.GetWithContext(ctx, "/api/v1/thunder-instances")
	g.Expect(err).NotTo(HaveOccurred(), "list Thunder instances request failed")
	defer resp.Body.Close()
	return framework.ExpectStatusAndDecode[framework.ThunderInstanceListResponse](g, resp, http.StatusOK)
}
