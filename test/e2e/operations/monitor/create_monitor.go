package monitor

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateMonitorParams holds parameters for creating a monitor.
type CreateMonitorParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	Request     framework.CreateMonitorRequest
}

// CreateMonitor creates a monitor for an agent and returns the response.
func CreateMonitor(g Gomega, client *framework.AMPClient, params *CreateMonitorParams) framework.MonitorResponse {
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors",
		params.OrgName, params.ProjectName, params.AgentName)

	resp, err := client.Post(basePath, params.Request)
	g.Expect(err).NotTo(HaveOccurred(), "create monitor request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.MonitorResponse](g, resp)
}
