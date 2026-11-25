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
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwerr"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwspec"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwsvc"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwtest"
	"github.com/wso2/agent-manager/agent-manager-service/pkg/httpx"
)

// stubBudgets answers each budget call with what its test sets.
type stubBudgets struct {
	gwsvc.BudgetService
	err      error
	imported *gwspec.BudgetImportResult
	gotName  string
	gotQuery []string
}

func (s *stubBudgets) Import(_ context.Context, _, _ string, file io.Reader, dryRun bool, onExisting string) (*gwspec.BudgetImportResult, error) {
	body, _ := io.ReadAll(file)
	s.gotQuery = []string{string(body), map[bool]string{true: "dry", false: "apply"}[dryRun], onExisting}
	return s.imported, s.err
}

func (s *stubBudgets) Get(_ context.Context, _, name string) (*gwspec.Budget, error) {
	s.gotName = name
	if s.err != nil {
		return nil, s.err
	}
	return &gwspec.Budget{Kind: "Budget", Metadata: gwspec.BudgetMetadata{Name: name}}, nil
}

func (s *stubBudgets) Usage(_ context.Context, _, name string, _, _ int32) (*gwspec.BudgetUsageListResponse, error) {
	s.gotName = name
	if s.err != nil {
		return nil, s.err
	}
	return &gwspec.BudgetUsageListResponse{}, nil
}

func (s *stubBudgets) Update(_ context.Context, _, _ string, kind gwspec.Budget) (*gwspec.Budget, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &kind, nil
}

func (s *stubBudgets) Delete(_ context.Context, _, name string) error {
	s.gotName = name
	return s.err
}

// stubCosts answers a cost report with what its test sets.
type stubCosts struct {
	gwsvc.CostService
	err error
}

func (s *stubCosts) Costs(_ context.Context, _, orgName, groupBy string, _ []string) (*gwspec.CostReport, error) {
	if s.err != nil {
		return nil, s.err
	}
	return &gwspec.CostReport{Currency: "USD", CostByGroup: []gwspec.CostByGroup{{Id: *gwspec.NewNullableString(gwspec.PtrString(orgName + "/" + groupBy))}}}, nil
}

func budgetRequest(method, target string, body io.Reader) *http.Request {
	r := httptest.NewRequest(method, target, body)
	r.SetPathValue(PathParamBudgetName, "org-daily")
	return r.WithContext(modkit.WithOrgID(r.Context(), "ou-1"))
}

