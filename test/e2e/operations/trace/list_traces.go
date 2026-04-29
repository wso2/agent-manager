package trace

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"testing"
	"time"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// ListTracesParams holds query parameters for listing traces.
type ListTracesParams struct {
	Organization string
	Project      string
	Agent        string
	Environment  string
	StartTime    string // ISO 8601
	EndTime      string // ISO 8601
	Limit        int
}

// ListTraces attempts to list traces from the traces-observer-service.
// Returns the response and an error on failure, allowing callers to decide
// whether to retry or fail.
func ListTraces(client *framework.AMPClient, params *ListTracesParams) (framework.TraceOverviewListResponse, error) {
	q := url.Values{}
	q.Set("organization", params.Organization)
	q.Set("project", params.Project)
	q.Set("agent", params.Agent)
	q.Set("environment", params.Environment)
	q.Set("startTime", params.StartTime)
	q.Set("endTime", params.EndTime)
	if params.Limit > 0 {
		q.Set("limit", fmt.Sprintf("%d", params.Limit))
	}
	q.Set("sortOrder", "desc")

	tracesURL := fmt.Sprintf("%s/api/v1/traces?%s", client.Cfg().TracesBaseURL, q.Encode())

	resp, err := client.DoRaw("GET", tracesURL)
	if err != nil {
		return framework.TraceOverviewListResponse{}, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return framework.TraceOverviewListResponse{}, fmt.Errorf("read body: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return framework.TraceOverviewListResponse{}, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}

	var result framework.TraceOverviewListResponse
	if err := json.Unmarshal(body, &result); err != nil {
		return framework.TraceOverviewListResponse{}, fmt.Errorf("decode: %w", err)
	}
	return result, nil
}

// WaitForTracesParams holds parameters for waiting on traces to appear.
type WaitForTracesParams struct {
	Organization string
	Project      string
	Agent        string
	Environment  string
	Timeout      time.Duration // default: 2 minutes
}

// WaitForTraces polls the traces API until at least one trace appears.
func WaitForTraces(t *testing.T, client *framework.AMPClient, params *WaitForTracesParams) framework.TraceOverviewListResponse {
	t.Helper()

	timeout := params.Timeout
	if timeout == 0 {
		timeout = 2 * time.Minute
	}

	startTime := time.Now().Add(-5 * time.Minute).UTC().Format(time.RFC3339)
	endTime := time.Now().Add(5 * time.Minute).UTC().Format(time.RFC3339)

	return framework.Poll(t, "traces to appear", framework.PollConfig{
		Timeout:         timeout,
		InitialInterval: 10 * time.Second,
		MaxInterval:     20 * time.Second,
	}, func() (framework.TraceOverviewListResponse, bool, error) {
		result, err := ListTraces(client, &ListTracesParams{
			Organization: params.Organization,
			Project:      params.Project,
			Agent:        params.Agent,
			Environment:  params.Environment,
			StartTime:    startTime,
			EndTime:      endTime,
			Limit:        10,
		})
		if err != nil {
			framework.Log(t, "  Traces not available yet: %v", err)
			return result, false, nil
		}
		if len(result.Traces) > 0 {
			return result, true, nil
		}
		return result, false, nil
	})
}
