package agent

import (
	"fmt"
	"net/url"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GenerateAgentToken generates a JWT token for an agent.
func GenerateAgentToken(g Gomega, client *framework.AMPClient, orgName, projName, agentName, environment string, expiresIn string) framework.TokenResponse {
	path := fmt.Sprintf("/api/v1/agents/%s/token?%s", url.PathEscape(agentName), url.Values{"project": {projName}, "environment": {environment}}.Encode())

	req := framework.TokenRequest{
		ExpiresIn: expiresIn,
	}

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "generate token request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.TokenResponse](g, resp)
}
