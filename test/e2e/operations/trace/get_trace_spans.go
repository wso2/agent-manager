package trace

import (
	"fmt"
	"net/url"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// GetTraceSpansParams holds query parameters for retrieving spans of a trace.
type GetTraceSpansParams struct {
	TraceID     string
	Namespace   string
	Project     string
	Component   string
	Environment string
	StartTime   string // ISO 8601
	EndTime     string // ISO 8601
}

// GetTraceSpans returns spans for a specific trace from the traces-observer-service.
func GetTraceSpans(t *testing.T, client *framework.AMPClient, params *GetTraceSpansParams) framework.SpanSummaryListResponse {
	t.Helper()

	q := url.Values{}
	q.Set("namespace", params.Namespace)
	q.Set("project", params.Project)
	q.Set("component", params.Component)
	q.Set("environment", params.Environment)
	q.Set("startTime", params.StartTime)
	q.Set("endTime", params.EndTime)

	tracesURL := fmt.Sprintf("%s/api/v1/traces/%s/spans?%s",
		client.Cfg().TracesBaseURL, params.TraceID, q.Encode())

	resp, err := client.DoRaw("GET", tracesURL)
	if err != nil {
		framework.Fatalf(t, "get trace spans request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.SpanSummaryListResponse](t, resp)
}
