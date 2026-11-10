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

package a2a

import (
	"fmt"
	"net/http"
	"time"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

func proxyPath(org, proj, agent, env string) string {
	return fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/environments/%s/a2a-proxy", org, proj, agent, env)
}

// SetProxy configures the upstream auth header of the A2A proxy, expecting 202.
func SetProxy(g Gomega, c *framework.AMPClient, org, proj, agent, env, header, value string) {
	body := map[string]any{"upstreamAuth": map[string]string{"header": header, "value": value}}
	resp, err := c.Put(proxyPath(org, proj, agent, env), body)
	g.Expect(err).NotTo(HaveOccurred())
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, http.StatusAccepted)
}

// WaitForProxyPublished polls until the A2A proxy is deployed to the gateway. A proxy that
// does not exist yet counts as not ready; a failed publish stops the wait at once.
func WaitForProxyPublished(c *framework.AMPClient, org, proj, agent, env string, timeout time.Duration) {
	waitForStatus(func() (string, string, bool) {
		resp, err := c.Get(proxyPath(org, proj, agent, env))
		if err != nil {
			return "", err.Error(), false
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusNotFound {
			return "", "", false
		}
		proxy := framework.DecodeBody[framework.A2AProxyResponse](Default, resp)
		return proxy.Status, proxy.LastError, true
	}, "DEPLOYED", "a2a proxy", timeout)
}
