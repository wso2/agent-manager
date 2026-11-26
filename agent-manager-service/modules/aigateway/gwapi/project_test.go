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
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/wso2/agent-manager/agent-manager-service/modkit"
	"github.com/wso2/agent-manager/agent-manager-service/modules/aigateway/gwmodels"
)

// envScope records the environment a handler names on the audit record.
type envScope struct{ env string }

func (s *envScope) AuditDetail(string, any)                                    {}
func (s *envScope) AuditResource(string, string, string)                       {}
func (s *envScope) AuditEnvironment(name string)                               { s.env = name }
func (s *envScope) AuditInternalCaller(context.Context, modkit.InternalCaller) {}

func TestInProject(t *testing.T) {
	serve := func(target string) (*envScope, *httptest.ResponseRecorder, bool) {
		scope := &envScope{}
		ran := false
		h := inProject(func(w http.ResponseWriter, _ *http.Request) { ran = true; w.WriteHeader(http.StatusOK) })
		req := httptest.NewRequest(http.MethodGet, target, nil)
		req = req.WithContext(modkit.WithAuditScope(req.Context(), scope))
		w := httptest.NewRecorder()
		h(w, req)
		return scope, w, ran
	}

	scope, w, ran := serve("/agents/a1?project=p1")
	assert.True(t, ran)
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, gwmodels.DefaultEnvironmentName, scope.env, "agent routes name the default environment")

	scope, w, ran = serve("/agents/a1")
	assert.False(t, ran)
	assert.Equal(t, http.StatusBadRequest, w.Code)
	assert.Empty(t, scope.env)
}
