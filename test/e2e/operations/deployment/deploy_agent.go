package deployment

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeployAgent deploys (or redeploys) an agent with the given image and env vars.
func DeployAgent(g Gomega, client *framework.AMPClient, orgName, projName, agentName string, req framework.DeployAgentRequest) framework.DeployAgentResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		orgName, projName, agentName)

	resp, err := client.Post(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "deploy agent request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 202)

	return framework.DecodeBody[framework.DeployAgentResponse](g, resp)
}

// GetDeploymentDetails retrieves the deployment details for an agent.
// Returns a map of environment name to deployment details.
func GetDeploymentDetails(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) map[string]framework.DeploymentDetailsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/deployments",
		orgName, projName, agentName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get deployments request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[map[string]framework.DeploymentDetailsResponse](g, resp)
}
