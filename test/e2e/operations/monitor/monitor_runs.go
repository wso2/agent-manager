package monitor

import (
	"fmt"
	"net/url"
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
	RunName     string
	StartedAt   *time.Time
	CompletedAt *time.Time
}

// GetMonitorRunLogs retrieves logs for a monitor run from the observer,
// keyed on the run name the monitor run response now carries. Sends the same
// absolute startedAt..completedAt range the console does for a completed run,
// and omits the range otherwise so the server's default lookback applies.
func GetMonitorRunLogs(g Gomega, client *framework.AMPClient, params *GetMonitorRunLogsParams) framework.LogsResponse {
	q := url.Values{}
	q.Set("organization", params.OrgName)
	q.Set("runName", params.RunName)
	if params.StartedAt != nil && params.CompletedAt != nil {
		q.Set("startTime", params.StartedAt.UTC().Format(time.RFC3339Nano))
		q.Set("endTime", params.CompletedAt.UTC().Format(time.RFC3339Nano))
	}

	logsURL := fmt.Sprintf("%s/api/v1/monitor-run-logs?%s", client.Cfg().ObserverBaseURL, q.Encode())

	resp, err := client.DoRaw("GET", logsURL)
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

	scope := fmt.Sprintf("org=%s project=%s agent=%s monitor=%s need>=%d success",
		params.OrgName, params.ProjectName, params.AgentName, params.MonitorName, minSuccessCount)

	var lastDiag string
	framework.AttachOnFailure("monitor-runs: last poll result", func() string { return lastDiag })

	var successRuns []framework.MonitorRunResponse
	attempt := 0
	Eventually(func(g Gomega) {
		attempt++
		runs := ListMonitorRuns(g, client, &ListMonitorRunsParams{
			OrgName:       params.OrgName,
			ProjectName:   params.ProjectName,
			AgentName:     params.AgentName,
			MonitorName:   params.MonitorName,
			IncludeScores: true,
		})

		successRuns = nil
		statuses := make([]string, 0, len(runs.Runs))
		for _, run := range runs.Runs {
			statuses = append(statuses, run.Status)
			if run.Status == "success" {
				successRuns = append(successRuns, run)
			}
		}

		lastDiag = fmt.Sprintf("%s | attempt %d | %d run(s) statuses=%v, %d success",
			scope, attempt, len(runs.Runs), statuses, len(successRuns))
		if len(successRuns) < minSuccessCount && len(runs.Runs) > 0 {
			ginkgo.GinkgoWriter.Printf("Monitor runs: %d, successful: %d (need %d), statuses=%v\n", len(runs.Runs), len(successRuns), minSuccessCount, statuses)
		}
		g.Expect(len(successRuns)).To(BeNumerically(">=", minSuccessCount),
			"not enough successful runs yet for %s (have %d run(s), statuses=%v)", scope, len(runs.Runs), statuses)
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

// WaitForMonitorRun polls until a monitor run reaches "success" status with at
// least one evaluator score, and returns that run. A scoreless success is
// treated as still in progress, because a run can report success before its
// scores are persisted (see the success case below).
func WaitForMonitorRun(client *framework.AMPClient, params *WaitForMonitorRunParams) framework.MonitorRunResponse {
	timeout := params.Timeout
	if timeout == 0 {
		timeout = 10 * time.Minute
	}

	scope := fmt.Sprintf("org=%s project=%s agent=%s monitor=%s",
		params.OrgName, params.ProjectName, params.AgentName, params.MonitorName)

	var lastDiag string
	framework.AttachOnFailure("monitor-run: last poll result", func() string { return lastDiag })

	var completedRun framework.MonitorRunResponse
	attempt := 0
	Eventually(func(g Gomega) {
		attempt++
		runs := ListMonitorRuns(g, client, &ListMonitorRunsParams{
			OrgName:       params.OrgName,
			ProjectName:   params.ProjectName,
			AgentName:     params.AgentName,
			MonitorName:   params.MonitorName,
			IncludeScores: true,
		})

		var found bool
		var pending bool
		var failedRun *framework.MonitorRunResponse
		statuses := make([]string, 0, len(runs.Runs))
		for i := range runs.Runs {
			run := runs.Runs[i]
			status := run.Status
			if run.Status == "success" {
				status = fmt.Sprintf("success(scores=%d)", len(run.Scores))
			}
			statuses = append(statuses, status)
			switch run.Status {
			case "success":
				// A run can briefly report "success" before its evaluator
				// scores are persisted. This is most visible for future
				// monitors, whose scheduled run may fire and complete before
				// freshly generated traces have been ingested and evaluated,
				// yielding a scoreless success. Treat that as still in
				// progress so we keep polling for a scored run (the next
				// scheduled run picks up the ingested traces).
				if len(run.Scores) > 0 {
					completedRun = run
					found = true
				} else {
					pending = true
				}
			case "failed":
				if failedRun == nil {
					failedRun = &runs.Runs[i]
				}
			default:
				// pending / running / queued — still in progress.
				pending = true
			}
			if found {
				break
			}
		}

		// Fail-fast only when a run failed and nothing succeeded or is still in
		// progress. The run list may include historical runs in an unspecified
		// order, so a single failed run must not abort while a newer run could
		// still be progressing toward (or already at) success.
		if !found && failedRun != nil && !pending {
			errMsg := ""
			if failedRun.ErrorMessage != nil {
				errMsg = *failedRun.ErrorMessage
			}
			lastDiag = fmt.Sprintf("%s | attempt %d | run %s failed: %s", scope, attempt, failedRun.ID, errMsg)
			StopTrying(fmt.Sprintf("monitor run %s failed (%s): %s", failedRun.ID, scope, errMsg)).Now()
		}

		lastDiag = fmt.Sprintf("%s | attempt %d | %d run(s) statuses=%v, no scored success yet", scope, attempt, len(runs.Runs), statuses)
		if !found && len(runs.Runs) > 0 {
			ginkgo.GinkgoWriter.Printf("Monitor runs: %d, statuses=%v\n", len(runs.Runs), statuses)
		}
		g.Expect(found).To(BeTrue(), "no scored successful monitor run found yet for %s (%d run(s), statuses=%v)", scope, len(runs.Runs), statuses)
	}).WithTimeout(timeout).WithPolling(15 * time.Second).Should(Succeed())

	return completedRun
}
