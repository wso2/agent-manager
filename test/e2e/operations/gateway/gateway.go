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
	"github.com/wso2/agent-manager/test/e2e/operations/environment"
)

// WaitForActiveGatewayForEnv polls the gateways API until a gateway serving the
// given environment reports status ACTIVE. A freshly created environment's
// gateway takes time to program its routes/auth, so callers should gate any
// invocation through that environment on this readiness check. Returns the
// gateway UUID.
func WaitForActiveGatewayForEnv(client *framework.AMPClient, orgName, envName string, timeout time.Duration) string {
	gatewayUUID, _ := WaitForActiveGatewayForEnvWithEnvUUID(client, orgName, envName, timeout)
	return gatewayUUID
}

// WaitForActiveGatewayForEnvWithEnvUUID behaves like WaitForActiveGatewayForEnv but also
// returns the environment UUID. Callers that key resources by environment UUID
// (e.g. MCP proxy per-environment blueprints) need both values. Every gateway
// serves the organization's one environment, so any ACTIVE gateway will do.
func WaitForActiveGatewayForEnvWithEnvUUID(client *framework.AMPClient, orgName, envName string, timeout time.Duration) (string, string) {
	if timeout == 0 {
		timeout = 3 * time.Minute
	}

	envUUID := environment.GetEnvironment(Default, client, orgName, envName).ID
	path := framework.AIGW("/gateways")
	scope := fmt.Sprintf("org=%s env=%s", orgName, envName)

	var lastDiag string
	framework.AttachOnFailure("env-gateway: last poll result", func() string { return lastDiag })

	var gatewayUUID string
	Eventually(func(g Gomega) {
		resp, err := client.Get(path)
		g.Expect(err).NotTo(HaveOccurred(), "list gateways request failed (%s)", scope)
		defer resp.Body.Close()

		// A 4xx is a client/config error (bad scope, auth, missing route) that
		// will not resolve by waiting — fail fast instead of polling to timeout.
		if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			lastDiag = fmt.Sprintf("%s | list gateways returned %d (non-retryable)", scope, resp.StatusCode)
			StopTrying(fmt.Sprintf("list gateways returned %d (%s)", resp.StatusCode, scope)).Now()
		}

		gateways := framework.ExpectStatusAndDecode[framework.GatewayListResponse](g, resp, 200)

		// A self-hosted gateway a spec registered may not be connected; any active one will do.
		var seen []string
		gatewayUUID = ""
		for _, gw := range gateways.List {
			seen = append(seen, gw.Metadata.Name+"="+gw.Status.State)
			if gw.Status.State == "ACTIVE" && gatewayUUID == "" {
				ginkgo.GinkgoWriter.Printf("Gateway for env %q: %s (UUID: %s)\n", envName, gw.Metadata.Name, gw.Metadata.UID)
				gatewayUUID = gw.Metadata.UID
			}
		}
		lastDiag = fmt.Sprintf("%s | gateways: %v", scope, seen)
		g.Expect(gatewayUUID).NotTo(BeEmpty(), "no ACTIVE gateway for environment %q (gateways: %v)", envName, seen)
	}).WithTimeout(timeout).WithPolling(10 * time.Second).Should(Succeed())

	return gatewayUUID, envUUID
}