func errorCode(t *testing.T, w *httptest.ResponseRecorder) (string, map[string]any) {
	t.Helper()
	var body struct {
		Code           string         `json:"code"`
		AdditionalData map[string]any `json:"additionalData"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body), w.Body.String())
	return body.Code, body.AdditionalData
}

func TestBudgetController_GetsUsesAndDeletesByName(t *testing.T) {
	s := &stubBudgets{}
	c := NewBudgetController(s)

	w := httptest.NewRecorder()
	c.GetBudget(w, budgetRequest(http.MethodGet, "/orgs/acme/budgets/org-daily", nil))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "org-daily", s.gotName)

	w = httptest.NewRecorder()
	c.GetBudgetUsage(w, budgetRequest(http.MethodGet, "/orgs/acme/budgets/org-daily/usage?limit=5&offset=10", nil))
	assert.Equal(t, http.StatusOK, w.Code)

	w = httptest.NewRecorder()
	c.GetBudgetUsage(w, budgetRequest(http.MethodGet, "/orgs/acme/budgets/org-daily/usage?offset=x", nil))
	assert.Equal(t, http.StatusBadRequest, w.Code, "a bad offset is refused")

	w = httptest.NewRecorder()
	c.DeleteBudget(w, budgetRequest(http.MethodDelete, "/orgs/acme/budgets/org-daily", nil))
	assert.Equal(t, http.StatusNoContent, w.Code)
}

func TestBudgetController_ErrorsMapToTheirStatus(t *testing.T) {
	cases := []struct {
		name   string
		err    error
		status int
		code   string
		field  string
	}{
		{"not found", &gwsvc.BudgetError{Code: gwerr.ErrCodeBudgetNotFound, Message: "no budget"}, http.StatusNotFound, gwerr.ErrCodeBudgetNotFound, ""},
		{"read only", &gwsvc.BudgetError{Code: gwerr.ErrCodeBudgetReadOnly, Message: "a gateway's budget"}, http.StatusConflict, gwerr.ErrCodeBudgetReadOnly, ""},
		{"invalid field", &gwsvc.BudgetError{Code: httpx.ErrCodeValidation, Message: "too low", Field: "spec.limit.amount"}, http.StatusBadRequest, httpx.ErrCodeValidation, "spec.limit.amount"},
		{"unexpected", errors.New("database unavailable"), http.StatusInternalServerError, httpx.ErrCodeInternalError, ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			c := NewBudgetController(&stubBudgets{err: tc.err})
			w := httptest.NewRecorder()
			c.DeleteBudget(w, budgetRequest(http.MethodDelete, "/orgs/acme/budgets/org-daily", nil))
			assert.Equal(t, tc.status, w.Code)
			code, extra := errorCode(t, w)
			assert.Equal(t, tc.code, code)
			if tc.field != "" {
				assert.Equal(t, tc.field, extra["field"], "the form shows the message beside the field")
			}
			assert.NotContains(t, w.Body.String(), "database unavailable", "internal errors are not shown")
		})
	}

	w := httptest.NewRecorder()
	update := budgetRequest(http.MethodPut, "/orgs/acme/budgets/org-daily", strings.NewReader(budgetKindYAML))
	update.Header.Set("Content-Type", "application/yaml")
	NewBudgetController(&stubBudgets{err: &gwsvc.BudgetError{Code: gwerr.ErrCodeBudgetReadOnly, Message: "a gateway's budget"}}).
		UpdateBudget(w, update)
	assert.Equal(t, http.StatusConflict, w.Code)
}

func TestBudgetController_ImportTakesACSVFile(t *testing.T) {
	csv := "name,amount\norg-daily,15.00\n"
	post := func(c BudgetController, query, contentType string) *httptest.ResponseRecorder {
		r := budgetRequest(http.MethodPost, "/orgs/acme/budgets/import"+query, strings.NewReader(csv))
		r.Header.Set("Content-Type", contentType)
		w := httptest.NewRecorder()
		c.ImportBudgets(w, r)
		return w
	}

	s := &stubBudgets{imported: &gwspec.BudgetImportResult{Summary: gwspec.BudgetImportSummary{Valid: 1, Created: 1}}}
	w := post(NewBudgetController(s), "?dryRun=true&onExisting=update", "text/csv; charset=utf-8")
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, []string{csv, "dry", "update"}, s.gotQuery)

	w = post(NewBudgetController(&stubBudgets{imported: &gwspec.BudgetImportResult{Summary: gwspec.BudgetImportSummary{Error: 1}}}), "", "text/csv")
	assert.Equal(t, http.StatusBadRequest, w.Code, "a file with a failed row is refused, with the same result")

	w = post(NewBudgetController(&stubBudgets{}), "", "application/json")
	assert.Equal(t, http.StatusBadRequest, w.Code)
	code, _ := errorCode(t, w)
	assert.Equal(t, gwerr.ErrCodeInvalidCSV, code)

	w = post(NewBudgetController(&stubBudgets{}), "?dryRun=maybe", "text/csv")
	assert.Equal(t, http.StatusBadRequest, w.Code)

	w = post(NewBudgetController(&stubBudgets{err: &gwsvc.BudgetError{Code: gwerr.ErrCodeInvalidCSV, Message: "no header"}}), "", "text/csv")
	assert.Equal(t, http.StatusBadRequest, w.Code)
}

func TestCostController_ReportsByTheOrganizationsHandle(t *testing.T) {
	core := &gwtest.Core{GetOrgFunc: func(_ context.Context, id string) (modkit.Org, error) {
		return modkit.Org{ID: id, Handle: "acme"}, nil
	}}
	get := func(c CostController) *httptest.ResponseRecorder {
		w := httptest.NewRecorder()
		c.GetCosts(w, budgetRequest(http.MethodGet, "/orgs/acme/costs?groupBy=organization", nil))
		return w
	}

	w := get(NewCostController(&stubCosts{}, core))
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Contains(t, w.Body.String(), "acme/organization")

	w = get(NewCostController(&stubCosts{err: &gwsvc.BudgetError{Code: httpx.ErrCodeValidation, Message: "unknown groupBy", Field: "groupBy"}}, core))
	assert.Equal(t, http.StatusBadRequest, w.Code)

	failing := &gwtest.Core{GetOrgFunc: func(context.Context, string) (modkit.Org, error) {
		return modkit.Org{}, errors.New("core unavailable")
	}}
	w = get(NewCostController(&stubCosts{}, failing))
	assert.Equal(t, http.StatusInternalServerError, w.Code)
}
