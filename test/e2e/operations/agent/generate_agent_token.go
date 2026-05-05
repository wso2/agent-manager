package agent

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GenerateAgentToken generates a JWT token for an agent.
func GenerateAgentToken(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, expiresIn string) framework.TokenResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token", orgName, projName, agentName)

	req := framework.TokenRequest{
		ExpiresIn: expiresIn,
	}

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "generate token request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.TokenResponse](g, resp)
}
