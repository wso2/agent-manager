package project

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// DeleteProject deletes a project by name.
func DeleteProject(t *testing.T, client *framework.AMPClient, orgName, projName string) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s", orgName, projName)

	resp, err := client.Delete(path)
	if err != nil {
		framework.Fatalf(t, "delete project request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 204)
}
