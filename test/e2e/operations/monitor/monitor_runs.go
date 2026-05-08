package monitor

import (
	"fmt"
	"time"

	"github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// ListMonitorRunsParams holds parameters for listing monitor runs.
type ListMonitorRunsParams struct {
	OrgName       string
	ProjectName   string
	AgentName     string
	MonitorName   string
	IncludeScores bool
}

// ListMonitorRuns retrieves runs for a monitor.
func ListMonitorRuns(g Gomega, client *framework.AMPClient, params *ListMonitorRunsParams) framework.MonitorRunListResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s/runs",
		params.OrgName, params.ProjectName, params.AgentName, params.MonitorName)

	if params.IncludeScores {
		path += "?includeScores=true"
	}

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list monitor runs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MonitorRunListResponse](g, resp)
}

// GetMonitorRunScores retrieves scores for a specific monitor run.
func GetMonitorRunScores(g Gomega, client *framework.AMPClient, orgName, projName, agentName, monitorName, runID string) framework.MonitorRunScoresResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s/runs/%s/scores",
		orgName, projName, agentName, monitorName, runID)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get monitor run scores request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.MonitorRunScoresResponse](g, resp)
}

// GetMonitorRunLogsParams holds parameters for getting monitor run logs.
type GetMonitorRunLogsParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	MonitorName string
	RunID       string
}

// GetMonitorRunLogs retrieves logs for a specific monitor run.
func GetMonitorRunLogs(g Gomega, client *framework.AMPClient, params *GetMonitorRunLogsParams) framework.LogsResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s/runs/%s/logs",
		params.OrgName, params.ProjectName, params.AgentName, params.MonitorName, params.RunID)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get monitor run logs request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.LogsResponse](g, resp)
}

// RerunMonitorParams holds parameters for rerunning a monitor.
type RerunMonitorParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	MonitorName string
	RunID       string
}

// RerunMonitor triggers a rerun of a monitor run.
func RerunMonitor(g Gomega, client *framework.AMPClient, params *RerunMonitorParams) framework.MonitorRunResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/projects/%s/agents/%s/monitors/%s/runs/%s/rerun",
		params.OrgName, params.ProjectName, params.AgentName, params.MonitorName, params.RunID)

	resp, err := client.Post(path, map[string]any{})
	g.Expect(err).NotTo(HaveOccurred(), "rerun monitor request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.MonitorRunResponse](g, resp)
}

// WaitForMonitorRunCount polls until the monitor has at least the given number of successful runs.
func WaitForMonitorRunCount(client *framework.AMPClient, params *WaitForMonitorRunParams, minSuccessCount int) []framework.MonitorRunResponse {
	timeout := params.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	var successRuns []framework.MonitorRunResponse
	Eventually(func(g Gomega) {
		runs := ListMonitorRuns(g, client, &ListMonitorRunsParams{
			OrgName:       params.OrgName,
			ProjectName:   params.ProjectName,
			AgentName:     params.AgentName,
			MonitorName:   params.MonitorName,
			IncludeScores: true,
		})

		successRuns = nil
		for _, run := range runs.Runs {
			if run.Status == "success" {
				successRuns = append(successRuns, run)
			}
		}

		if len(successRuns) < minSuccessCount && len(runs.Runs) > 0 {
			ginkgo.GinkgoWriter.Printf("Monitor runs: %d, successful: %d (need %d)\n", len(runs.Runs), len(successRuns), minSuccessCount)
		}
		g.Expect(len(successRuns)).To(BeNumerically(">=", minSuccessCount), "not enough successful runs yet")
	}).WithTimeout(timeout).WithPolling(15 * time.Second).Should(Succeed())

	return successRuns
}

// WaitForMonitorRunParams holds parameters for waiting on a monitor run.
type WaitForMonitorRunParams struct {
	OrgName     string
	ProjectName string
	AgentName   string
	MonitorName string
	Timeout     time.Duration // default: 10 minutes
}

// WaitForMonitorRun polls until a monitor run reaches "completed" status.
// Returns the first completed run.
func WaitForMonitorRun(client *framework.AMPClient, params *WaitForMonitorRunParams) framework.MonitorRunResponse {
	timeout := params.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	var completedRun framework.MonitorRunResponse
	Eventually(func(g Gomega) {
		runs := ListMonitorRuns(g, client, &ListMonitorRunsParams{
			OrgName:       params.OrgName,
			ProjectName:   params.ProjectName,
			AgentName:     params.AgentName,
			MonitorName:   params.MonitorName,
			IncludeScores: true,
		})

		var found bool
		for _, run := range runs.Runs {
			if run.Status == "failed" {
				errMsg := ""
				if run.ErrorMessage != nil {
					errMsg = *run.ErrorMessage
				}
				StopTrying(fmt.Sprintf("monitor run %s failed: %s", run.ID, errMsg)).Now()
			}
			if run.Status == "success" {
				completedRun = run
				found = true
				break
			}
		}

		if !found && len(runs.Runs) > 0 {
			ginkgo.GinkgoWriter.Printf("Monitor runs: %d, latest status: %s\n", len(runs.Runs), runs.Runs[0].Status)
		}
		g.Expect(found).To(BeTrue(), "no completed monitor run found yet")
	}).WithTimeout(timeout).WithPolling(15 * time.Second).Should(Succeed())

	return completedRun
}
