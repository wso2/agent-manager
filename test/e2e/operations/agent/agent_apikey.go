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
	"fmt"
	"net/url"

	"github.com/google/uuid"
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateAgentAPIKey creates an API key for an agent in the given environment and
// registers a DeferCleanup to revoke it when the current spec finishes.
//
// Revoking is necessary because the gateway caps active keys per agent (currently 10);
// without cleanup each run's key accumulates and once the cap is hit the gateway
// rejects new key registration, so invocations 401 forever.
//
// Callers must create the key in the SAME It block that performs the invocation, so
// the DeferCleanup revoke runs only after the invocation completes. The key name is
// suffixed with a unique token so a not-yet-revoked key from a crashed run can't cause
// a same-name/different-ID collision. Revoke is best-effort — it only logs on failure.
func CreateAgentAPIKey(g Gomega, client *framework.AMPClient, projName, agentName string, req framework.CreateAgentAPIKeyRequest) framework.CreateAgentAPIKeyResponse {
	if req.DisplayName != "" {
		req.DisplayName = fmt.Sprintf("%s-%s", req.DisplayName, uuid.New().String()[:8])
	}

	resp, err := client.Post(framework.GWAgent(projName, agentName, "/api-keys"), req)
	g.Expect(err).NotTo(HaveOccurred(), "create agent API key request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	out := framework.DecodeBody[framework.CreateAgentAPIKeyResponse](g, resp)

	keyName := out.KeyID
	if keyName == "" {
		keyName = req.DisplayName
	}
	if keyName != "" {
		ginkgo.DeferCleanup(func() {
			delResp, derr := client.Delete(framework.GWAgent(projName, agentName, "/api-keys/%s", url.PathEscape(keyName)))
			if derr != nil {
				ginkgo.GinkgoWriter.Printf("revoke agent API key %q: request failed: %v\n", keyName, derr)
				return
			}
			delResp.Body.Close()
			ginkgo.GinkgoWriter.Printf("revoked agent API key %q (status %d)\n", keyName, delResp.StatusCode)
		})
	}

	return out
}
