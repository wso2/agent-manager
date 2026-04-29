package evaluator

import (
	"fmt"
	"testing"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// CreateCustomEvaluatorParams holds parameters for creating a custom evaluator.
type CreateCustomEvaluatorParams struct {
	OrgName     string
	Identifier  string
	DisplayName string
	Description string
	Type        string // "code" or "llm_judge"
	Level       string // "trace", "agent", or "llm"
	Source      string
	Tags        []string
}

// CreateCustomEvaluator creates a custom evaluator and returns the response.
// It registers a cleanup function to delete the evaluator when the test finishes.
func CreateCustomEvaluator(t *testing.T, client *framework.AMPClient, params *CreateCustomEvaluatorParams) framework.EvaluatorResponse {
	t.Helper()
	basePath := fmt.Sprintf("/api/v1/orgs/%s/evaluators/custom", params.OrgName)

	req := framework.CreateCustomEvaluatorRequest{
		Identifier:  params.Identifier,
		DisplayName: params.DisplayName,
		Description: params.Description,
		Type:        params.Type,
		Level:       params.Level,
		Source:      params.Source,
	}

	resp, err := client.Post(basePath, req)
	if err != nil {
		framework.Fatalf(t, "create custom evaluator request failed: %v", err)
	}
	defer resp.Body.Close()
	framework.RequireStatus(t, resp, 201)

	ev := framework.DecodeBody[framework.EvaluatorResponse](t, resp)

	evalPath := fmt.Sprintf("%s/%s", basePath, params.Identifier)
	framework.RegisterCleanup(t, client, evalPath, "evaluator "+params.Identifier)

	return ev
}
