package trace

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// ListTracesParams holds query parameters for listing traces.
type ListTracesParams struct {
	Namespace   string
	Project     string
	Component   string
	Environment string
	StartTime   string // ISO 8601
	EndTime     string // ISO 8601
	Limit       int
}

// ListTraces returns traces from the traces-observer-service.
func ListTraces(t *testing.T, client *framework.AMPClient, params *ListTracesParams) framework.TraceOverviewListResponse {
	t.Helper()

	q := url.Values{}
	q.Set("namespace", params.Namespace)
	q.Set("project", params.Project)
	q.Set("component", params.Component)
	q.Set("environment", params.Environment)
	q.Set("startTime", params.StartTime)
	q.Set("endTime", params.EndTime)
	if params.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", params.Limit))
	}

	tracesURL := fmt.Sprintf("%s/api/v1/traces?%s", client.Cfg().TracesBaseURL, q.Encode())

	resp, err := client.DoRaw("GET", tracesURL)
	if err != nil {
		t.Fatalf("list traces request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.TraceOverviewListResponse](t, resp)
}
