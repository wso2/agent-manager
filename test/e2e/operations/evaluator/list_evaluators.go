package evaluator

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// ListEvaluators returns all evaluators (built-in and custom) for an organization.
func ListEvaluators(t *testing.T, client *framework.AMPClient, orgName string) framework.EvaluatorListResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators", orgName)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "list evaluators request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.EvaluatorListResponse](t, resp)
}

// GetEvaluator retrieves a specific evaluator by ID.
func GetEvaluator(t *testing.T, client *framework.AMPClient, orgName, evaluatorID string) framework.EvaluatorResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators/%s", orgName, evaluatorID)

	resp, err := client.Get(path)
	if err != nil {
		framework.Fatalf(t, "get evaluator request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.EvaluatorResponse](t, resp)
}
