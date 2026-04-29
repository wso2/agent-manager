package evaluator

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// UpdateCustomEvaluator updates a custom evaluator by identifier.
func UpdateCustomEvaluator(t *testing.T, client *framework.AMPClient, orgName, identifier string, req framework.UpdateCustomEvaluatorRequest) framework.EvaluatorResponse {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators/custom/%s", orgName, identifier)

	resp, err := client.Put(path, req)
	if err != nil {
		framework.Fatalf(t, "update custom evaluator request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 200)

	return framework.DecodeBody[framework.EvaluatorResponse](t, resp)
}

// DeleteCustomEvaluator deletes a custom evaluator by identifier.
func DeleteCustomEvaluator(t *testing.T, client *framework.AMPClient, orgName, identifier string) {
	t.Helper()
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators/custom/%s", orgName, identifier)

	resp, err := client.Delete(path)
	if err != nil {
		framework.Fatalf(t, "delete custom evaluator request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 204)
}
