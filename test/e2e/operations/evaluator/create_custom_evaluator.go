package evaluator

import (
	"fmt"

	. "github.com/onsi/gomega"

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
func CreateCustomEvaluator(g Gomega, client *framework.AMPClient, params *CreateCustomEvaluatorParams) framework.EvaluatorResponse {
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
	g.Expect(err).NotTo(HaveOccurred(), "create custom evaluator request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 201)

	return framework.DecodeBody[framework.EvaluatorResponse](g, resp)
}
