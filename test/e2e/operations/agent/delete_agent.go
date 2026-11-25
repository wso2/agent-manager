package agent

import (
	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeleteAgent deletes an agent by name.
func DeleteAgent(g Gomega, client *framework.AMPClient, orgName, projName, agentName string) {
	path := AgentPath(projName, agentName)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete agent request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}

// DeleteAgentBestEffort deletes a disposable per-run agent and ignores the
// outcome. Intended for AfterAll teardown to free cluster CPU between suites: it
// never fails the spec, since a wedged deployment can make the delete return a
// non-204 status.
func DeleteAgentBestEffort(client *framework.AMPClient, orgName, projName, agentName string) {
	path := AgentPath(projName, agentName)
	resp, err := client.Delete(path)
	if err != nil {
		ginkgo.GinkgoWriter.Printf("teardown: delete agent %q failed: %v\n", agentName, err)
		return
	}
	defer resp.Body.Close()
	ginkgo.GinkgoWriter.Printf("teardown: deleted agent %q (status %d)\n", agentName, resp.StatusCode)
}
