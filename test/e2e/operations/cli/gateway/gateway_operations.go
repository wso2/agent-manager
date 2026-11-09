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

// Package cligateway holds amctl gateway commands as thin, assertion-backed
// operations over the amctl harness, mirroring operations/gateway (HTTP).
package cligateway

import (
	"fmt"
	"time"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework/amctl"
)

// Gateway is the subset of the CLI's gateway envelope data we assert on.
type Gateway struct {
	Name        string `json:"name"`
	UUID        string `json:"uuid"`
	Status      string `json:"status"`
	GatewayType string `json:"gatewayType"`
}

// GatewayList is the data shape of `amctl gateway list --json`.
type GatewayList struct {
	Gateways []Gateway `json:"gateways"`
}

// ListGateways runs `amctl gateway list --env <env>`.
func ListGateways(g Gomega, h *amctl.Harness, org, env string) GatewayList {
	return amctl.DecodeData[GatewayList](g, h.Run("gateway", "list", "--env", env, "--org", org, "--json"))
}

// WaitForActiveEgressGateway polls `amctl gateway list` until a gateway in env
// that carries egress traffic (EGRESS or BOTH, where LLM providers are placed)
// is ACTIVE, and returns its name.
func WaitForActiveEgressGateway(h *amctl.Harness, org, env string, timeout time.Duration) string {
	var name string
	Eventually(func(g Gomega) {
		statuses := []string{}
		for _, gw := range ListGateways(g, h, org, env).Gateways {
			if gw.GatewayType == "INGRESS" {
				continue
			}
			if gw.Status == "ACTIVE" {
				name = gw.Name
				return
			}
			statuses = append(statuses, fmt.Sprintf("%s=%s", gw.Name, gw.Status))
		}
		g.Expect(name).NotTo(BeEmpty(), "no ACTIVE egress gateway in env %q yet (gateways: %v)", env, statuses)
	}).WithTimeout(timeout).WithPolling(5 * time.Second).Should(Succeed())
	return name
}
