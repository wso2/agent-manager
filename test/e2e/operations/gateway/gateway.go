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

package gateway

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// WaitForActiveAIGateway polls the gateways API until an active AI gateway
// with the given name is found. Returns the gateway UUID.
func WaitForActiveAIGateway(client *framework.AMPClient, orgName, gatewayName string, timeout time.Duration) string {
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	path := fmt.Sprintf("/api/v1/orgs/%s/gateways", orgName)

	var gatewayUUID string
	Eventually(func(g Gomega) {
		resp, err := client.Get(path)
		g.Expect(err).NotTo(HaveOccurred(), "list gateways request failed")
		defer resp.Body.Close()

		gateways := framework.ExpectStatusAndDecode[framework.GatewayListResponse](g, resp, 200)

		var found bool
		for _, gw := range gateways.Gateways {
			if gw.Name == gatewayName && gw.GatewayType == "ai" {
				ginkgo.GinkgoWriter.Printf("AI Gateway: %s (UUID: %s, status: %s)\n", gw.Name, gw.UUID, gw.Status)
				g.Expect(gw.Status).To(Equal("ACTIVE"), "AI gateway exists but is not ACTIVE yet")
				gatewayUUID = gw.UUID
				found = true
				break
			}
		}
		g.Expect(found).To(BeTrue(), "AI gateway %q not found", gatewayName)
	}).WithTimeout(timeout).WithPolling(10 * time.Second).Should(Succeed())

	return gatewayUUID
}
