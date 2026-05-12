package trace

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"time"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// ExportTracesParams holds query parameters for exporting traces.
type ExportTracesParams struct {
	Organization string
	Project      string
	Agent        string
	Environment  string
	StartTime    string // ISO 8601
	EndTime      string // ISO 8601
	Limit        int
	SortOrder    string // "asc" or "desc"
}

// ExportTraces calls the traces export endpoint and returns the raw response body.
func ExportTraces(g Gomega, client *framework.AMPClient, params *ExportTracesParams) []byte {
	q := url.Values{}
	q.Set("organization", params.Organization)
	q.Set("project", params.Project)
	q.Set("agent", params.Agent)
	q.Set("environment", params.Environment)
	if params.StartTime == "" {
		q.Set("startTime", time.Now().Add(-10*time.Minute).UTC().Format(time.RFC3339))
	} else {
		q.Set("startTime", params.StartTime)
	}
	if params.EndTime == "" {
		q.Set("endTime", time.Now().UTC().Format(time.RFC3339))
	} else {
		q.Set("endTime", params.EndTime)
	}
	if params.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", params.Limit))
	} else {
		q.Set("limit", "10")
	}
	if params.SortOrder != "" {
		q.Set("sortOrder", params.SortOrder)
	} else {
		q.Set("sortOrder", "desc")
	}

	exportURL := fmt.Sprintf("%s/api/v1/traces/export?%s", client.Cfg().TracesBaseURL, q.Encode())

	resp, err := client.DoRaw("GET", exportURL)
	g.Expect(err).NotTo(HaveOccurred(), "export traces request failed")
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	g.Expect(err).NotTo(HaveOccurred(), "failed to read export response body")

	g.Expect(resp.StatusCode).To(Equal(http.StatusOK), "export traces returned %d: %s", resp.StatusCode, string(body))
	g.Expect(body).NotTo(BeEmpty(), "export response body is empty")

	return body
}
