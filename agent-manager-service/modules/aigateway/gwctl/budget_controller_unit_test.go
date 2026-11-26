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

package gwctl

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
)

// kindCapturingBudgets records the Budget kind a create or update hands the service.
type kindCapturingBudgets struct {
	gwsvc.BudgetService
	got *gwspec.Budget
}

func (k *kindCapturingBudgets) Create(_ context.Context, _, _ string, kind gwspec.Budget) (*gwspec.Budget, error) {
	k.got = &kind
	return &kind, nil
}

func (k *kindCapturingBudgets) Update(_ context.Context, _, _ string, kind gwspec.Budget) (*gwspec.Budget, error) {
	k.got = &kind
	return &kind, nil
}

const budgetKindYAML = `apiVersion: gateway.api-platform.wso2.com/v1
kind: Budget
metadata:
  name: api-platform-agents
  annotations:
    gateway.api-platform.wso2.com/project-id: api-platform
spec:
  limit:
    amount: "100.00"
    period: month
    action: block
    splitBy:
      type: agent
status:
  origin: gateway_api
`

func TestBudgetController_TakesTheKindAsYAMLOrJSON(t *testing.T) {
	json := `{"apiVersion":"gateway.api-platform.wso2.com/v1","kind":"Budget","metadata":{"name":"api-platform-agents"},` +
		`"spec":{"limit":{"amount":"100.00","period":"month","action":"block"}}}`
	for _, tc := range []struct{ name, contentType, body string }{
		{"yaml", "application/yaml", budgetKindYAML},
		{"json", "application/json", json},
	} {
		t.Run(tc.name, func(t *testing.T) {
			budgets := &kindCapturingBudgets{}
			c := NewBudgetController(budgets)
			req := httptest.NewRequest(http.MethodPost, "/orgs/acme/budgets", strings.NewReader(tc.body))
			req.Header.Set("Content-Type", tc.contentType)
			w := httptest.NewRecorder()
			c.CreateBudget(w, req)
			require.Equal(t, http.StatusCreated, w.Code, w.Body.String())
			require.NotNil(t, budgets.got)
			assert.Equal(t, "api-platform-agents", budgets.got.Metadata.Name)
			assert.Equal(t, "100.00", budgets.got.Spec.Limit.Amount)
			assert.Nil(t, budgets.got.Status, "status is the server's; a client's is ignored")
		})
	}
}

func TestBudgetController_RefusesUnknownFields(t *testing.T) {
	budgets := &kindCapturingBudgets{}
	c := NewBudgetController(budgets)
	req := httptest.NewRequest(http.MethodPut, "/orgs/acme/budgets/x", strings.NewReader(budgetKindYAML+"extra: 1\n"))
	req.Header.Set("Content-Type", "application/yaml")
	w := httptest.NewRecorder()
	c.UpdateBudget(w, req)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Nil(t, budgets.got)
}

func TestBudgetController_RefusesALimitBelowOne(t *testing.T) {
	c := NewBudgetController(&kindCapturingBudgets{})
	eleven := "label=a&label=b&label=c&label=d&label=e&label=f&label=g&label=h&label=i&label=j&label=k"
	for _, q := range []string{"limit=0", "limit=-1", "offset=-1", eleven} {
		w := httptest.NewRecorder()
		c.ListBudgets(w, httptest.NewRequest(http.MethodGet, "/orgs/acme/budgets?"+q, nil))
		assert.Equal(t, http.StatusBadRequest, w.Code, q)
	}
}
