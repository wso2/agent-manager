package agent

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GenerateAgentToken generates a JWT token for an agent.
func GenerateAgentToken(t *testing.T, client *framework.AMPClient, orgName, projName, agentName string, expiresIn string) framework.TokenResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/token", orgName, projName, agentName)

	req := framework.TokenRequest{
		ExpiresIn: expiresIn,
	}

	resp, err := client.Post(path, req)
	if err != nil {
		framework.Fatalf(t, "generate token request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.TokenResponse](t, resp)
}
