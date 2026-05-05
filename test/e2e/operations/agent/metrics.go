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
	"time"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetMetrics fetches resource metrics for a deployed agent.
func GetMetrics(g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string) framework.MetricsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/metrics",
		orgName, projName, agentName)

	req := framework.MetricsFilterRequest{
		EnvironmentName: environment,
		StartTime:       time.Now().Add(-10 * time.Minute).UTC().Format(time.RFC3339),
		EndTime:         time.Now().Add(1 * time.Minute).UTC().Format(time.RFC3339),
	}

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "metrics request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MetricsResponse](g, resp)
}
