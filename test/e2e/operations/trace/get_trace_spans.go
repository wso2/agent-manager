package trace

import (
	"fmt"
	"net/url"

	. "github.com/onsi/gomega"

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
func GetTraceSpans(g Gomega, client *framework.AMPClient, params *GetTraceSpansParams) framework.SpanSummaryListResponse {
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
	g.Expect(err).NotTo(HaveOccurred(), "get trace spans request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.SpanSummaryListResponse](g, resp)
}
