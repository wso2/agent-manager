package evaluator

import (
	"fmt"

	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/framework"
)

// UpdateCustomEvaluator updates a custom evaluator by identifier.
func UpdateCustomEvaluator(g Gomega, client *framework.AMPClient, orgName, identifier string, req framework.UpdateCustomEvaluatorRequest) framework.EvaluatorResponse {
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators/custom/%s", orgName, identifier)

	resp, err := client.Put(path, req)
	g.Expect(err).NotTo(HaveOccurred(), "update custom evaluator request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 200)

	return framework.DecodeBody[framework.EvaluatorResponse](g, resp)
}

// DeleteCustomEvaluator deletes a custom evaluator by identifier.
func DeleteCustomEvaluator(g Gomega, client *framework.AMPClient, orgName, identifier string) {
	path := fmt.Sprintf("/api/v1/orgs/%s/evaluators/custom/%s", orgName, identifier)

	resp, err := client.Delete(path)
	g.Expect(err).NotTo(HaveOccurred(), "delete custom evaluator request failed")
	defer resp.Body.Close()
	framework.ExpectStatus(g, resp, 204)
}
