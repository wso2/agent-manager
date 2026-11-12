// Copyright (c) 2026, WSO2 LLC. (https://www.wso2.com).
//
// WSO2 LLC. licenses this file to you under the Apache License,
// Version 2.0 (the "License"); you may not use this file except
// in compliance with the License.
// You may obtain a copy of the License at
//
// http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing,
// software distributed under the License is distributed on an
// "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
// KIND, either express or implied.  See the License for the
// specific language governing permissions and limitations
// under the License.

// Validates evaluator listing: the built-in evaluators are served. Custom
// evaluators need the evaluation runtime, which this deployment does not run.

package evaluators

import (
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/wso2/agent-manager/test/e2e/operations/evaluator"
)

var _ = Describe("Evaluators:", Ordered, Label("evaluators"), func() {
	var builtinEvalIdentifier string

	It("lists evaluators including the built-in ones", func() {
		evals := evaluator.ListEvaluators(Default, Client, Cfg.DefaultOrg)
		Expect(evals.Evaluators).NotTo(BeEmpty(), "expected at least one evaluator")

		for _, ev := range evals.Evaluators {
			if ev.Identifier == "length_compliance" {
				builtinEvalIdentifier = ev.Identifier
				break
			}
		}
		Expect(builtinEvalIdentifier).NotTo(BeEmpty(), "expected 'length_compliance' evaluator")
		GinkgoWriter.Printf("Found built-in evaluator: %s\n", builtinEvalIdentifier)
	})
})
