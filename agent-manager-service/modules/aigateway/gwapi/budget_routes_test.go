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

package gwapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwctl"
)

type importOnlyBudgets struct {
	gwctl.BudgetController
	called bool
}

func (c *importOnlyBudgets) ImportBudgets(w http.ResponseWriter, _ *http.Request) {
	c.called = true
	w.WriteHeader(http.StatusOK)
}

// Creating budgets does not let a caller replace existing ones through the import.
func TestImportBudgets_UpdateNeedsBudgetUpdate(t *testing.T) {
	cases := []struct {
		name      string
		query     string
		canUpdate bool
		want      int
	}{
		{"create only, new budgets", "", false, http.StatusOK},
		{"create only, skip existing", "?onExisting=skip", false, http.StatusOK},
		{"create only, update existing", "?onExisting=update", false, http.StatusForbidden},
		{"create only, update dry run", "?onExisting=update&dryRun=true", false, http.StatusForbidden},
		{"create and update", "?onExisting=update", true, http.StatusOK},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			ctrl := &importOnlyBudgets{}
			var asked []string
			can := func(_ *http.Request, perm string) bool {
				asked = append(asked, perm)
				return tc.canUpdate
			}
			req := httptest.NewRequest(http.MethodPost, "/orgs/default/budgets/import"+tc.query, nil)
			rec := httptest.NewRecorder()
			importBudgets(ctrl, can)(rec, req)
			assert.Equal(t, tc.want, rec.Code)
			assert.Equal(t, tc.want == http.StatusOK, ctrl.called)
			for _, p := range asked {
				assert.Equal(t, "budgets.update", p, "the runtime takes the module-relative name")
			}
		})
	}
}
