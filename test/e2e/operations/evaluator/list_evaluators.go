package evaluator

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// ListEvaluators returns all evaluators (built-in and custom) for an organization.
func ListEvaluators(g Gomega, client *framework.AMPClient, orgName string) framework.EvaluatorListResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators", orgName)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "list evaluators request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.EvaluatorListResponse](g, resp)
}

// GetEvaluator retrieves a specific evaluator by ID.
func GetEvaluator(g Gomega, client *framework.AMPClient, orgName, evaluatorID string) framework.EvaluatorResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators/%s", orgName, evaluatorID)

	resp, err := client.Get(path)
	g.Expect(err).NotTo(HaveOccurred(), "get evaluator request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.EvaluatorResponse](g, resp)
}
