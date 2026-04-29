package monitor

import (
	"fmt"
	"testing"

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
// It registers a cleanup function to delete the monitor when the test finishes.
func CreateMonitor(t *testing.T, client *framework.AMPClient, params *CreateMonitorParams) framework.MonitorResponse {
	t.Helper()
	basePath := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors",
		params.OrgName, params.ProjectName, params.AgentName)

	resp, err := client.Post(basePath, params.Request)
	if err != nil {
		framework.Fatalf(t, "create monitor request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 201)

	mon := framework.DecodeBody[framework.MonitorResponse](t, resp)

	monPath := fmt.Sprintf("%s/%s", basePath, params.Request.Name)
	framework.RegisterCleanup(t, client, monPath, "monitor "+params.Request.Name)

	return mon
}
